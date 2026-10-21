package analytics_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/routerarchitects/ow-common-mods/servicerpc/analytics"
	"github.com/routerarchitects/ow-common-mods/servicerpc/common"
	"github.com/routerarchitects/ra-common-mods/apperror"
)

type mockServiceResolver struct {
	instance *common.ServiceInstance
	err      error
}

func (m *mockServiceResolver) Resolve(serviceName string) (*common.ServiceInstance, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.instance, nil
}

// 1. All five endpoint mappings
func TestUserScopedClient_AllFiveEndpoints(t *testing.T) {
	endpointsHit := make(map[string]bool)
	var mu sync.Mutex

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		endpointsHit[r.URL.Path] = true
		mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer ts.Close()

	client, err := analytics.NewUserScopedClient(
		analytics.UserScopedConfig{BaseURL: ts.URL},
		analytics.WithHTTPClient(ts.Client()),
	)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	ctx := context.Background()
	routerID := "gw-100"
	timeTill := "2026-10-19T10:00:00Z"
	lookback := 24
	token := "Bearer test-token"

	tests := []struct {
		name     string
		call     func() (any, error)
		expected string
	}{
		{
			name: "GetGatewayFreeMemory",
			call: func() (any, error) {
				return client.GetGatewayFreeMemory(ctx, routerID, timeTill, lookback, token)
			},
			expected: "/api/v1/devices/gw-100/memory-summary",
		},
		{
			name: "GetGatewayWifiTemp",
			call: func() (any, error) {
				return client.GetGatewayWifiTemp(ctx, routerID, timeTill, lookback, token)
			},
			expected: "/api/v1/devices/gw-100/radio-temperature-summary",
		},
		{
			name: "GetDeviceBandwidthConsumption",
			call: func() (any, error) {
				return client.GetDeviceBandwidthConsumption(ctx, routerID, timeTill, lookback, token)
			},
			expected: "/api/v1/devices/gw-100/wifi-clients/usage-summary",
		},
		{
			name: "GetDeviceRssiQuality",
			call: func() (any, error) {
				return client.GetDeviceRssiQuality(ctx, routerID, timeTill, lookback, token)
			},
			expected: "/api/v1/devices/gw-100/wifi-clients/rssi-summary",
		},
		{
			name: "GetGatewayOfflineCount",
			call: func() (any, error) {
				return client.GetGatewayOfflineCount(ctx, routerID, timeTill, lookback, token)
			},
			expected: "/api/v1/devices/gw-100/availability-summary",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			res, err := tc.call()
			if err != nil {
				t.Fatalf("call %s failed: %v", tc.name, err)
			}
			if res == nil {
				t.Fatalf("call %s returned nil", tc.name)
			}

			mu.Lock()
			hit := endpointsHit[tc.expected]
			mu.Unlock()
			if !hit {
				t.Errorf("expected endpoint %s was not reached", tc.expected)
			}
		})
	}
}

// 2. Correct query parameters
func TestUserScopedClient_CorrectQueryParameters(t *testing.T) {
	var receivedQuery url.Values

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedQuery = r.URL.Query()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"data":{"value":1}}`))
	}))
	defer ts.Close()

	client, err := analytics.NewUserScopedClient(
		analytics.UserScopedConfig{BaseURL: ts.URL},
		analytics.WithHTTPClient(ts.Client()),
	)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	_, err = client.GetGatewayFreeMemory(context.Background(), "gw-1", "2026-10-19T12:34:56Z", 48, "Bearer tok")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if receivedQuery.Get("timestampTill") != "2026-10-19T12:34:56Z" {
		t.Errorf("expected timestampTill '2026-10-19T12:34:56Z', got %q", receivedQuery.Get("timestampTill"))
	}
	if receivedQuery.Get("lookbackHours") != "48" {
		t.Errorf("expected lookbackHours '48', got %q", receivedQuery.Get("lookbackHours"))
	}
}

// 3. Original Bearer-token forwarding & 4. No internal API-key substitution
func TestUserScopedClient_BearerTokenAndNoInternalAPIKey(t *testing.T) {
	var receivedHeaders http.Header

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedHeaders = r.Header.Clone()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer ts.Close()

	client, err := analytics.NewUserScopedClient(
		analytics.UserScopedConfig{BaseURL: ts.URL},
		analytics.WithHTTPClient(ts.Client()),
	)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	// Test 1: Full Bearer header
	_, err = client.GetGatewayWifiTemp(context.Background(), "gw-1", "2026-10-21T10:00:00Z", 1, "Bearer original-user-token-xyz")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if receivedHeaders.Get("Authorization") != "Bearer original-user-token-xyz" {
		t.Errorf("expected Authorization 'Bearer original-user-token-xyz', got %q", receivedHeaders.Get("Authorization"))
	}
	if receivedHeaders.Get("X-API-KEY") != "" {
		t.Errorf("expected X-API-KEY to be empty on user-scoped request, got %q", receivedHeaders.Get("X-API-KEY"))
	}
	if receivedHeaders.Get("X-INTERNAL-NAME") != "" {
		t.Errorf("expected X-INTERNAL-NAME to be empty on user-scoped request, got %q", receivedHeaders.Get("X-INTERNAL-NAME"))
	}

	// Test 2: Raw token without "Bearer " prefix gets formatted properly
	_, err = client.GetGatewayWifiTemp(context.Background(), "gw-1", "2026-10-21T10:00:00Z", 1, "raw-token-123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if receivedHeaders.Get("Authorization") != "Bearer raw-token-123" {
		t.Errorf("expected Authorization 'Bearer raw-token-123', got %q", receivedHeaders.Get("Authorization"))
	}
}

// 5. Concurrent calls using different tokens
func TestUserScopedClient_ConcurrentCallsDifferentTokens(t *testing.T) {
	concurrency := 20
	var receivedAuth sync.Map

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		routerID := strings.TrimPrefix(r.URL.Path, "/api/v1/devices/")
		routerID = strings.TrimSuffix(routerID, "/memory-summary")
		auth := r.Header.Get("Authorization")
		receivedAuth.Store(routerID, auth)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"router":"` + routerID + `"}`))
	}))
	defer ts.Close()

	client, err := analytics.NewUserScopedClient(
		analytics.UserScopedConfig{BaseURL: ts.URL},
		analytics.WithHTTPClient(ts.Client()),
	)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	var wg sync.WaitGroup
	errCh := make(chan error, concurrency)

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			rid := fmt.Sprintf("router-%d", idx)
			tok := fmt.Sprintf("Bearer token-user-%d", idx)

			res, err := client.GetGatewayFreeMemory(context.Background(), rid, "2026-10-21T10:00:00Z", 1, tok)
			if err != nil {
				errCh <- fmt.Errorf("call %d failed: %w", idx, err)
				return
			}
			m, ok := res.(map[string]any)
			if !ok || m["router"] != rid {
				errCh <- fmt.Errorf("call %d unexpected response: %v", idx, res)
			}
		}(i)
	}

	wg.Wait()
	close(errCh)

	for err := range errCh {
		t.Errorf("concurrent error: %v", err)
	}

	// Verify each router got its own exact token
	for i := 0; i < concurrency; i++ {
		rid := fmt.Sprintf("router-%d", i)
		expectedTok := fmt.Sprintf("Bearer token-user-%d", i)
		val, ok := receivedAuth.Load(rid)
		if !ok || val != expectedTok {
			t.Errorf("router %s expected auth %q, got %v", rid, expectedTok, val)
		}
	}
}

