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
	_, err = client.GetGatewayWifiTemp(context.Background(), "gw-1", "now", 1, "Bearer original-user-token-xyz")
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
	_, err = client.GetGatewayWifiTemp(context.Background(), "gw-1", "now", 1, "raw-token-123")
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

			res, err := client.GetGatewayFreeMemory(context.Background(), rid, "now", 1, tok)
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

			_, err = client.GetGatewayFreeMemory(context.Background(), "gw-1", "now", 1, "Bearer tok")
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
			if !strings.Contains(formatted, fmt.Sprintf("status=%d", code)) {
				t.Errorf("formatted message %q missing status=%d", formatted, code)
			}
			if !strings.Contains(formatted, "access_denied") {
				t.Errorf("formatted message %q missing error payload", formatted)
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

		res, err := client.GetGatewayFreeMemory(context.Background(), "gw-1", "now", 1, "Bearer tok")
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

		_, err = client.GetGatewayWifiTemp(context.Background(), "gw-1", "now", 1, "Bearer tok")
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

		_, err = client.GetGatewayWifiTemp(context.Background(), "gw-1", "now", 1, "Bearer tok")
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

	_, err = client.GetGatewayFreeMemory(ctx, "gw-1", "now", 1, "Bearer tok")
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

	res, err := client.GetDeviceBandwidthConsumption(context.Background(), "gw-1", "now", 1, "Bearer tok")
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

	res, err := client.GetGatewayOfflineCount(context.Background(), "gw-1", "now", 1, "Bearer tok")
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

	res, err := clientA.GetGatewayWifiTemp(context.Background(), "gw-1", "now", 1, "Bearer tok")
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

	_, err = clientB.GetGatewayWifiTemp(context.Background(), "gw-1", "now", 1, "Bearer tok")
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

	_, err = client.GetGatewayFreeMemory(context.Background(), "router/123 ?#", "now", 1, "Bearer tok")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expectedSegment := "router%2F123%20%3F%23"
	if !strings.Contains(recordedPath, expectedSegment) {
		t.Errorf("expected EscapedPath to contain %q, got %q", expectedSegment, recordedPath)
	}
}

// Sensitive info redaction verification
func TestFormatAnalyticsErrorMessage_RedactsSensitiveData(t *testing.T) {
	statusCode := 403
	apiErr := &analytics.APIClientError{
		Operation:  "get_gateway_free_memory",
		StatusCode: &statusCode,
		ResponseBody: map[string]any{
			"error":        "forbidden",
			"message":      "access denied",
			"token":        "secret-user-token",
			"password":     "super-secret-password",
			"api_key":      "secret-api-key",
			"public_field": "visible",
		},
	}

	msg := analytics.FormatAnalyticsErrorMessage(apiErr)
	if strings.Contains(msg, "secret-user-token") {
		t.Errorf("token was not redacted: %s", msg)
	}
	if strings.Contains(msg, "super-secret-password") {
		t.Errorf("password was not redacted: %s", msg)
	}
	if strings.Contains(msg, "secret-api-key") {
		t.Errorf("api_key was not redacted: %s", msg)
	}
	if !strings.Contains(msg, "******") {
		t.Errorf("expected ****** redaction marker in: %s", msg)
	}
	if !strings.Contains(msg, "visible") {
		t.Errorf("expected non-sensitive field visible in: %s", msg)
	}
}
