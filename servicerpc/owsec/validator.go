package owsec

import (
	"context"
	"fmt"
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

// ValidateToken validates token by checking both subscription-token and
// regular-token endpoints.
//
// Caller must pass ctx with timeout/deadline.
func (s *SecurityClient) ValidateToken(ctx context.Context, rawToken string) error {
	token := strings.TrimSpace(rawToken)

	subResp, subErr := s.deps.Send(ctx, http.MethodGet, "/api/v1/validateSubToken?token="+url.QueryEscape(token), nil, serviceName)
	if subResp != nil {
		defer subResp.Close()
	}

	if subErr != nil {
		return subErr
	}

	if subResp != nil && subResp.StatusCode() == http.StatusOK {
		return nil
	}

	tokenResp, tokenErr := s.deps.Send(ctx, http.MethodGet, "/api/v1/validateToken?token="+url.QueryEscape(token), nil, serviceName)
	if tokenResp != nil {
		defer tokenResp.Close()
	}

	if tokenErr != nil {
		s.deps.Logger().With("service", serviceName, "operation", "validateToken").Error("validation request failed")
		return tokenErr
	}

	if tokenResp == nil {
		return apperror.New(apperror.CodeInternal, "token validation response is empty")
	}

	if tokenResp.StatusCode() == http.StatusOK {
		return nil
	}

	subStatus := http.StatusInternalServerError
	if subResp != nil {
		subStatus = subResp.StatusCode()
	}

	if isInvalidTokenStatus(subStatus) && isInvalidTokenStatus(tokenResp.StatusCode()) {
		return apperror.New(apperror.CodeUnauthorized, "unauthorized")
	}

	return apperror.New(apperror.CodeInternal, fmt.Sprintf("token validation failed (subtoken=%d token=%d)", subStatus, tokenResp.StatusCode()))
}

func isInvalidTokenStatus(status int) bool {
	return status == http.StatusUnauthorized || status == http.StatusForbidden || status == http.StatusNotFound
}
