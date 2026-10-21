package analytics

import (
	"net/url"
	"strconv"
	"strings"

	"github.com/routerarchitects/ra-common-mods/apperror"
)

// EndpointMode selects how a user-scoped Analytics request locates its target.
type EndpointMode string

const (
	EndpointModeDiscoveryRequired EndpointMode = "discovery-required"
	EndpointModeStandalone        EndpointMode = "standalone"
)

type analyticsTarget struct {
	baseURL      string
	internalKey  string
	internalName string
}

// ValidatePrivateIdentity validates the advertised caller URL used by OpenWiFi's
// X-INTERNAL-NAME header. It is not an application name or a target service URL.
func ValidatePrivateIdentity(identity string) error {
	if _, err := validatePrivateURL(identity); err != nil {
		return apperror.New(apperror.CodeInvalidInput, "internal caller identity must be its advertised public HTTP or HTTPS endpoint")
	}
	return nil
}

func validatePrivateURL(raw string) (*url.URL, error) {
	u, err := validateEndpointURL(raw)
	if err != nil || strings.TrimSpace(raw) != raw || strings.ContainsAny(raw, "\r\n\t ") {
		return nil, apperror.New(apperror.CodeInvalidInput, "invalid private HTTP or HTTPS endpoint")
	}
	if u.RawQuery != "" || u.ForceQuery || u.Hostname() == "" || strings.HasSuffix(u.Host, ":") {
		return nil, apperror.New(apperror.CodeInvalidInput, "invalid private HTTP or HTTPS endpoint")
	}
	if port := u.Port(); port != "" {
		n, err := strconv.Atoi(port)
		if err != nil || n < 1 || n > 65535 {
			return nil, apperror.New(apperror.CodeInvalidInput, "invalid private endpoint port")
		}
	}
	return u, nil
}

func (c *userScopedClient) resolveTarget() (analyticsTarget, error) {
	switch c.cfg.EndpointMode {
	case EndpointModeDiscoveryRequired:
		if c.resolver == nil {
			return analyticsTarget{}, apperror.New(apperror.CodeInternal, "OWANALYTICS service discovery unavailable")
		}
		instance, err := c.resolver.Resolve(serviceName)
		if err != nil || instance == nil {
			// Resolver errors can contain discovery records and their credentials.
			// Preserve only their semantic code, never their message or cause.
			code := apperror.CodeNotFound
			if err != nil {
				code = apperror.CodeOf(err)
				if code == apperror.CodeUnknown {
					code = apperror.CodeInternal
				}
			}
			return analyticsTarget{}, apperror.New(code, "OWANALYTICS service unavailable in discovery")
		}
		if _, err := validatePrivateURL(instance.PrivateEndPoint); err != nil {
			return analyticsTarget{}, apperror.New(apperror.CodeInvalidInput, "OWANALYTICS private endpoint missing or invalid")
		}
		if !validInternalKey(instance.Key) {
			return analyticsTarget{}, apperror.New(apperror.CodeUnauthorized, "OWANALYTICS internal credentials missing or invalid")
		}
		if err := ValidatePrivateIdentity(c.cfg.InternalName); err != nil {
			return analyticsTarget{}, err
		}
		return analyticsTarget{
			baseURL:     strings.TrimRight(instance.PrivateEndPoint, "/"),
			internalKey: instance.Key, internalName: c.cfg.InternalName,
		}, nil
	case EndpointModeStandalone:
		if _, err := validateEndpointURL(c.baseURL); err != nil {
			return analyticsTarget{}, apperror.New(apperror.CodeInvalidInput, "invalid standalone Analytics endpoint")
		}
		return analyticsTarget{baseURL: c.baseURL}, nil
	case "":
		// Compatibility for existing shared-module consumers. New integrations
		// must select one of the explicit modes; strict mode never enters here.
		baseURL, err := c.resolveBaseURL()
		return analyticsTarget{baseURL: baseURL}, err
	default:
		return analyticsTarget{}, apperror.New(apperror.CodeInvalidInput, "invalid Analytics endpoint mode")
	}
}

func validInternalKey(key string) bool {
	if key == "" {
		return false
	}
	for _, value := range key {
		if value < 0x21 || value > 0x7e {
			return false
		}
	}
	return true
}