// 6. HTTP 401 and 403 handling (non-retryable)
func TestUserScopedClient_HTTP401And403Handling(t *testing.T) {
	for _, code := range []int{http.StatusUnauthorized, http.StatusForbidden} {
		t.Run(fmt.Sprintf("Status_%d", code), func(t *testing.T) {
			var attempts int32
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				atomic.AddInt32(&attempts, 1)
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(code)
				_, _ = w.Write([]byte(`{"error":"access_denied","message":"forbidden or unauthorized"}`))
			}))
			defer ts.Close()

			client, err := analytics.NewUserScopedClient(
				analytics.UserScopedConfig{BaseURL: ts.URL, MaxRetries: 3},
				analytics.WithHTTPClient(ts.Client()),
			)
			if err != nil {
				t.Fatalf("failed to create client: %v", err)
			}

			_, err = client.GetGatewayFreeMemory(context.Background(), "gw-1", "2026-10-21T10:00:00Z", 1, "Bearer tok")
			if err == nil {
				t.Fatalf("expected error for status %d, got nil", code)
			}

			var apiErr *analytics.APIClientError
			if !errors.As(err, &apiErr) {
				t.Fatalf("expected *analytics.APIClientError, got %T: %v", err, err)
			}
			if apiErr.StatusCode == nil || *apiErr.StatusCode != code {
				t.Errorf("expected status code %d, got %v", code, apiErr.StatusCode)
			}

			// Must not retry on 401/403
			if attempts != 1 {
				t.Errorf("expected exactly 1 attempt, got %d", attempts)
			}

			formatted := analytics.FormatAnalyticsErrorMessage(apiErr)
			if want := fmt.Sprintf("OWANALYTICS request failed: status=%d", code); formatted != want {
				t.Errorf("formatted message = %q, want %q", formatted, want)
			}
			if body, ok := apiErr.ResponseBody.(map[string]any); !ok || body["error"] != "access_denied" {
				t.Errorf("expected structured error to retain upstream response, got %v", apiErr.ResponseBody)
			}
		})
	}
}

