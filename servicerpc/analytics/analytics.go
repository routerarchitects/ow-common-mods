package analytics

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/routerarchitects/ow-common-mods/servicerpc/common"
	"github.com/routerarchitects/ra-common-mods/apperror"
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

// GetTimepoints fetches analytics timepoints.
//
// Caller must pass ctx with timeout/deadline.
func (v *AnalyticsClient) GetTimepoints(ctx context.Context, req TimepointRequest) ([]TimepointsData, error) {
	if err := validateTimepointRequest(req); err != nil {
		return nil, err
	}

	endpoint := "/api/v1/board/" + url.PathEscape(strings.TrimSpace(req.BoardID)) + "/timepoints"

	q := url.Values{}
	q.Set("fromDate", strconv.FormatUint(req.FromDate, 10))
	q.Set("endDate", strconv.FormatUint(req.EndDate, 10))
	q.Set("maxRecords", strconv.Itoa(req.MaxRecords))
	q.Set("statsOnly", strconv.FormatBool(boolOrDefault(req.StatsOnly, false)))
	q.Set("pointsOnly", strconv.FormatBool(boolOrDefault(req.PointsOnly, true)))
	q.Set("pointStatsOnly", strconv.FormatBool(boolOrDefault(req.PointStatsOnly, true)))
	q.Set("LatestPerDevice", strconv.FormatBool(boolOrDefault(req.LatestPerDevice, true)))

	fullURL := endpoint + "?" + q.Encode()

	v.deps.Logger().With("url", fullURL).Info("getting timepoints")

	start := time.Now()
	resp, err := v.deps.Send(ctx, http.MethodGet, fullURL, nil, serviceName)
	if err != nil {
		return nil, err
	}
	defer resp.Close()

	if resp.StatusCode() == http.StatusNotFound {
		v.deps.Logger().With("status", resp.StatusCode()).Error("timepoints not found")
		return nil, apperror.New(apperror.CodeNotFound, "resource does not exist")
	}

	if resp.StatusCode() != http.StatusOK {
		return nil, apperror.Wrap(apperror.CodeInternal, fmt.Sprintf("failed to get Timepoints: %d", resp.StatusCode()), nil)
	}

	type timepointResponse struct {
		Points [][]TimepointsData `json:"points"`
	}

	var tpResp timepointResponse
	if err := json.Unmarshal(resp.Body(), &tpResp); err != nil {
		return nil, apperror.Wrap(apperror.CodeInternal, "failed to parse timepoints response", err)
	}

	var timepoints []TimepointsData
	for _, bucket := range tpResp.Points {
		if len(bucket) == 0 {
			continue
		}
		timepoints = append(timepoints, bucket...)
	}

	v.deps.Logger().With(
		"records", len(timepoints),
		"status", resp.StatusCode(),
		"duration_ms", time.Since(start).Milliseconds(),
	).Info("received timepoints response")

	return timepoints, nil
}

func boolOrDefault(v *bool, defaultValue bool) bool {
	if v == nil {
		return defaultValue
	}
	return *v
}

// GetDeviceInfo fetches device details for a board.
//
// Caller must pass ctx with timeout/deadline.
func (v *AnalyticsClient) GetDeviceInfo(ctx context.Context, boardID string) ([]DeviceInfo, error) {
	resp, err := v.deps.Send(ctx, http.MethodGet, "/api/v1/board/"+boardID+"/devices", nil, serviceName)
	if err != nil {
		return nil, err
	}
	defer resp.Close()

	if resp.StatusCode() == http.StatusNotFound {
		return nil, apperror.New(apperror.CodeNotFound, "resource does not exist")
	}

	if resp.StatusCode() != http.StatusOK {
		return nil, apperror.Wrap(apperror.CodeInternal, fmt.Sprintf("failed to get device info : %d", resp.StatusCode()), nil)
	}

	type deviceInfoResponse struct {
		Devices []DeviceInfo `json:"devices"`
	}

	var payload deviceInfoResponse
	if err := json.Unmarshal(resp.Body(), &payload); err != nil {
		return nil, apperror.Wrap(apperror.CodeInternal, "failed to parse device info response", err)
	}

	return payload.Devices, nil
}

// GetWifiClientHistoryMACs fetches paginated MAC history for a board.
//
// Caller must pass ctx with timeout/deadline.
func (v *AnalyticsClient) GetWifiClientHistoryMACs(ctx context.Context, boardID string, limit, offset int) ([]string, error) {
	fullURL := "/api/v1/wifiClientHistory" +
		"?macsOnly=true" +
		"&boardId=" + url.QueryEscape(strings.TrimSpace(boardID)) +
		"&limit=" + strconv.Itoa(limit) +
		"&offset=" + strconv.Itoa(offset)

	resp, err := v.deps.Send(ctx, http.MethodGet, fullURL, nil, serviceName)
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeInternal, "failed to get wifi client history MACs", err)
	}
	defer resp.Close()

	if resp.StatusCode() == http.StatusNotFound {
		return nil, apperror.New(apperror.CodeNotFound, "resource does not exist")
	}

	if resp.StatusCode() != http.StatusOK {
		return nil, apperror.Wrap(apperror.CodeInternal, fmt.Sprintf("failed to get wifi client history MACs: %d", resp.StatusCode()), nil)
	}

	type wifiClientHistoryResponse struct {
		Entries []string `json:"entries"`
	}
	var out wifiClientHistoryResponse
	if err := json.Unmarshal(resp.Body(), &out); err != nil {
		return nil, apperror.Wrap(apperror.CodeInternal, "failed to parse wifiClientHistory response", err)
	}
	return out.Entries, nil
}
