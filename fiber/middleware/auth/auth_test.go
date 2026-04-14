package auth

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
)

type validatorStub struct {
	lastToken        string
	lastAPIKey       string
	lastTokenCtx     context.Context
	lastAPIKeyCtx    context.Context
	tokenErr         error
	apiKeyErr        error
	validateTokenFn  func(ctx context.Context, token string) error
	validateAPIKeyFn func(ctx context.Context, apiKey string) error
}

func (v *validatorStub) ValidateToken(ctx context.Context, token string) error {
	v.lastTokenCtx = ctx
	v.lastToken = token
	if v.validateTokenFn != nil {
		return v.validateTokenFn(ctx, token)
	}
	return v.tokenErr
}

func (v *validatorStub) ValidateAPIKey(ctx context.Context, apiKey string) error {
	v.lastAPIKeyCtx = ctx
	v.lastAPIKey = apiKey
	if v.validateAPIKeyFn != nil {
		return v.validateAPIKeyFn(ctx, apiKey)
	}
	return v.apiKeyErr
}

func mustInternalAuth(t *testing.T, cfg InternalAPIKeyConfig) fiber.Handler {
	t.Helper()

	handler, err := RequireInternalAPIKey(cfg)
	if err != nil {
		t.Fatalf("RequireInternalAPIKey() error = %v", err)
	}
	return handler
}

func mustPublicAuth(t *testing.T, cfg PublicAuthConfig) fiber.Handler {
	t.Helper()

	handler, err := RequirePublicAuth(cfg)
	if err != nil {
		t.Fatalf("RequirePublicAuth() error = %v", err)
	}
	return handler
}

func TestRequireInternalAPIKey_AllowsValidRequest(t *testing.T) {
	t.Parallel()

	app := fiber.New()
	app.Use(mustInternalAuth(t, InternalAPIKeyConfig{
		ExpectedAPIKey: "secret-key",
	}))
	app.Get("/", func(c fiber.Ctx) error { return c.SendString("ok") })

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-INTERNAL-NAME", "svc-a")
	req.Header.Set("X-API-KEY", "secret-key")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test() error = %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "ok" {
		t.Fatalf("body = %q, want %q", string(body), "ok")
	}
}

func TestRequireInternalAPIKey_RejectsMissingInternalHeader(t *testing.T) {
	t.Parallel()

	app := fiber.New()
	app.Use(mustInternalAuth(t, InternalAPIKeyConfig{
		ExpectedAPIKey: "secret-key",
	}))
	app.Get("/", func(c fiber.Ctx) error { return c.SendString("ok") })

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-API-KEY", "secret-key")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test() error = %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusUnauthorized)
	}
}

func TestRequireInternalAPIKey_RejectsWrongAPIKey(t *testing.T) {
	t.Parallel()

	app := fiber.New()
	app.Use(mustInternalAuth(t, InternalAPIKeyConfig{
		ExpectedAPIKey: "secret-key",
	}))
	app.Get("/", func(c fiber.Ctx) error { return c.SendString("ok") })

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-INTERNAL-NAME", "svc-a")
	req.Header.Set("X-API-KEY", "wrong-key")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test() error = %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusUnauthorized)
	}
}

func TestRequireInternalAPIKey_AllowedInternalName(t *testing.T) {
	t.Parallel()

	app := fiber.New()
	app.Use(mustInternalAuth(t, InternalAPIKeyConfig{
		ExpectedAPIKey:      "secret-key",
		AllowedInternalName: "svc-a",
	}))
	app.Get("/", func(c fiber.Ctx) error { return c.SendString("ok") })

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-INTERNAL-NAME", "svc-b")
	req.Header.Set("X-API-KEY", "secret-key")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test() error = %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusUnauthorized)
	}
}

func TestRequireInternalAPIKey_AllowsConfiguredInternalName(t *testing.T) {
	t.Parallel()

	app := fiber.New()
	app.Use(mustInternalAuth(t, InternalAPIKeyConfig{
		ExpectedAPIKey:      "secret-key",
		AllowedInternalName: "svc-a",
	}))
	app.Get("/", func(c fiber.Ctx) error { return c.SendString("ok") })

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-INTERNAL-NAME", "svc-a")
	req.Header.Set("X-API-KEY", "secret-key")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test() error = %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
}