// 7. Retry behavior (5xx recovery, 429 Retry-After, cap exceeding)
func TestUserScopedClient_RetryBehavior(t *testing.T) {
	t.Run("5xx_Recovery", func(t *testing.T) {
		var attempts int32
		var sleptDurations []time.Duration
		var mu sync.Mutex

		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			cur := atomic.AddInt32(&attempts, 1)
			if cur < 3 {
				w.WriteHeader(http.StatusServiceUnavailable)
				_, _ = w.Write([]byte(`{"error":"temporarily unavailable"}`))
				return
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"recovered":true}`))
		}))
		defer ts.Close()

		client, err := analytics.NewUserScopedClient(
			analytics.UserScopedConfig{
				BaseURL:       ts.URL,
				MaxRetries:    2,
				MaxRetryDelay: 5 * time.Second,
			},
			analytics.WithHTTPClient(ts.Client()),
			analytics.WithSleeper(func(ctx context.Context, d time.Duration) error {
				mu.Lock()
				sleptDurations = append(sleptDurations, d)
				mu.Unlock()
				return nil
			}),
			analytics.WithJitter(func(base float64) float64 { return 0 }),
		)
		if err != nil {
			t.Fatalf("failed to create client: %v", err)
		}

		res, err := client.GetGatewayFreeMemory(context.Background(), "gw-1", "2026-10-21T10:00:00Z", 1, "Bearer tok")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		m := res.(map[string]any)
		if m["recovered"] != true {
			t.Errorf("expected recovered true, got %v", m["recovered"])
		}
		if attempts != 3 {
			t.Errorf("expected 3 attempts, got %d", attempts)
		}
		mu.Lock()
		defer mu.Unlock()
		if len(sleptDurations) != 2 {
			t.Errorf("expected 2 sleeps, got %d", len(sleptDurations))
		}
	})

	t.Run("429_RetryAfter_Honored", func(t *testing.T) {
		var attempts int32
		var sleptDurations []time.Duration
		var mu sync.Mutex

		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			cur := atomic.AddInt32(&attempts, 1)
			if cur == 1 {
				w.Header().Set("Retry-After", "2")
				w.WriteHeader(http.StatusTooManyRequests)
				_, _ = w.Write([]byte(`{"error":"rate_limited"}`))
				return
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"recovered":true}`))
		}))
		defer ts.Close()

		client, err := analytics.NewUserScopedClient(
			analytics.UserScopedConfig{
				BaseURL:       ts.URL,
				MaxRetries:    2,
				MaxRetryDelay: 5 * time.Second,
			},
			analytics.WithHTTPClient(ts.Client()),
			analytics.WithSleeper(func(ctx context.Context, d time.Duration) error {
				mu.Lock()
				sleptDurations = append(sleptDurations, d)
				mu.Unlock()
				return nil
			}),
		)
		if err != nil {
			t.Fatalf("failed to create client: %v", err)
		}

		_, err = client.GetGatewayWifiTemp(context.Background(), "gw-1", "2026-10-21T10:00:00Z", 1, "Bearer tok")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		mu.Lock()
		defer mu.Unlock()
		if len(sleptDurations) != 1 || sleptDurations[0] != 2*time.Second {
			t.Errorf("expected 1 sleep of 2s, got %v", sleptDurations)
		}
	})

	t.Run("429_RetryAfter_ExceedsCap_ReturnsImmediately", func(t *testing.T) {
		var attempts int32
		var sleptDurations []time.Duration
		var mu sync.Mutex

		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			atomic.AddInt32(&attempts, 1)
			w.Header().Set("Retry-After", "60")
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"error":"rate_limited"}`))
		}))
		defer ts.Close()

		client, err := analytics.NewUserScopedClient(
			analytics.UserScopedConfig{
				BaseURL:       ts.URL,
				MaxRetries:    2,
				MaxRetryDelay: 5 * time.Second,
			},
			analytics.WithHTTPClient(ts.Client()),
			analytics.WithSleeper(func(ctx context.Context, d time.Duration) error {
				mu.Lock()
				sleptDurations = append(sleptDurations, d)
				mu.Unlock()
				return nil
			}),
		)
		if err != nil {
			t.Fatalf("failed to create client: %v", err)
		}

		_, err = client.GetGatewayWifiTemp(context.Background(), "gw-1", "2026-10-21T10:00:00Z", 1, "Bearer tok")
		if err == nil {
			t.Fatalf("expected error, got nil")
		}

		var apiErr *analytics.APIClientError
		if !errors.As(err, &apiErr) || apiErr.StatusCode == nil || *apiErr.StatusCode != 429 {
			t.Errorf("expected 429 APIClientError, got %v", err)
		}
		if attempts != 1 {
			t.Errorf("expected 1 attempt, got %d", attempts)
		}
		mu.Lock()
		defer mu.Unlock()
		if len(sleptDurations) != 0 {
			t.Errorf("expected 0 sleeps, got %v", sleptDurations)
		}
	})
}

// 8. Context cancellation
func TestUserScopedClient_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // canceled before request

	client, err := analytics.NewUserScopedClient(
		analytics.UserScopedConfig{BaseURL: "http://127.0.0.1:9999"},
	)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	_, err = client.GetGatewayFreeMemory(ctx, "gw-1", "2026-10-21T10:00:00Z", 1, "Bearer tok")
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got %v", err)
	}
}

// 9. JSON response compatibility (exact numbers, large integer counters)
func TestUserScopedClient_JSONResponseCompatibility(t *testing.T) {
	largeIntJSON := `{"meta":{"routerId":"gw-1"},"data":{"rxBytes":9223372036854775807,"txBytes":1234567890123456,"floatVal":45.5}}`

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(largeIntJSON))
	}))
	defer ts.Close()

	client, err := analytics.NewUserScopedClient(
		analytics.UserScopedConfig{BaseURL: ts.URL},
		analytics.WithHTTPClient(ts.Client()),
	)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	res, err := client.GetDeviceBandwidthConsumption(context.Background(), "gw-1", "2026-10-21T10:00:00Z", 1, "Bearer tok")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify exact numeric serialization without float64 precision loss
	marshaled, err := json.Marshal(res)
	if err != nil {
		t.Fatalf("failed to marshal decoded response: %v", err)
	}

	marshaledStr := string(marshaled)
	if !strings.Contains(marshaledStr, "9223372036854775807") {
		t.Errorf("expected 9223372036854775807 preserved without precision loss, got: %s", marshaledStr)
	}

	m, ok := res.(map[string]any)
	if !ok {
		t.Fatalf("expected map[string]any, got %T", res)
	}
	data := m["data"].(map[string]any)
	num, ok := data["rxBytes"].(json.Number)
	if !ok {
		t.Errorf("expected json.Number for rxBytes, got %T", data["rxBytes"])
	} else if num.String() != "9223372036854775807" {
		t.Errorf("expected string '9223372036854775807', got %s", num.String())
	}
}

// 10. Standalone mode without discovery
func TestUserScopedClient_StandaloneMode(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"standalone":true}`))
	}))
	defer ts.Close()

	// Explicitly configured BaseURL, no Discovery, no Resolver
	client, err := analytics.NewUserScopedClient(
		analytics.UserScopedConfig{
			BaseURL:   ts.URL,
			Discovery: nil,
			Resolver:  nil,
		},
		analytics.WithHTTPClient(ts.Client()),
	)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	res, err := client.GetGatewayOfflineCount(context.Background(), "gw-1", "2026-10-21T10:00:00Z", 1, "Bearer tok")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m := res.(map[string]any)
	if m["standalone"] != true {
		t.Errorf("expected standalone true, got %v", m["standalone"])
	}
}

