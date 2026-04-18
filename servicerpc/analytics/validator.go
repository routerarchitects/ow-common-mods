package analytics

import (
	"strings"

	"github.com/routerarchitects/ra-common-mods/apperror"
)

func validateTimepointRequest(req TimepointRequest) error {
	if strings.TrimSpace(req.BoardID) == "" {
		return apperror.New(apperror.CodeInvalidInput, "boardId is required")
	}
	if req.FromDate == 0 {
		return apperror.New(apperror.CodeInvalidInput, "fromDate is required")
	}
	if req.EndDate == 0 {
		return apperror.New(apperror.CodeInvalidInput, "endDate is required")
	}
	if req.EndDate < req.FromDate {
		return apperror.New(apperror.CodeInvalidInput, "endDate must be greater than or equal to fromDate")
	}
	if req.MaxRecords <= 0 {
		return apperror.New(apperror.CodeInvalidInput, "maxRecords must be greater than 0")
	}
	return nil
}
