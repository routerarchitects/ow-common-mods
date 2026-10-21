package analytics

import (
	"strings"
	"time"

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

func validateWifiClientHistoryRequest(boardID string, limit, offset int) error {
	if strings.TrimSpace(boardID) == "" {
		return apperror.New(apperror.CodeInvalidInput, "boardId is required")
	}
	if limit <= 0 {
		return apperror.New(apperror.CodeInvalidInput, "limit must be greater than 0")
	}
	if offset < 0 {
		return apperror.New(apperror.CodeInvalidInput, "offset must be greater than or equal to 0")
	}
	return nil
}

func validateDeviceSummaryRequest(routerID, timestampTill string, lookbackHours int, authorization string) error {
	if strings.TrimSpace(routerID) == "" {
		return apperror.New(apperror.CodeInvalidInput, "routerID is required")
	}
	if strings.TrimSpace(authorization) == "" {
		return apperror.New(apperror.CodeInvalidInput, "authorization is required")
	}
	if _, err := time.Parse(time.RFC3339, timestampTill); err != nil {
		return apperror.New(apperror.CodeInvalidInput, "timestampTill must be a valid RFC3339 timestamp")
	}
	if lookbackHours <= 0 {
		return apperror.New(apperror.CodeInvalidInput, "lookbackHours must be greater than 0")
	}
	return nil
}