// 11. Discovery-enabled endpoint resolution
func TestUserScopedClient_DiscoveryEnabledResolution(t *testing.T) {
	publicServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"resolvedPublic":true}`))
	}))
	defer publicServer.Close()

	// Scenario A: Discovered instance has verified PublicEndPoint
	resolverWithPublic := &mockServiceResolver{
		instance: &common.ServiceInstance{
			Key:             "internal-secret-key",
			PrivateEndPoint: "http://internal-private.local",
			PublicEndPoint:  publicServer.URL,
		},
	}

	clientA, err := analytics.NewUserScopedClient(
		analytics.UserScopedConfig{Resolver: resolverWithPublic},
		analytics.WithHTTPClient(publicServer.Client()),
	)
	if err != nil {
		t.Fatalf("failed to create client A: %v", err)
	}

	res, err := clientA.GetGatewayWifiTemp(context.Background(), "gw-1", "2026-10-21T10:00:00Z", 1, "Bearer tok")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	mA := res.(map[string]any)
	if mA["resolvedPublic"] != true {
		t.Errorf("expected resolvedPublic true, got %v", mA["resolvedPublic"])
	}

	// Scenario B: Discovered instance has only PrivateEndPoint (NO PublicEndPoint)
	// Must NOT send user-scoped request to internal endpoint!
	resolverInternalOnly := &mockServiceResolver{
		instance: &common.ServiceInstance{
			Key:             "internal-secret-key",
			PrivateEndPoint: "http://internal-private.local",
			PublicEndPoint:  "", // No verified public endpoint!
		},
	}

	clientB, err := analytics.NewUserScopedClient(
		analytics.UserScopedConfig{Resolver: resolverInternalOnly},
	)
	if err != nil {
		t.Fatalf("failed to create client B: %v", err)
	}

	_, err = clientB.GetGatewayWifiTemp(context.Background(), "gw-1", "2026-10-21T10:00:00Z", 1, "Bearer tok")
	if err == nil {
		t.Fatal("expected error when discovered service lacks verified public endpoint, got nil")
	}
	if apperror.CodeOf(err) != apperror.CodeInvalidInput {
		t.Errorf("expected CodeInvalidInput, got %s", apperror.CodeOf(err))
	}
}

// 12. Existing internal Analytics RPC methods remain functional
func TestExistingInternalAnalyticsRPC_RemainsFunctional(t *testing.T) {
	var receivedAuth, receivedInternalName, receivedApiKey string

	internalServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedAuth = r.Header.Get("Authorization")
		receivedApiKey = r.Header.Get("X-API-KEY")
		receivedInternalName = r.Header.Get("X-INTERNAL-NAME")

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"devices":[{"boardId":"b1","serialNumber":"SER123"}]}`))
	}))
	defer internalServer.Close()

	resolver := &mockServiceResolver{
		instance: &common.ServiceInstance{
			Key:             "service-internal-api-key",
			PrivateEndPoint: internalServer.URL,
		},
	}

	requester, err := common.NewFiberRequester("")
	if err != nil {
		t.Fatalf("failed to create fiber requester: %v", err)
	}

	base, err := common.NewServiceRPCBaseWithDeps(
		resolver,
		requester,
		"mango-mcp-server",
		slog.Default(),
	)
	if err != nil {
		t.Fatalf("failed to create base: %v", err)
	}

	internalClient, err := analytics.NewAnalyticsClient(base)
	if err != nil {
		t.Fatalf("failed to create internal analytics client: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	devices, err := internalClient.GetDeviceInfo(ctx, "b1")
	if err != nil {
		t.Fatalf("GetDeviceInfo failed: %v", err)
	}

	if len(devices) != 1 || devices[0].SerialNumber != "SER123" {
		t.Errorf("unexpected devices response: %v", devices)
	}

	// Verify internal RPC headers
	if receivedApiKey != "service-internal-api-key" {
		t.Errorf("expected X-API-KEY 'service-internal-api-key', got %q", receivedApiKey)
	}
	if receivedInternalName != "mango-mcp-server" {
		t.Errorf("expected X-INTERNAL-NAME 'mango-mcp-server', got %q", receivedInternalName)
	}
	if receivedAuth != "" {
		t.Errorf("expected Authorization empty for internal RPC, got %q", receivedAuth)
	}
}

// Path escaping verification
func TestUserScopedClient_URLEscaping(t *testing.T) {
	var recordedPath string

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		recordedPath = r.URL.EscapedPath()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer ts.Close()

	client, err := analytics.NewUserScopedClient(
		analytics.UserScopedConfig{BaseURL: ts.URL},
		analytics.WithHTTPClient(ts.Client()),
	)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	_, err = client.GetGatewayFreeMemory(context.Background(), "router/123 ?#", "2026-10-21T10:00:00Z", 1, "Bearer tok")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expectedSegment := "router%2F123%20%3F%23"
	if !strings.Contains(recordedPath, expectedSegment) {
		t.Errorf("expected EscapedPath to contain %q, got %q", expectedSegment, recordedPath)
	}
}

func TestFormatAnalyticsErrorMessage_OmitsUpstreamBody(t *testing.T) {
	statusCode := 403
	for _, tc := range []struct {
		name string
		body any
	}{
		{
			name: "top-level credentials and arbitrary fields",
			body: map[string]any{
				"token":        "secret-user-token",
				"password":     "super-secret-password",
				"api_key":      "secret-api-key",
				"public_field": "sensitive despite its name",
			},
		},
		{
			name: "nested token",
			body: map[string]any{"error": "failed", "details": map[string]any{"token": "sensitive-token"}},
		},
		{
			name: "nested array",
			body: map[string]any{"details": []any{map[string]any{"Authorization": "Bearer secret"}}},
		},
		{
			name: "structured error and message",
			body: map[string]any{
				"error":   map[string]any{"token": "secret-token"},
				"message": []any{map[string]any{"password": "secret-password"}},
			},
		},
		{
			name: "credentials in error and message strings",
			body: map[string]any{"error": "Bearer secret-token", "message": "password=secret-password"},
		},
		{name: "plain text", body: "request failed for Bearer secret-token"},
		{name: "JSON string", body: `{"token":"secret-token"}`},
		{name: "array body", body: []any{map[string]any{"token": "secret-token"}}},
		{name: "empty body"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			apiErr := &analytics.APIClientError{
				Operation:    "operation-with-sensitive-data",
				StatusCode:   &statusCode,
				ResponseBody: tc.body,
				Err:          errors.New("transport error with secret-token"),
			}
			if got, want := analytics.FormatAnalyticsErrorMessage(apiErr), "OWANALYTICS request failed: status=403"; got != want {
				t.Errorf("formatted message = %q, want %q", got, want)
			}
		})
	}
}

func TestFormatAnalyticsErrorMessage_WithoutStatus(t *testing.T) {
	if got := analytics.FormatAnalyticsErrorMessage(nil); got != "" {
		t.Errorf("nil error formatted as %q", got)
	}
	apiErr := &analytics.APIClientError{
		Operation:    "operation-with-sensitive-data",
		ResponseBody: "secret-response",
		Err:          errors.New("transport error with secret-token"),
	}
	if got, want := analytics.FormatAnalyticsErrorMessage(apiErr), "OWANALYTICS request failed: service unavailable"; got != want {
		t.Errorf("formatted message = %q, want %q", got, want)
	}
}

// -----------------------------------------------------------------------------
// Regression Tests: User-Scoped OWANALYTICS Endpoint Resolution & Precedence
// -----------------------------------------------------------------------------

