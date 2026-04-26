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

func NewSecurityClient(deps *common.ServiceRPCBase) (*SecurityClient, error) {
	if deps == nil {
		return nil, apperror.New(apperror.CodeInternal, "dependencies cannot be nil")
	}
	return &SecurityClient{
		deps: deps,
	}, nil
}

// ValidateToken validates token by checking both subscription-token and
// regular-token endpoints.
//
// Caller must pass ctx with timeout/deadline.
func (s *SecurityClient) ValidateToken(ctx context.Context, rawToken string) error {
	token := strings.TrimSpace(rawToken)
	if token == "" {
		return apperror.New(apperror.CodeUnauthorized, "unauthorized")
	}

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

func (s *SecurityClient) ValidateAPIKey(ctx context.Context, apiKey string) error {
	trimmedAPIKey := strings.TrimSpace(apiKey)
	if trimmedAPIKey == "" {
		return apperror.New(apperror.CodeUnauthorized, "unauthorized")
	}

	resp, err := s.deps.Send(ctx, http.MethodGet, "/api/v1/validateAPIKey?apiKey="+url.QueryEscape(trimmedAPIKey), nil, serviceName)
	if resp != nil {
		defer resp.Close()
	}

	if err != nil {
		s.deps.Logger().With("service", serviceName, "operation", "validateAPIKey").Error("validation request failed")
		return err
	}

	if resp == nil {
		s.deps.Logger().With("service", serviceName, "operation", "validateAPIKey").Error("validation response is empty")
		return apperror.New(apperror.CodeInternal, "api key validation response is empty")
	}

	if resp.StatusCode() == http.StatusOK {
		return nil
	}

	if isInvalidTokenStatus(resp.StatusCode()) {
		s.deps.Logger().With("service", serviceName, "operation", "validateAPIKey", "status", resp.StatusCode()).Error("api key validation failed")
		return apperror.New(apperror.CodeUnauthorized, "unauthorized")
	}

	s.deps.Logger().With("service", serviceName, "operation", "validateAPIKey", "status", resp.StatusCode()).Error("api key validation failed")
	return apperror.New(apperror.CodeInternal, fmt.Sprintf("api key validation failed (status=%d)", resp.StatusCode()))
}
