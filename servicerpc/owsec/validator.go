package owsec

import (
	"context"
	"net/url"
	"strings"

	"github.com/gofiber/fiber/v3"
	"github.com/routerarchitects/ow-common-mods/apperrors"
	"github.com/routerarchitects/ow-common-mods/servicerpc/common"
)

const serviceName = "owsec"

type Validator struct {
	deps *common.ServiceRPCBase
}

func NewValidator(deps *common.ServiceRPCBase) *Validator {
	return &Validator{
		deps: deps,
	}
}

func (v *Validator) ValidateToken(rawToken string) error {
	token := strings.TrimSpace(rawToken)

	resp, err := v.deps.Send(context.Background(), fiber.MethodGet, "/api/v1/validateSubToken?token="+url.QueryEscape(token), nil, serviceName)
	if resp != nil {
		defer resp.Close()
	}

	if err == nil && resp != nil && resp.StatusCode() == fiber.StatusOK {
		return nil
	}

	fallbackResp, fallbackErr := v.deps.Send(context.Background(), fiber.MethodGet, "/api/v1/validateToken?token="+url.QueryEscape(token), nil, serviceName)
	if fallbackResp != nil {
		defer fallbackResp.Close()
	}

	if fallbackErr != nil {
		v.deps.Logger.With("service", serviceName, "operation", "validateToken").Error("validation request failed")
		err := apperrors.New(apperrors.CodeUnauthorized, "")
		info := apperrors.InfoOf(err)
		return apperrors.Wrap(apperrors.CodeUnauthorized, info.Description, fallbackErr)
	}

	if fallbackResp == nil || fallbackResp.StatusCode() != fiber.StatusOK {
		err := apperrors.New(apperrors.CodeUnauthorized, "")
		info := apperrors.InfoOf(err)
		return apperrors.Wrap(apperrors.CodeUnauthorized, info.Description, nil)
	}

	return nil
}