// 1. Explicit BaseURL overrides discovered localhost.
func TestUserScopedClient_Regression_ExplicitBaseURLOverridesDiscoveredLocalhost(t *testing.T) {
	var explicitHost, explicitPath string
	var explicitQuery url.Values
	var explicitHeaders http.Header
	var explicitHit, discoveredHit bool
	var mu sync.Mutex

	explicitServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		explicitHit = true
		explicitHost = r.Host
		explicitPath = r.URL.Path
		explicitQuery = r.URL.Query()
		explicitHeaders = r.Header.Clone()
		mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"source":"explicit-docker-service"}`))
	}))
	defer explicitServer.Close()

	discoveredServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		discoveredHit = true
		mu.Unlock()
		t.Errorf("discovered localhost server was hit unexpectedly")
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer discoveredServer.Close()

	// Service discovery advertises localhost:16009 (which would fail in container)
	resolver := &mockServiceResolver{
		instance: &common.ServiceInstance{
			Key:             "internal-key",
			PrivateEndPoint: "http://owanalytics-priv:16009",
			PublicEndPoint:  discoveredServer.URL,
		},
	}

	client, err := analytics.NewUserScopedClient(
		analytics.UserScopedConfig{
			BaseURL:  explicitServer.URL,
			Resolver: resolver,
		},
		analytics.WithHTTPClient(explicitServer.Client()),
	)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	ctx := context.Background()
	res, err := client.GetGatewayFreeMemory(ctx, "gw-10", "2026-10-21T10:00:00Z", 24, "Bearer user-token-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()

	if !explicitHit {
		t.Fatal("expected explicit server to receive request")
	}
	if discoveredHit {
		t.Fatal("discovered server was hit unexpectedly")
	}

	explicitURL, _ := url.Parse(explicitServer.URL)
	if explicitHost != explicitURL.Host {
		t.Errorf("expected destination host %q, got %q", explicitURL.Host, explicitHost)
	}
	expectedPath := "/api/v1/devices/gw-10/memory-summary"
	if explicitPath != expectedPath {
		t.Errorf("expected path %q, got %q", expectedPath, explicitPath)
	}
	if explicitQuery.Get("timestampTill") != "2026-10-21T10:00:00Z" {
		t.Errorf("expected timestampTill '2026-10-21T10:00:00Z', got %q", explicitQuery.Get("timestampTill"))
	}
	if explicitQuery.Get("lookbackHours") != "24" {
		t.Errorf("expected lookbackHours '24', got %q", explicitQuery.Get("lookbackHours"))
	}
	if explicitHeaders.Get("Authorization") != "Bearer user-token-1" {
		t.Errorf("expected Authorization 'Bearer user-token-1', got %q", explicitHeaders.Get("Authorization"))
	}
	if explicitHeaders.Get("Accept") != "application/json" {
		t.Errorf("expected Accept 'application/json', got %q", explicitHeaders.Get("Accept"))
	}
	if explicitHeaders.Get("X-API-KEY") != "" {
		t.Errorf("expected no X-API-KEY, got %q", explicitHeaders.Get("X-API-KEY"))
	}
	if explicitHeaders.Get("X-INTERNAL-NAME") != "" {
		t.Errorf("expected no X-INTERNAL-NAME, got %q", explicitHeaders.Get("X-INTERNAL-NAME"))
	}

	m, ok := res.(map[string]any)
	if !ok || m["source"] != "explicit-docker-service" {
		t.Errorf("unexpected response body: %v", res)
	}
}

// 2. Explicit BaseURL overrides a different discovered public hostname.
func TestUserScopedClient_Regression_ExplicitBaseURLOverridesDifferentDiscoveredPublicHostname(t *testing.T) {
	var explicitHost, explicitPath string
	var explicitQuery url.Values
	var explicitHeaders http.Header
	var explicitHit, discoveredHit bool
	var mu sync.Mutex

	explicitServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		explicitHit = true
		explicitHost = r.Host
		explicitPath = r.URL.Path
		explicitQuery = r.URL.Query()
		explicitHeaders = r.Header.Clone()
		mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"source":"custom-configured-domain"}`))
	}))
	defer explicitServer.Close()

	discoveredServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		discoveredHit = true
		mu.Unlock()
		t.Errorf("discovered public server was hit unexpectedly")
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer discoveredServer.Close()

	resolver := &mockServiceResolver{
		instance: &common.ServiceInstance{
			Key:             "internal-key",
			PrivateEndPoint: "http://owanalytics-priv:16009",
			PublicEndPoint:  discoveredServer.URL,
		},
	}

	client, err := analytics.NewUserScopedClient(
		analytics.UserScopedConfig{
			BaseURL:  explicitServer.URL,
			Resolver: resolver,
		},
		analytics.WithHTTPClient(explicitServer.Client()),
	)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	ctx := context.Background()
	res, err := client.GetGatewayWifiTemp(ctx, "gw-20", "2026-10-21T11:00:00Z", 6, "Bearer user-token-2")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()

	if !explicitHit {
		t.Fatal("expected explicit server to receive request")
	}
	if discoveredHit {
		t.Fatal("discovered server was hit unexpectedly")
	}

	explicitURL, _ := url.Parse(explicitServer.URL)
	if explicitHost != explicitURL.Host {
		t.Errorf("expected destination host %q, got %q", explicitURL.Host, explicitHost)
	}
	expectedPath := "/api/v1/devices/gw-20/radio-temperature-summary"
	if explicitPath != expectedPath {
		t.Errorf("expected path %q, got %q", expectedPath, explicitPath)
	}
	if explicitQuery.Get("timestampTill") != "2026-10-21T11:00:00Z" {
		t.Errorf("expected timestampTill '2026-10-21T11:00:00Z', got %q", explicitQuery.Get("timestampTill"))
	}
	if explicitQuery.Get("lookbackHours") != "6" {
		t.Errorf("expected lookbackHours '6', got %q", explicitQuery.Get("lookbackHours"))
	}
	if explicitHeaders.Get("Authorization") != "Bearer user-token-2" {
		t.Errorf("expected Authorization 'Bearer user-token-2', got %q", explicitHeaders.Get("Authorization"))
	}
	if explicitHeaders.Get("Accept") != "application/json" {
		t.Errorf("expected Accept 'application/json', got %q", explicitHeaders.Get("Accept"))
	}
	if explicitHeaders.Get("X-API-KEY") != "" {
		t.Errorf("expected no X-API-KEY, got %q", explicitHeaders.Get("X-API-KEY"))
	}
	if explicitHeaders.Get("X-INTERNAL-NAME") != "" {
		t.Errorf("expected no X-INTERNAL-NAME, got %q", explicitHeaders.Get("X-INTERNAL-NAME"))
	}

	m, ok := res.(map[string]any)
	if !ok || m["source"] != "custom-configured-domain" {
		t.Errorf("unexpected response body: %v", res)
	}
}

