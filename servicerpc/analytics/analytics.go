package analytics

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/routerarchitects/ow-common-mods/apperrors"
	"github.com/routerarchitects/ow-common-mods/servicerpc/common"
)

const serviceName = "owanalytics"

type AnalyticsClient struct {
	deps *common.ServiceRPCBase
}

func NewAnalyticsClient(deps *common.ServiceRPCBase) *AnalyticsClient {
	return &AnalyticsClient{
		deps: deps,
	}
}

func (v *AnalyticsClient) GetTimepoints(req TimepointRequest) ([]TimepointsData, error) {
	fullURL := "/api/v1/board/" + req.BoardID + "/timepoints?"

	fullURL += "fromDate=" + strconv.FormatUint(req.FromDate, 10) + "&"
	fullURL += "endDate=" + strconv.FormatUint(req.EndDate, 10) + "&"
	fullURL += "maxRecords=" + strconv.Itoa(req.MaxRecords) + "&"

	fullURL += "statsOnly=false&pointsOnly=true&pointStatsOnly=true&LatestPerDevice=true"

	v.deps.Logger.With("url", fullURL).Info("getting timepoints")

	start := time.Now()
	resp, err := v.deps.Send(context.Background(), fiber.MethodGet, fullURL, nil, serviceName)
	if err != nil {
		return nil, apperrors.Wrap(apperrors.CodeInternal, "failed to get timepoints", err)
	}
	defer resp.Close()

	if resp.StatusCode() == fiber.StatusNotFound {
		v.deps.Logger.With("status", resp.StatusCode()).Error("timepoints not found")
		err := apperrors.New(apperrors.CodeNotFound, "")
		info := apperrors.InfoOf(err)
		return nil, apperrors.Wrap(apperrors.CodeNotFound, info.Description, nil)
	}

	if resp.StatusCode() != fiber.StatusOK {
		return nil, apperrors.Wrap(apperrors.CodeInternal, fmt.Sprintf("failed to get Timepoints: %d", resp.StatusCode()), nil)
	}

	type timepointResponse struct {
		Points [][]TimepointsData `json:"points"`
	}

	var tpResp timepointResponse
	if err := json.Unmarshal(resp.Body(), &tpResp); err != nil {
		return nil, apperrors.Wrap(apperrors.CodeInternal, "failed to parse timepoints response", err)
	}

	var timepoints []TimepointsData
	for _, bucket := range tpResp.Points {
		if len(bucket) == 0 {
			continue
		}
		timepoints = append(timepoints, bucket...)
	}

	v.deps.Logger.With(
		"records", len(timepoints),
		"status", resp.StatusCode(),
		"duration_ms", time.Since(start).Milliseconds(),
	).Info("received timepoints response")

	return timepoints, nil
}

func (v *AnalyticsClient) GetDeviceInfo(boardID string) ([]DeviceInfo, error) {
	resp, err := v.deps.Send(context.Background(), fiber.MethodGet, "/api/v1/board/"+boardID+"/devices", nil, serviceName)
	if err != nil {
		return nil, apperrors.Wrap(apperrors.CodeInternal, "failed to get device info", err)
	}
	defer resp.Close()

	if resp.StatusCode() == fiber.StatusNotFound {
		err := apperrors.New(apperrors.CodeNotFound, "")
		info := apperrors.InfoOf(err)
		return nil, apperrors.Wrap(apperrors.CodeNotFound, info.Description, nil)
	}

	if resp.StatusCode() != fiber.StatusOK {
		return nil, apperrors.Wrap(apperrors.CodeInternal, fmt.Sprintf("failed to get device info : %d", resp.StatusCode()), nil)
	}

	type deviceInfoResponse struct {
		Devices []DeviceInfo `json:"devices"`
	}

	var payload deviceInfoResponse
	if err := json.Unmarshal(resp.Body(), &payload); err != nil {
		return nil, apperrors.Wrap(apperrors.CodeInternal, "failed to parse device info response", err)
	}

	return payload.Devices, nil
}

func (v *AnalyticsClient) GetWifiClientHistoryMACs(boardId string, limit, offset int) ([]string, error) {
	fullURL := "/api/v1/wifiClientHistory" +
		"?macsOnly=true" +
		"&boardId=" + url.QueryEscape(strings.TrimSpace(boardId)) +
		"&limit=" + strconv.Itoa(limit) +
		"&offset=" + strconv.Itoa(offset)

	resp, err := v.deps.Send(context.Background(), fiber.MethodGet, fullURL, nil, serviceName)
	if err != nil {
		return nil, apperrors.Wrap(apperrors.CodeInternal, "failed to get wifi client history MACs", err)
	}
	defer resp.Close()

	if resp.StatusCode() == fiber.StatusNotFound {
		err := apperrors.New(apperrors.CodeNotFound, "")
		info := apperrors.InfoOf(err)
		return nil, apperrors.Wrap(apperrors.CodeNotFound, info.Description, nil)
	}

	if resp.StatusCode() != fiber.StatusOK {
		return nil, apperrors.Wrap(apperrors.CodeInternal, fmt.Sprintf("failed to get wifi client history MACs: %d", resp.StatusCode()), nil)
	}

	type wifiClientHistoryResponse struct {
		Entries []string `json:"entries"`
	}
	var out wifiClientHistoryResponse
	if err := json.Unmarshal(resp.Body(), &out); err != nil {
		return nil, apperrors.Wrap(apperrors.CodeInternal, "failed to parse wifiClientHistory response", err)
	}
	return out.Entries, nil
}
