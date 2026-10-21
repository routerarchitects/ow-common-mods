package analytics

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

// GetGatewayFreeMemory retrieves memory summary for a gateway router.
func (c *userScopedClient) GetGatewayFreeMemory(
	ctx context.Context,
	routerID, timestampTill string,
	lookbackHours int,
	authorization string,
) (any, error) {
	return c.getDeviceSummary(ctx, routerID, "memory-summary", timestampTill, lookbackHours, authorization, "get_gateway_free_memory")
}

// GetGatewayWifiTemp retrieves radio temperature summary for a gateway router.
func (c *userScopedClient) GetGatewayWifiTemp(
	ctx context.Context,
	routerID, timestampTill string,
	lookbackHours int,
	authorization string,
) (any, error) {
	return c.getDeviceSummary(ctx, routerID, "radio-temperature-summary", timestampTill, lookbackHours, authorization, "get_gateway_wifi_temp")
}

// GetDeviceBandwidthConsumption retrieves bandwidth consumption summary for connected clients.
func (c *userScopedClient) GetDeviceBandwidthConsumption(
	ctx context.Context,
	routerID, timestampTill string,
	lookbackHours int,
	authorization string,
) (any, error) {
	return c.getDeviceSummary(ctx, routerID, "wifi-clients/usage-summary", timestampTill, lookbackHours, authorization, "get_device_bandwidth_consumption")
}

// GetDeviceRssiQuality retrieves RSSI quality summary for connected clients.
func (c *userScopedClient) GetDeviceRssiQuality(
	ctx context.Context,
	routerID, timestampTill string,
	lookbackHours int,
	authorization string,
) (any, error) {
	return c.getDeviceSummary(ctx, routerID, "wifi-clients/rssi-summary", timestampTill, lookbackHours, authorization, "get_device_rssi_quality")
}

// GetGatewayOfflineCount retrieves gateway availability and offline event count summary.
func (c *userScopedClient) GetGatewayOfflineCount(
	ctx context.Context,
	routerID, timestampTill string,
	lookbackHours int,
	authorization string,
) (any, error) {
	return c.getDeviceSummary(ctx, routerID, "availability-summary", timestampTill, lookbackHours, authorization, "get_gateway_offline_count")
}

func (c *userScopedClient) getDeviceSummary(
	ctx context.Context,
	routerID, endpoint, timestampTill string,
	lookbackHours int,
	authorization, operation string,
) (any, error) {
	if err := validateDeviceSummaryRequest(routerID, timestampTill, lookbackHours, authorization); err != nil {
		return nil, err
	}

	safeRouterID := url.PathEscape(strings.TrimSpace(routerID))
	path := fmt.Sprintf("/api/v1/devices/%s/%s", safeRouterID, endpoint)

	params := map[string]string{
		"timestampTill": timestampTill,
		"lookbackHours": strconv.Itoa(lookbackHours),
	}

	return c.getJSON(ctx, path, params, authorization, operation)
}