func TestRequireInternalAPIKey_ReturnsErrorWhenExpectedAPIKeyMissing(t *testing.T) {
	t.Parallel()

	handler, err := RequireInternalAPIKey(InternalAPIKeyConfig{})
	if !errors.Is(err, ErrMissingExpectedAPIKey) {
		t.Fatalf("error = %v, want %v", err, ErrMissingExpectedAPIKey)
	}
	if handler != nil {
		t.Fatalf("handler = %v, want nil", handler)
	}
}

func TestRequirePublicAuth_AllowsBearerToken(t *testing.T) {
	t.Parallel()

	validator := &validatorStub{}
	app := fiber.New()
	app.Use(mustPublicAuth(t, PublicAuthConfig{
		Validator: validator,
	}))
	app.Get("/", func(c fiber.Ctx) error { return c.SendString("ok") })

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer abc.def.ghi")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test() error = %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	if validator.lastToken != "abc.def.ghi" {
		t.Fatalf("token = %q, want %q", validator.lastToken, "abc.def.ghi")
	}
}

func TestRequirePublicAuth_AllowsAPIKey(t *testing.T) {
	t.Parallel()

	validator := &validatorStub{}
	app := fiber.New()
	app.Use(mustPublicAuth(t, PublicAuthConfig{
		Validator: validator,
	}))
	app.Get("/", func(c fiber.Ctx) error { return c.SendString("ok") })

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-API-KEY", "public-api-key")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test() error = %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	if validator.lastAPIKey != "public-api-key" {
		t.Fatalf("api key = %q, want %q", validator.lastAPIKey, "public-api-key")
	}
}

func TestRequirePublicAuth_UsesAPIKeyWhenBothProvidedAndAPIKeySucceeds(t *testing.T) {
	t.Parallel()

	validator := &validatorStub{tokenErr: errors.New("token should not be used")}
	app := fiber.New()
	app.Use(mustPublicAuth(t, PublicAuthConfig{
		Validator: validator,
	}))
	app.Get("/", func(c fiber.Ctx) error { return c.SendString("ok") })

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-API-KEY", "good-key")
	req.Header.Set("Authorization", "Bearer bad-token")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test() error = %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	if validator.lastToken != "" {
		t.Fatalf("token validator should not be used when API key succeeds, got %q", validator.lastToken)
	}
}

func TestRequirePublicAuth_FallsBackToBearerWhenAPIKeyFails(t *testing.T) {
	t.Parallel()

	validator := &validatorStub{apiKeyErr: errors.New("invalid api key")}
	app := fiber.New()
	app.Use(mustPublicAuth(t, PublicAuthConfig{
		Validator: validator,
	}))
	app.Get("/", func(c fiber.Ctx) error { return c.SendString("ok") })

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-API-KEY", "bad-key")
	req.Header.Set("Authorization", "Bearer good-token")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test() error = %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
}

func TestRequirePublicAuth_RejectsRawAuthorizationWithoutBearerPrefix(t *testing.T) {
	t.Parallel()

	validator := &validatorStub{}
	app := fiber.New()
	app.Use(mustPublicAuth(t, PublicAuthConfig{
		Validator: validator,
	}))
	app.Get("/", func(c fiber.Ctx) error { return c.SendString("ok") })

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "raw-token")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test() error = %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusUnauthorized)
	}
}

func TestRequirePublicAuth_RejectsWhenNoCredentialsProvided(t *testing.T) {
	t.Parallel()

	app := fiber.New()
	app.Use(mustPublicAuth(t, PublicAuthConfig{
		Validator: &validatorStub{},
	}))
	app.Get("/", func(c fiber.Ctx) error { return c.SendString("ok") })

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test() error = %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusUnauthorized)
	}
}

func TestRequirePublicAuth_RejectsWhenBothCredentialsFail(t *testing.T) {
	t.Parallel()

	validator := &validatorStub{
		apiKeyErr: errors.New("invalid api key"),
		tokenErr:  errors.New("invalid token"),
	}
	app := fiber.New()
	app.Use(mustPublicAuth(t, PublicAuthConfig{
		Validator: validator,
	}))
	app.Get("/", func(c fiber.Ctx) error { return c.SendString("ok") })

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-API-KEY", "bad-key")
	req.Header.Set("Authorization", "Bearer bad-token")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test() error = %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusUnauthorized)
	}
}

