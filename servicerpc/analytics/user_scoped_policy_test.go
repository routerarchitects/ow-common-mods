package analytics_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/routerarchitects/ow-common-mods/servicerpc/analytics"
	"github.com/routerarchitects/ra-common-mods/apperror"
)

func TestUserScopedClient_RetryPolicy(t *testing.T) {
	for _, tc := range []struct {
		name       string
		configured int
		opts       []analytics.UserScopedOption
		attempts   int32
	}{
		{name: "zero value defaults to two retries", attempts: 3},
		{name: "negative config retains default", configured: -1, attempts: 3},
		{name: "configured count", configured: 1, attempts: 2},
		{name: "disabled explicitly", opts: []analytics.UserScopedOption{analytics.WithMaxRetries(0)}, attempts: 1},
		{name: "negative option disables retries", opts: []analytics.UserScopedOption{analytics.WithMaxRetries(-1)}, attempts: 1},
		{name: "option overrides config", configured: 1, opts: []analytics.UserScopedOption{analytics.WithMaxRetries(3)}, attempts: 4},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var attempts atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				attempts.Add(1)
				w.WriteHeader(http.StatusServiceUnavailable)
			}))
			defer server.Close()
			opts := append([]analytics.UserScopedOption{
				analytics.WithHTTPClient(server.Client()),
				analytics.WithSleeper(func(context.Context, time.Duration) error { return nil }),
			}, tc.opts...)
			client, err := analytics.NewUserScopedClient(analytics.UserScopedConfig{
				BaseURL: server.URL, MaxRetries: tc.configured,
			}, opts...)
			if err != nil {
				t.Fatalf("create client: %v", err)
			}
			if _, err := client.GetGatewayFreeMemory(context.Background(), "gw-1", "2026-10-21T10:00:00Z", 1, "Bearer tok"); err == nil {
				t.Fatal("expected error after retry exhaustion")
			}
			if got := attempts.Load(); got != tc.attempts {
				t.Errorf("HTTP attempts = %d, want %d", got, tc.attempts)
			}
		})
	}
}

func TestUserScopedClient_SummaryInputValidation(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
	defer server.Close()
	client, err := analytics.NewUserScopedClient(analytics.UserScopedConfig{
		BaseURL: server.URL, HTTPClient: server.Client(),
	})
	if err != nil {
		t.Fatalf("create client: %v", err)
	}
	for _, method := range []struct {
		name string
		call func(context.Context, string, string, int, string) (any, error)
	}{
		{"memory", client.GetGatewayFreeMemory},
		{"temperature", client.GetGatewayWifiTemp},
		{"bandwidth", client.GetDeviceBandwidthConsumption},
		{"RSSI", client.GetDeviceRssiQuality},
		{"availability", client.GetGatewayOfflineCount},
	} {
		t.Run(method.name, func(t *testing.T) {
			for _, tc := range []struct {
				name      string
				routerID  string
				timestamp string
				lookback  int
				token     string
			}{
				{"empty router", " ", "2026-10-21T10:00:00Z", 1, "Bearer tok"},
				{"empty authorization", "gw-1", "2026-10-21T10:00:00Z", 1, " "},
				{"empty timestamp", "gw-1", "", 1, "Bearer tok"},
				{"blank timestamp", "gw-1", " ", 1, "Bearer tok"},
				{"malformed timestamp", "gw-1", "not-a-timestamp", 1, "Bearer tok"},
				{"missing timezone", "gw-1", "2026-10-21T10:00:00", 1, "Bearer tok"},
				{"invalid calendar date", "gw-1", "2026-02-30T10:00:00Z", 1, "Bearer tok"},
				{"zero lookback", "gw-1", "2026-10-21T10:00:00Z", 0, "Bearer tok"},
				{"negative lookback", "gw-1", "2026-10-21T10:00:00Z", -1, "Bearer tok"},
			} {
				t.Run(tc.name, func(t *testing.T) {
					before := requests.Load()
					_, err := method.call(context.Background(), tc.routerID, tc.timestamp, tc.lookback, tc.token)
					if got := apperror.CodeOf(err); got != apperror.CodeInvalidInput {
						t.Errorf("error code = %s, want invalid input (error: %v)", got, err)
					}
					if requests.Load() != before {
						t.Error("invalid input reached the upstream server")
					}
				})
			}
		})
	}
}

func TestUserScopedClient_ValidTimestampFormats(t *testing.T) {
	for _, timestamp := range []string{
		"2026-10-21T10:00:00Z",
		"2026-10-21T15:30:00+05:30",
		"2026-10-21T10:00:00.123456789Z",
	} {
		t.Run(timestamp, func(t *testing.T) {
			var requests atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				if got := r.URL.Query().Get("timestampTill"); got != timestamp {
					t.Errorf("timestampTill = %q, want %q", got, timestamp)
				}
				if got := r.URL.Query().Get("lookbackHours"); got != "1" {
					t.Errorf("lookbackHours = %q, want 1", got)
				}
				_, _ = w.Write([]byte(`{}`))
			}))
			defer server.Close()
			client, err := analytics.NewUserScopedClient(analytics.UserScopedConfig{
				BaseURL: server.URL, HTTPClient: server.Client(),
			})
			if err != nil {
				t.Fatalf("create client: %v", err)
			}
			if _, err := client.GetGatewayFreeMemory(context.Background(), "gw-1", timestamp, 1, "Bearer tok"); err != nil {
				t.Fatalf("valid request rejected: %v", err)
			}
			if got := requests.Load(); got != 1 {
				t.Errorf("HTTP requests = %d, want 1", got)
			}
		})
	}
}
