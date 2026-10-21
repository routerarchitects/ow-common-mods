package analytics

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/routerarchitects/ow-common-mods/servicediscovery"
	"github.com/routerarchitects/ow-common-mods/servicerpc/common"
)

// UserScopedAnalyticsClient defines the client interface for user-authorized OWANALYTICS operations.
type UserScopedAnalyticsClient interface {
	GetGatewayFreeMemory(
		ctx context.Context,
		routerID, timestampTill string,
		lookbackHours int,
		authorization string,
	) (any, error)

	GetGatewayWifiTemp(
		ctx context.Context,
		routerID, timestampTill string,
		lookbackHours int,
		authorization string,
	) (any, error)

	GetDeviceBandwidthConsumption(
		ctx context.Context,
		routerID, timestampTill string,
		lookbackHours int,
		authorization string,
	) (any, error)

	GetDeviceRssiQuality(
		ctx context.Context,
		routerID, timestampTill string,
		lookbackHours int,
		authorization string,
	) (any, error)

	GetGatewayOfflineCount(
		ctx context.Context,
		routerID, timestampTill string,
		lookbackHours int,
		authorization string,
	) (any, error)
}

// APIClientError represents an error during an outbound API call to OWANALYTICS.
type APIClientError struct {
	Operation    string
	StatusCode   *int
	ResponseBody any
	Err          error
}

// ApiClientError is an alias for APIClientError to ensure naming compatibility.
type ApiClientError = APIClientError

func (e *APIClientError) Error() string {
	if e == nil {
		return ""
	}
	if e.StatusCode != nil {
		return fmt.Sprintf("Outbound API operation failed: %s status=%d", e.Operation, *e.StatusCode)
	}
	if e.Err != nil {
		return fmt.Sprintf("Outbound API operation failed: %s: %v", e.Operation, e.Err)
	}
	return fmt.Sprintf("Outbound API operation failed: %s", e.Operation)
}