func TestRequirePublicAuth_UsesValidationErrorMapper(t *testing.T) {
	t.Parallel()

	validator := &validatorStub{
		apiKeyErr: errors.New("invalid api key"),
		tokenErr:  errors.New("expired token"),
	}
	app := fiber.New()
	app.Use(mustPublicAuth(t, PublicAuthConfig{
		Validator: validator,
		OnValidationError: func(c fiber.Ctx, err error) error {
			return c.Status(http.StatusForbidden).SendString(err.Error())
		},
	}))
	app.Get("/", func(c fiber.Ctx) error { return c.SendString("ok") })

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-API-KEY", "bad-key")
	req.Header.Set("Authorization", "Bearer bad-token")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test() error = %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusForbidden)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "invalid api key") || !strings.Contains(string(body), "expired token") {
		t.Fatalf("expected combined errors in body, got %q", string(body))
	}
}

func TestRequirePublicAuth_ReturnsErrorWhenValidatorMissing(t *testing.T) {
	t.Parallel()

	handler, err := RequirePublicAuth(PublicAuthConfig{})
	if !errors.Is(err, ErrMissingValidator) {
		t.Fatalf("error = %v, want %v", err, ErrMissingValidator)
	}
	if handler != nil {
		t.Fatalf("handler = %v, want nil", handler)
	}
}

func TestRequireInternalAPIKey_UsesCustomHeaderNames(t *testing.T) {
	t.Parallel()

	app := fiber.New()
	app.Use(mustInternalAuth(t, InternalAPIKeyConfig{
		InternalNameHeader: "X-SERVICE-NAME",
		APIKeyHeader:       "X-SVC-KEY",
		ExpectedAPIKey:     "secret-key",
	}))
	app.Get("/", func(c fiber.Ctx) error { return c.SendString("ok") })

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-SERVICE-NAME", "svc-a")
	req.Header.Set("X-SVC-KEY", "secret-key")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test() error = %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
}

func TestRequirePublicAuth_UsesCustomAuthorizationHeaderAndBearerPrefix(t *testing.T) {
	t.Parallel()

	validator := &validatorStub{}
	app := fiber.New()
	app.Use(mustPublicAuth(t, PublicAuthConfig{
		AuthorizationHeader: "X-AUTH",
		BearerPrefix:        "Token ",
		Validator:           validator,
	}))
	app.Get("/", func(c fiber.Ctx) error { return c.SendString("ok") })

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-AUTH", "Token abc.def.ghi")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test() error = %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	if validator.lastToken != "abc.def.ghi" {
		t.Fatalf("token = %q, want %q", validator.lastToken, "abc.def.ghi")
	}
}

func TestRequirePublicAuth_UsesCustomAPIKeyHeader(t *testing.T) {
	t.Parallel()

	validator := &validatorStub{}
	app := fiber.New()
	app.Use(mustPublicAuth(t, PublicAuthConfig{
		APIKeyHeader: "X-PUBLIC-KEY",
		Validator:    validator,
	}))
	app.Get("/", func(c fiber.Ctx) error { return c.SendString("ok") })

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-PUBLIC-KEY", "public-api-key")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test() error = %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	if validator.lastAPIKey != "public-api-key" {
		t.Fatalf("api key = %q, want %q", validator.lastAPIKey, "public-api-key")
	}
}

func TestRequirePublicAuth_ValidationTimeoutCancelsValidationCall(t *testing.T) {
	t.Parallel()

	validator := &validatorStub{
		validateAPIKeyFn: func(ctx context.Context, _ string) error {
			<-ctx.Done()
			return ctx.Err()
		},
	}
	app := fiber.New()
	app.Use(mustPublicAuth(t, PublicAuthConfig{
		APIKeyHeader:      "X-API-KEY",
		ValidationTimeout: 20 * time.Millisecond,
		Validator:         validator,
		OnValidationError: func(c fiber.Ctx, err error) error { return c.Status(http.StatusGatewayTimeout).SendString(err.Error()) },
	}))
	app.Get("/", func(c fiber.Ctx) error { return c.SendString("ok") })

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-API-KEY", "slow-key")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test() error = %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusGatewayTimeout {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusGatewayTimeout)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), context.DeadlineExceeded.Error()) {
		t.Fatalf("expected deadline exceeded in body, got %q", string(body))
	}
}