// 3. Discovery is used when BaseURL is empty.
func TestUserScopedClient_Regression_DiscoveryUsedWhenBaseURLEmpty(t *testing.T) {
	var receivedHost, receivedPath string
	var receivedQuery url.Values
	var receivedHeaders http.Header
	var serverHit bool
	var mu sync.Mutex

	discoveredServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		serverHit = true
		receivedHost = r.Host
		receivedPath = r.URL.Path
		receivedQuery = r.URL.Query()
		receivedHeaders = r.Header.Clone()
		mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"source":"discovered-public-endpoint"}`))
	}))
	defer discoveredServer.Close()

	resolver := &mockServiceResolver{
		instance: &common.ServiceInstance{
			Key:             "internal-key",
			PrivateEndPoint: "http://owanalytics-priv:16009",
			PublicEndPoint:  discoveredServer.URL,
		},
	}

	client, err := analytics.NewUserScopedClient(
		analytics.UserScopedConfig{
			BaseURL:  "", // Empty BaseURL triggers discovery
			Resolver: resolver,
		},
		analytics.WithHTTPClient(discoveredServer.Client()),
	)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	ctx := context.Background()
	res, err := client.GetDeviceBandwidthConsumption(ctx, "gw-30", "2026-10-21T12:00:00Z", 12, "Bearer disc-token")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()

	if !serverHit {
		t.Fatal("expected discovered server to be hit")
	}

	discoveredURL, _ := url.Parse(discoveredServer.URL)
	if receivedHost != discoveredURL.Host {
		t.Errorf("expected destination host %q, got %q", discoveredURL.Host, receivedHost)
	}
	expectedPath := "/api/v1/devices/gw-30/wifi-clients/usage-summary"
	if receivedPath != expectedPath {
		t.Errorf("expected path %q, got %q", expectedPath, receivedPath)
	}
	if receivedQuery.Get("timestampTill") != "2026-10-21T12:00:00Z" {
		t.Errorf("expected timestampTill '2026-10-21T12:00:00Z', got %q", receivedQuery.Get("timestampTill"))
	}
	if receivedQuery.Get("lookbackHours") != "12" {
		t.Errorf("expected lookbackHours '12', got %q", receivedQuery.Get("lookbackHours"))
	}
	if receivedHeaders.Get("Authorization") != "Bearer disc-token" {
		t.Errorf("expected Authorization 'Bearer disc-token', got %q", receivedHeaders.Get("Authorization"))
	}
	if receivedHeaders.Get("Accept") != "application/json" {
		t.Errorf("expected Accept 'application/json', got %q", receivedHeaders.Get("Accept"))
	}
	if receivedHeaders.Get("X-API-KEY") != "" {
		t.Errorf("expected no X-API-KEY, got %q", receivedHeaders.Get("X-API-KEY"))
	}
	if receivedHeaders.Get("X-INTERNAL-NAME") != "" {
		t.Errorf("expected no X-INTERNAL-NAME, got %q", receivedHeaders.Get("X-INTERNAL-NAME"))
	}

	m, ok := res.(map[string]any)
	if !ok || m["source"] != "discovered-public-endpoint" {
		t.Errorf("unexpected response body: %v", res)
	}
}

// 4. Missing BaseURL and missing resolver return a clear error.
func TestUserScopedClient_Regression_MissingBaseURLAndMissingResolverReturnError(t *testing.T) {
	client, err := analytics.NewUserScopedClient(analytics.UserScopedConfig{
		BaseURL:   "",
		Resolver:  nil,
		Discovery: nil,
	})
	if err != nil {
		t.Fatalf("unexpected client creation error: %v", err)
	}

	_, err = client.GetGatewayFreeMemory(context.Background(), "gw-1", "2026-10-21T10:00:00Z", 1, "Bearer tok")
	if err == nil {
		t.Fatal("expected error when BaseURL and resolver are both missing, got nil")
	}
	if apperror.CodeOf(err) != apperror.CodeInvalidInput {
		t.Errorf("expected CodeInvalidInput, got %s", apperror.CodeOf(err))
	}
	errLower := strings.ToLower(err.Error())
	if !strings.Contains(errLower, "baseurl") && !strings.Contains(errLower, "resolver") {
		t.Errorf("expected error message to mention baseURL or resolver, got %v", err)
	}
}

// 5. Discovery failure returns an error when no explicit URL exists.
func TestUserScopedClient_Regression_DiscoveryFailureReturnsErrorWhenNoExplicitURL(t *testing.T) {
	expectedErr := errors.New("kafka discovery connection timeout")
	resolver := &mockServiceResolver{
		err: expectedErr,
	}

	client, err := analytics.NewUserScopedClient(analytics.UserScopedConfig{
		BaseURL:  "",
		Resolver: resolver,
	})
	if err != nil {
		t.Fatalf("unexpected client creation error: %v", err)
	}

	_, err = client.GetDeviceRssiQuality(context.Background(), "gw-1", "2026-10-21T10:00:00Z", 1, "Bearer tok")
	if err == nil {
		t.Fatal("expected error when discovery fails, got nil")
	}
	if !errors.Is(err, expectedErr) && !strings.Contains(err.Error(), expectedErr.Error()) {
		t.Errorf("expected error to wrap or contain discovery error, got %v", err)
	}
}

// 6. A discovered instance containing only `PrivateEndPoint` is rejected.
func TestUserScopedClient_Regression_DiscoveredInstanceWithOnlyPrivateEndPointRejected(t *testing.T) {
	var privateHit bool
	privateServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		privateHit = true
		t.Errorf("unexpected call to private endpoint for user-scoped request")
		w.WriteHeader(http.StatusOK)
	}))
	defer privateServer.Close()

	resolver := &mockServiceResolver{
		instance: &common.ServiceInstance{
			Key:             "internal-secret-key",
			PrivateEndPoint: privateServer.URL,
			PublicEndPoint:  "", // No PublicEndPoint provided
		},
	}

	client, err := analytics.NewUserScopedClient(analytics.UserScopedConfig{
		BaseURL:  "",
		Resolver: resolver,
	})
	if err != nil {
		t.Fatalf("unexpected client creation error: %v", err)
	}

	_, err = client.GetGatewayOfflineCount(context.Background(), "gw-1", "2026-10-21T10:00:00Z", 1, "Bearer tok")
	if err == nil {
		t.Fatal("expected error when discovered service has only PrivateEndPoint, got nil")
	}
	if apperror.CodeOf(err) != apperror.CodeInvalidInput {
		t.Errorf("expected CodeInvalidInput, got %s", apperror.CodeOf(err))
	}
	if !strings.Contains(err.Error(), "public endpoint") {
		t.Errorf("expected error message to mention public endpoint, got %v", err)
	}
	if privateHit {
		t.Fatal("PrivateEndPoint was unexpectedly invoked for user-scoped request")
	}
}

// 7. Invalid URL configurations are rejected.
func TestUserScopedClient_Regression_InvalidURLConfigurationsRejected(t *testing.T) {
	invalidURLs := []struct {
		name string
		url  string
	}{
		{name: "malformed URL", url: "://missing-scheme"},
		{name: "unsupported scheme ftp", url: "ftp://analytics.example.com:16009"},
		{name: "unsupported scheme file", url: "file:///tmp/socket"},
		{name: "missing hostname http://", url: "http://"},
		{name: "missing hostname https://", url: "https://"},
		{name: "missing hostname with port", url: "http://:16009"},
		{name: "missing hostname path only", url: "http:///api/v1"},
		{name: "embedded user:pass credentials", url: "https://user:pass@analytics.example.com:16009"},
		{name: "embedded user credentials", url: "https://user@analytics.example.com:16009"},
		{name: "URL fragment", url: "https://analytics.example.com:16009/api#section"},
		{name: "trailing hash fragment", url: "https://analytics.example.com:16009/#"},
	}

	for _, tc := range invalidURLs {
		t.Run("ConfiguredBaseURL_"+tc.name, func(t *testing.T) {
			validDiscoveredServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				t.Errorf("discovery endpoint hit despite invalid configured BaseURL")
			}))
			defer validDiscoveredServer.Close()

			resolver := &mockServiceResolver{
				instance: &common.ServiceInstance{
					PublicEndPoint: validDiscoveredServer.URL,
				},
			}

			client, err := analytics.NewUserScopedClient(analytics.UserScopedConfig{
				BaseURL:  tc.url,
				Resolver: resolver,
			})
			if err != nil {
				if apperror.CodeOf(err) != apperror.CodeInvalidInput {
					t.Errorf("expected CodeInvalidInput at construction, got %s", apperror.CodeOf(err))
				}
				return
			}

			// If client creation allowed it, request must reject it without falling back to discovery
			_, err = client.GetGatewayFreeMemory(context.Background(), "gw-1", "2026-10-21T10:00:00Z", 1, "Bearer tok")
			if err == nil {
				t.Fatalf("expected error for invalid BaseURL %q, got nil", tc.url)
			}
			if apperror.CodeOf(err) != apperror.CodeInvalidInput {
				t.Errorf("expected CodeInvalidInput on request, got %s: %v", apperror.CodeOf(err), err)
			}
		})

		t.Run("DiscoveredPublicEndPoint_"+tc.name, func(t *testing.T) {
			resolver := &mockServiceResolver{
				instance: &common.ServiceInstance{
					PublicEndPoint: tc.url,
				},
			}

			client, err := analytics.NewUserScopedClient(analytics.UserScopedConfig{
				BaseURL:  "",
				Resolver: resolver,
			})
			if err != nil {
				t.Fatalf("unexpected error creating client with resolver: %v", err)
			}

			_, err = client.GetGatewayFreeMemory(context.Background(), "gw-1", "2026-10-21T10:00:00Z", 1, "Bearer tok")
			if err == nil {
				t.Fatalf("expected error for invalid discovered endpoint %q, got nil", tc.url)
			}
			if apperror.CodeOf(err) != apperror.CodeInvalidInput {
				t.Errorf("expected CodeInvalidInput, got %s: %v", apperror.CodeOf(err), err)
			}
		})
	}
}

// 8. Bearer tokens are forwarded only to the selected public API endpoint.
func TestUserScopedClient_Regression_BearerTokenForwardedOnlyToSelectedPublicEndpoint(t *testing.T) {
	var targetHost, otherHost string
	var targetAuth, otherAuth string
	var targetApiKey, otherApiKey string
	var targetInternalName, otherInternalName string
	var otherHit bool
	var mu sync.Mutex

	// Unexpected host that targetServer redirects to
	otherServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		otherHit = true
		otherHost = r.Host
		otherAuth = r.Header.Get("Authorization")
		otherApiKey = r.Header.Get("X-API-KEY")
		otherInternalName = r.Header.Get("X-INTERNAL-NAME")
		mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"other":true}`))
	}))
	defer otherServer.Close()

	// Selected public endpoint redirects cross-origin to otherServer
	targetServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		targetHost = r.Host
		targetAuth = r.Header.Get("Authorization")
		targetApiKey = r.Header.Get("X-API-KEY")
		targetInternalName = r.Header.Get("X-INTERNAL-NAME")
		mu.Unlock()

		http.Redirect(w, r, otherServer.URL+"/api/v1/devices/gw-sec-1/memory-summary", http.StatusFound)
	}))
	defer targetServer.Close()

	client, err := analytics.NewUserScopedClient(
		analytics.UserScopedConfig{
			BaseURL: targetServer.URL,
		},
		analytics.WithHTTPClient(targetServer.Client()),
	)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	res, err := client.GetGatewayFreeMemory(context.Background(), "gw-sec-1", "2026-10-21T10:00:00Z", 1, "Bearer secret-user-token")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()

	targetURL, _ := url.Parse(targetServer.URL)
	if targetHost != targetURL.Host {
		t.Errorf("expected target host %q, got %q", targetURL.Host, targetHost)
	}
	if targetAuth != "Bearer secret-user-token" {
		t.Errorf("expected target server to receive bearer token, got %q", targetAuth)
	}
	if targetApiKey != "" {
		t.Errorf("expected no X-API-KEY on target server, got %q", targetApiKey)
	}
	if targetInternalName != "" {
		t.Errorf("expected no X-INTERNAL-NAME on target server, got %q", targetInternalName)
	}

	if !otherHit {
		t.Fatal("expected otherServer to be hit via redirect")
	}
	otherURL, _ := url.Parse(otherServer.URL)
	if otherHost != otherURL.Host {
		t.Errorf("expected other host %q, got %q", otherURL.Host, otherHost)
	}
	// CRITICAL SECURITY ASSERTION: Bearer token must NOT be leaked across origins!
	if otherAuth != "" {
		t.Errorf("SECURITY LEAK: Bearer token was forwarded to unexpected host: %q", otherAuth)
	}
	if otherApiKey != "" {
		t.Errorf("expected no X-API-KEY on other server, got %q", otherApiKey)
	}
	if otherInternalName != "" {
		t.Errorf("expected no X-INTERNAL-NAME on other server, got %q", otherInternalName)
	}

	m, ok := res.(map[string]any)
	if !ok || m["other"] != true {
		t.Errorf("unexpected response body: %v", res)
	}
}

