package owsec

import (
	"context"
	"net/http"
	"net/url"
	"strings"

	"github.com/routerarchitects/ow-common-mods/servicerpc/common"
	"github.com/routerarchitects/ra-common-mods/apperror"
)

const serviceName = "owsec"

type SecurityClient struct {
	deps *common.ServiceRPCBase
}

func NewSecurityClient(deps *common.ServiceRPCBase) *SecurityClient {
	return &SecurityClient{
		deps: deps,
	}
}

// ValidateToken validates subscription/token with fallback endpoint support.
//
// Caller must pass ctx with timeout/deadline.
func (s *SecurityClient) ValidateToken(ctx context.Context, rawToken string) error {
	token := strings.TrimSpace(rawToken)

	resp, err := s.deps.Send(ctx, http.MethodGet, "/api/v1/validateSubToken?token="+url.QueryEscape(token), nil, serviceName)
	if resp != nil {
		defer resp.Close()
	}

	if err == nil && resp != nil && resp.StatusCode() == http.StatusOK {
		return nil
	}

	fallbackResp, fallbackErr := s.deps.Send(ctx, http.MethodGet, "/api/v1/validateToken?token="+url.QueryEscape(token), nil, serviceName)
	if fallbackResp != nil {
		defer fallbackResp.Close()
	}

	if fallbackErr != nil {
		s.deps.Logger().With("service", serviceName, "operation", "validateToken").Error("validation request failed")
		return apperror.Wrap(apperror.CodeUnauthorized, "unauthorized", fallbackErr)
	}

	if fallbackResp == nil || fallbackResp.StatusCode() != http.StatusOK {
		return apperror.New(apperror.CodeUnauthorized, "unauthorized")
	}

	return nil
}