func (e *APIClientError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

// Sleeper is a function that sleeps for a duration while respecting context cancellation.
type Sleeper func(ctx context.Context, d time.Duration) error

// JitterFunc computes jitter for a given base delay.
type JitterFunc func(base float64) float64

// ClockFunc returns the current time (injectable for testing).
type ClockFunc func() time.Time

// UserScopedConfig encapsulates configuration for UserScopedAnalyticsClient.
type UserScopedConfig struct {
	// BaseURL specifies an explicit base URL (e.g. "https://127.0.0.1:16009") which takes precedence over discovery.
	BaseURL string

	// Discovery optionally provides service discovery for discovery-enabled mode.
	Discovery *servicediscovery.Discovery

	// Resolver optionally provides custom service resolver for DI / testing.
	Resolver common.ServiceResolver

	// Timeout specifies per-attempt HTTP timeout (default 5s if <= 0).
	Timeout time.Duration

	// MaxRetries specifies maximum number of retries for retryable failures (default 2 if < 0).
	MaxRetries int

	// MaxRetryDelay specifies maximum delay for exponential backoff or Retry-After (default 5s if <= 0).
	MaxRetryDelay time.Duration

	// InsecureSkipVerify controls TLS certificate verification.
	InsecureSkipVerify bool

	// TLSRootCA is an optional path to PEM certificate pool.
	TLSRootCA string

	// Logger is an optional logger instance.
	Logger *slog.Logger

	// HTTPClient optionally overrides the internal http.Client (e.g. for testing with httptest).
	HTTPClient *http.Client
}

// FormatAnalyticsErrorMessage formats an APIClientError for user-facing MCP error messages,
// redacting sensitive fields.
func FormatAnalyticsErrorMessage(apiErr *APIClientError) string {
	if apiErr == nil {
		return ""
	}
	if apiErr.StatusCode == nil {
		return "OWANALYTICS request failed: service unavailable"
	}

	parts := []string{fmt.Sprintf("OWANALYTICS request failed: status=%d", *apiErr.StatusCode)}

	if m, ok := apiErr.ResponseBody.(map[string]any); ok {
		if errVal, exists := m["error"]; exists && errVal != nil {
			parts = append(parts, fmt.Sprintf("error=%v", errVal))
		}
		if msgVal, exists := m["message"]; exists && msgVal != nil {
			parts = append(parts, fmt.Sprintf("message=%q", msgVal))
		}

		// Collect other details while redacting sensitive fields
		var detailKeys []string
		for k := range m {
			if k != "error" && k != "message" {
				detailKeys = append(detailKeys, k)
			}
		}

		if len(detailKeys) > 0 {
			sort.Strings(detailKeys)
			var detailParts []string
			for _, k := range detailKeys {
				val := m[k]
				if isSensitiveKey(k) {
					val = "******"
				}
				detailParts = append(detailParts, fmt.Sprintf("'%s': %v", k, val))
			}
			parts = append(parts, fmt.Sprintf("details={%s}", strings.Join(detailParts, ", ")))
		}
	} else if apiErr.ResponseBody != nil {
		bodyStr := fmt.Sprintf("%v", apiErr.ResponseBody)
		if strings.TrimSpace(bodyStr) != "" {
			parts = append(parts, fmt.Sprintf("body=%s", bodyStr))
		}
	}

	return strings.Join(parts, " ")
}

func isSensitiveKey(k string) bool {
	switch strings.ToLower(strings.TrimSpace(k)) {
	case "authorization", "token", "access_token", "refresh_token", "password", "passwd", "secret", "api_key", "x-api-key", "cookie", "set-cookie":
		return true
	default:
		return false
	}
}

type TimepointsData struct {
	ID         string     `json:"id"`
	BoardID    string     `json:"boardId"`
	Timestamp  int64      `json:"timestamp"`
	Serial     string     `json:"serialNumber"`
	DeviceInfo DeviceInfo `json:"device_info"`
	SSIDData   []SSIDData `json:"ssid_data"`
}

type DeviceInfo struct {
	Associations2G int     `json:"associations_2g"`
	Associations5G int     `json:"associations_5g"`
	Associations6G int     `json:"associations_6g"`
	BoardID        string  `json:"boardId"`
	Connected      bool    `json:"connected"`
	ConnectionIP   string  `json:"connectionIp"`
	DeviceType     string  `json:"deviceType"`
	Health         int     `json:"health"`
	LastConnection int64   `json:"lastConnection"`
	LastContact    int64   `json:"lastContact"`
	LastDisconn    int64   `json:"lastDisconnection"`
	LastFirmware   string  `json:"lastFirmware"`
	LastFWUpdate   int64   `json:"lastFirmwareUpdate"`
	LastHealth     int64   `json:"lastHealth"`
	LastPing       int64   `json:"lastPing"`
	LastState      int64   `json:"lastState"`
	Locale         string  `json:"locale"`
	Memory         float64 `json:"memory"`
	Pings          int     `json:"pings"`
	SerialNumber   string  `json:"serialNumber"`
	States         int     `json:"states"`
	Type           string  `json:"type"`
	Uptime         int64   `json:"uptime"`
}

type SSIDData struct {
	Associations  []SSIDAssociation `json:"associations"`
	Band          int               `json:"band"`
	BSSID         string            `json:"bssid"`
	Channel       int               `json:"channel"`
	Mode          string            `json:"mode"`
	SSID          string            `json:"ssid"`
	RxBytesBW     *SSIDMetric       `json:"rx_bytes_bw,omitempty"`
	RxPacketsBW   *SSIDMetric       `json:"rx_packets_bw,omitempty"`
	TxBytesBW     *SSIDMetric       `json:"tx_bytes_bw,omitempty"`
	TxDurationPct *SSIDMetric       `json:"tx_duration_pct,omitempty"`
	TxFailedPct   *SSIDMetric       `json:"tx_failed_pct,omitempty"`
	TxPacketsBW   *SSIDMetric       `json:"tx_packets_bw,omitempty"`
	TxRetriesPct  *SSIDMetric       `json:"tx_retries_pct,omitempty"`
}

type SSIDMetric struct {
	Avg float64 `json:"avg"`
	Max float64 `json:"max"`
	Min float64 `json:"min"`
}

type SSIDAssociation struct {
	Connected   int             `json:"connected"`
	Inactive    int             `json:"inactive"`
	RSSI        int             `json:"rssi"`
	Station     string          `json:"station"`
	RxRate      AssociationRate `json:"rx_rate"`
	TxRate      AssociationRate `json:"tx_rate"`
	Fingerprint map[string]any  `json:"fingerprint,omitempty"`
}

type AssociationRate struct {
	Bitrate int `json:"bitrate"`
	Chwidth int `json:"chwidth"`
}

type TimepointRequest struct {
	BoardID         string `json:"boardId"`
	FromDate        uint64 `json:"fromDate,omitempty"`
	EndDate         uint64 `json:"endDate,omitempty"`
	MaxRecords      int    `json:"maxRecords,omitempty"`
	StatsOnly       *bool  `json:"statsOnly,omitempty"`
	PointsOnly      *bool  `json:"pointsOnly,omitempty"`
	PointStatsOnly  *bool  `json:"pointStatsOnly,omitempty"`
	LatestPerDevice *bool  `json:"LatestPerDevice,omitempty"`
}