// 9. Existing internal RPC methods continue using PrivateEndPoint with their original authentication headers.
func TestUserScopedClient_Regression_InternalRPCContinuesUsingPrivateEndPoint(t *testing.T) {
	var receivedAuth, receivedInternalName, receivedApiKey, receivedHost string

	internalServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedHost = r.Host
		receivedAuth = r.Header.Get("Authorization")
		receivedApiKey = r.Header.Get("X-API-KEY")
		receivedInternalName = r.Header.Get("X-INTERNAL-NAME")

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"devices":[{"boardId":"b2","serialNumber":"INTERNAL-SER-456"}]}`))
	}))
	defer internalServer.Close()

	// Instance provides BOTH PrivateEndPoint and a distinct PublicEndPoint.
	// Internal RPC must strictly use PrivateEndPoint and internal auth headers.
	resolver := &mockServiceResolver{
		instance: &common.ServiceInstance{
			Key:             "service-internal-api-key-999",
			PrivateEndPoint: internalServer.URL,
			PublicEndPoint:  "https://public.example.com:16009",
		},
	}

	requester, err := common.NewFiberRequester("")
	if err != nil {
		t.Fatalf("failed to create fiber requester: %v", err)
	}

	base, err := common.NewServiceRPCBaseWithDeps(
		resolver,
		requester,
		"internal-caller-service",
		slog.Default(),
	)
	if err != nil {
		t.Fatalf("failed to create base: %v", err)
	}

	internalClient, err := analytics.NewAnalyticsClient(base)
	if err != nil {
		t.Fatalf("failed to create internal client: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	devices, err := internalClient.GetDeviceInfo(ctx, "b2")
	if err != nil {
		t.Fatalf("GetDeviceInfo failed: %v", err)
	}

	if len(devices) != 1 || devices[0].SerialNumber != "INTERNAL-SER-456" {
		t.Errorf("unexpected devices response: %v", devices)
	}

	internalURL, _ := url.Parse(internalServer.URL)
	if receivedHost != internalURL.Host {
		t.Errorf("expected destination host %q, got %q", internalURL.Host, receivedHost)
	}
	if receivedApiKey != "service-internal-api-key-999" {
		t.Errorf("expected X-API-KEY 'service-internal-api-key-999', got %q", receivedApiKey)
	}
	if receivedInternalName != "internal-caller-service" {
		t.Errorf("expected X-INTERNAL-NAME 'internal-caller-service', got %q", receivedInternalName)
	}
	if receivedAuth != "" {
		t.Errorf("expected Authorization empty for internal RPC, got %q", receivedAuth)
	}
}

// 10. Concurrent requests remain safe.
func TestUserScopedClient_Regression_ConcurrentRequestsRemainSafe(t *testing.T) {
	concurrency := 30
	var receivedTokens sync.Map
	var hitCount int64

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&hitCount, 1)
		token := r.Header.Get("Authorization")
		apiKey := r.Header.Get("X-API-KEY")
		if apiKey != "" {
			t.Errorf("internal API key leaked into user-scoped request: %s", apiKey)
		}

		parts := strings.Split(r.URL.Path, "/")
		var routerID string
		if len(parts) >= 5 {
			routerID = parts[4]
		}
		receivedTokens.Store(routerID, token)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(fmt.Sprintf(`{"routerId":"%s"}`, routerID)))
	}))
	defer ts.Close()

	client, err := analytics.NewUserScopedClient(
		analytics.UserScopedConfig{
			BaseURL: ts.URL,
		},
		analytics.WithHTTPClient(ts.Client()),
	)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	var wg sync.WaitGroup
	errCh := make(chan error, concurrency)

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			rid := fmt.Sprintf("gw-concurrent-%d", idx)
			tok := fmt.Sprintf("Bearer token-user-concurrent-%d", idx)

			var res any
			var callErr error

			// Interleave all 5 user-scoped methods
			switch idx % 5 {
			case 0:
				res, callErr = client.GetGatewayFreeMemory(context.Background(), rid, "2026-10-21T10:00:00Z", 1, tok)
			case 1:
				res, callErr = client.GetGatewayWifiTemp(context.Background(), rid, "2026-10-21T10:00:00Z", 1, tok)
			case 2:
				res, callErr = client.GetDeviceBandwidthConsumption(context.Background(), rid, "2026-10-21T10:00:00Z", 1, tok)
			case 3:
				res, callErr = client.GetDeviceRssiQuality(context.Background(), rid, "2026-10-21T10:00:00Z", 1, tok)
			case 4:
				res, callErr = client.GetGatewayOfflineCount(context.Background(), rid, "2026-10-21T10:00:00Z", 1, tok)
			}

			if callErr != nil {
				errCh <- fmt.Errorf("call %d failed: %w", idx, callErr)
				return
			}
			m, ok := res.(map[string]any)
			if !ok || m["routerId"] != rid {
				errCh <- fmt.Errorf("call %d returned unexpected body: %v", idx, res)
			}
		}(i)
	}

	wg.Wait()
	close(errCh)

	for err := range errCh {
		t.Errorf("concurrent error: %v", err)
	}

	if atomic.LoadInt64(&hitCount) != int64(concurrency) {
		t.Errorf("expected %d hits, got %d", concurrency, hitCount)
	}

	for i := 0; i < concurrency; i++ {
		rid := fmt.Sprintf("gw-concurrent-%d", i)
		expectedTok := fmt.Sprintf("Bearer token-user-concurrent-%d", i)
		val, ok := receivedTokens.Load(rid)
		if !ok || val != expectedTok {
			t.Errorf("router %s expected token %q, got %v", rid, expectedTok, val)
		}
	}
}
