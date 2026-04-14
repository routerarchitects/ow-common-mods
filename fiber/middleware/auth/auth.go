package auth

import (
	"context"
	"crypto/subtle"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/gofiber/fiber/v3"
)

const (
	defaultInternalNameHeader  = "X-INTERNAL-NAME"
	defaultAPIKeyHeader        = "X-API-KEY"
	defaultAuthorizationHeader = "Authorization"
	defaultBearerPrefix        = "Bearer "
	defaultValidationTimeout   = 5 * time.Second
)

// InternalAPIKeyConfig configures private API-key middleware.
type InternalAPIKeyConfig struct {
	// InternalNameHeader is the header used to identify internal callers.
	// Defaults to X-INTERNAL-NAME.
	InternalNameHeader string
	// APIKeyHeader is the header that carries the caller's API key.
	// Defaults to X-API-KEY.
	APIKeyHeader string

	// AllowedInternalName restricts calls to a single internal caller name.
	// When empty, only non-empty presence is required.
	AllowedInternalName string

	// ExpectedAPIKey is the static expected API key.
	ExpectedAPIKey string

	// OnUnauthorized customizes unauthorized responses.
	// Defaults to StatusUnauthorized.
	OnUnauthorized func(c fiber.Ctx) error
}

// PublicAuthValidator validates public auth credentials.
// Implementations can call another auth service.
type PublicAuthValidator interface {
	// ValidateToken validates a bearer token.
	// The context passed here is derived from fiber.Ctx.Context() and wrapped with ValidationTimeout.
	ValidateToken(ctx context.Context, token string) error
	// ValidateAPIKey validates an API key.
	// The context passed here is derived from fiber.Ctx.Context() and wrapped with ValidationTimeout.
	ValidateAPIKey(ctx context.Context, apiKey string) error
}

// PublicAuthConfig configures public auth middleware.
type PublicAuthConfig struct {
	// AuthorizationHeader is the header that carries the auth token.
	// Defaults to Authorization.
	AuthorizationHeader string
	// BearerPrefix is trimmed from the start of Authorization header when present.
	// Defaults to "Bearer ".
	BearerPrefix string
	// APIKeyHeader is the header that carries API key credentials.
	// Defaults to X-API-KEY.
	APIKeyHeader string
	// ValidationTimeout bounds each credential validation call.
	// Defaults to 5 seconds when unset or invalid.
	ValidationTimeout time.Duration
	// Validator validates incoming credentials.
	Validator PublicAuthValidator
	// OnUnauthorized customizes unauthorized responses.
	// Defaults to StatusUnauthorized.
	OnUnauthorized func(c fiber.Ctx) error
	// OnValidationError can map validator errors to custom responses.
	// If unset, any validator error returns unauthorized.
	OnValidationError func(c fiber.Ctx, err error) error
}

var (
	ErrMissingExpectedAPIKey = errors.New("auth: ExpectedAPIKey is required")
	ErrMissingValidator      = errors.New("auth: Validator is required")
)

// RequireInternalAPIKey validates internal calls using internal-name and API key headers.
func RequireInternalAPIKey(cfg InternalAPIKeyConfig) (fiber.Handler, error) {
	internalHeader := firstNonEmpty(cfg.InternalNameHeader, defaultInternalNameHeader)
	apiKeyHeader := firstNonEmpty(cfg.APIKeyHeader, defaultAPIKeyHeader)
	expected := strings.TrimSpace(cfg.ExpectedAPIKey)
	if expected == "" {
		return nil, ErrMissingExpectedAPIKey
	}
	unauthorized := cfg.OnUnauthorized
	if unauthorized == nil {
		unauthorized = func(c fiber.Ctx) error {
			return c.SendStatus(http.StatusUnauthorized)
		}
	}

	return func(c fiber.Ctx) error {
		internalName := strings.TrimSpace(c.Get(internalHeader))
		if internalName == "" {
			return unauthorized(c)
		}

		if allowed := strings.TrimSpace(cfg.AllowedInternalName); allowed != "" && internalName != allowed {
			return unauthorized(c)
		}

		got := strings.TrimSpace(c.Get(apiKeyHeader))
		if got == "" {
			return unauthorized(c)
		}

		if !secureEqual(got, expected) {
			return unauthorized(c)
		}

		return c.Next()
	}, nil
}

// RequirePublicAuth validates external/public requests using configured auth method.
func RequirePublicAuth(cfg PublicAuthConfig) (fiber.Handler, error) {
	authorizationHeader := firstNonEmpty(cfg.AuthorizationHeader, defaultAuthorizationHeader)
	bearerPrefix := firstNonEmpty(cfg.BearerPrefix, defaultBearerPrefix)
	apiKeyHeader := firstNonEmpty(cfg.APIKeyHeader, defaultAPIKeyHeader)
	validationTimeout := cfg.ValidationTimeout
	if validationTimeout <= 0 {
		validationTimeout = defaultValidationTimeout
	}
	validator := cfg.Validator
	if validator == nil {
		return nil, ErrMissingValidator
	}
	unauthorized := cfg.OnUnauthorized
	if unauthorized == nil {
		unauthorized = func(c fiber.Ctx) error {
			return c.SendStatus(http.StatusUnauthorized)
		}
	}

	return func(c fiber.Ctx) error {
		var validationErr error

		apiKey := strings.TrimSpace(c.Get(apiKeyHeader))
		if apiKey != "" {
			validationCtx, cancel := context.WithTimeout(c.Context(), validationTimeout)
			err := validator.ValidateAPIKey(validationCtx, apiKey)
			cancel()
			if err == nil {
				return c.Next()
			} else {
				validationErr = errors.Join(validationErr, err)
			}
		}

		authHeader := strings.TrimSpace(c.Get(authorizationHeader))
		if authHeader != "" {
			token, ok := extractBearerToken(authHeader, bearerPrefix)
			if ok && token != "" {
				validationCtx, cancel := context.WithTimeout(c.Context(), validationTimeout)
				err := validator.ValidateToken(validationCtx, token)
				cancel()
				if err == nil {
					return c.Next()
				} else {
					validationErr = errors.Join(validationErr, err)
				}
			}
		}

		if validationErr != nil && cfg.OnValidationError != nil {
			return cfg.OnValidationError(c, validationErr)
		}
		return unauthorized(c)
	}, nil
}

func secureEqual(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

func firstNonEmpty(value, fallback string) string {
	v := strings.TrimSpace(value)
	if v == "" {
		return fallback
	}
	return v
}

func extractBearerToken(authHeader, bearerPrefix string) (string, bool) {
	token := strings.TrimSpace(authHeader)
	prefix := firstNonEmpty(bearerPrefix, defaultBearerPrefix)

	if len(token) < len(prefix) || !strings.EqualFold(token[:len(prefix)], prefix) {
		return "", false
	}

	token = strings.TrimSpace(token[len(prefix):])
	return token, true
}
