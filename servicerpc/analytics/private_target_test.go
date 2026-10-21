package analytics_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/routerarchitects/ow-common-mods/servicerpc/analytics"
	"github.com/routerarchitects/ow-common-mods/servicerpc/common"
	"github.com/routerarchitects/ra-common-mods/apperror"
)

type privateResolverFunc func(string) (*common.ServiceInstance, error)

func (f privateResolverFunc) Resolve(name string) (*common.ServiceInstance, error) { return f(name) }

const privateCaller = "https://mango.example.com:8000"

func TestPrivateAnalyticsResolution(t *testing.T) {
	var fallbackCalls atomic.Int32
	fallback := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fallbackCalls.Add(1)
		t.Error("strict discovery request contacted public/static fallback")
	}))
	defer fallback.Close()
	for _, tt := range []struct {
		name       string
		instance   *common.ServiceInstance
		resolveErr error
		noResolver bool
		code       apperror.Code
	}{
		{name: "unavailable resolver", noResolver: true, code: apperror.CodeInternal},
		{name: "not registered", resolveErr: apperror.New(apperror.CodeNotFound, "not registered"), code: apperror.CodeNotFound},
		{name: "resolver failure", resolveErr: errors.New("discovery failed with secret-internal-key"), code: apperror.CodeInternal},
		{name: "nil service", code: apperror.CodeNotFound},
		{name: "no private endpoint", instance: &common.ServiceInstance{Key: "secret-internal-key"}, code: apperror.CodeInvalidInput},
		{name: "public only", instance: &common.ServiceInstance{PublicEndPoint: fallback.URL, Key: "secret-internal-key"}, code: apperror.CodeInvalidInput},
		{name: "relative endpoint", instance: &common.ServiceInstance{PrivateEndPoint: "/private", Key: "secret-internal-key"}, code: apperror.CodeInvalidInput},
		{name: "missing hostname", instance: &common.ServiceInstance{PrivateEndPoint: "https:///private", Key: "secret-internal-key"}, code: apperror.CodeInvalidInput},
		{name: "unsupported scheme", instance: &common.ServiceInstance{PrivateEndPoint: "ftp://analytics", Key: "secret-internal-key"}, code: apperror.CodeInvalidInput},
		{name: "endpoint credentials", instance: &common.ServiceInstance{PrivateEndPoint: "http://user:secret-internal-key@analytics", Key: "secret-internal-key"}, code: apperror.CodeInvalidInput},
		{name: "query", instance: &common.ServiceInstance{PrivateEndPoint: "http://analytics?token=secret-internal-key", Key: "secret-internal-key"}, code: apperror.CodeInvalidInput},
		{name: "fragment", instance: &common.ServiceInstance{PrivateEndPoint: "http://analytics#secret-internal-key", Key: "secret-internal-key"}, code: apperror.CodeInvalidInput},
		{name: "invalid port", instance: &common.ServiceInstance{PrivateEndPoint: "http://analytics:99999", Key: "secret-internal-key"}, code: apperror.CodeInvalidInput},
		{name: "missing key", instance: &common.ServiceInstance{PrivateEndPoint: fallback.URL}, code: apperror.CodeUnauthorized},
		{name: "invalid key", instance: &common.ServiceInstance{PrivateEndPoint: fallback.URL, Key: "secret-internal-key\r\nInjected: true"}, code: apperror.CodeUnauthorized},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var resolver common.ServiceResolver
			if !tt.noResolver {
				resolver = privateResolverFunc(func(name string) (*common.ServiceInstance, error) {
					if name != "owanalytics" {
						t.Errorf("unexpected service type %q", name)
					}
					return tt.instance, tt.resolveErr
				})
			}
			client, err := analytics.NewUserScopedClient(analytics.UserScopedConfig{
				EndpointMode: analytics.EndpointModeDiscoveryRequired, InternalName: privateCaller,
				BaseURL: fallback.URL, Resolver: resolver,
			})
			if err != nil {
				t.Fatal(err)
			}
			_, err = client.GetGatewayFreeMemory(context.Background(), "router-a", "2026-10-21T10:00:00Z", 24, "Bearer secret-user-token")
			if apperror.CodeOf(err) != tt.code {
				t.Fatalf("expected code %s, got %v", tt.code, err)
			}
			if strings.Contains(err.Error(), "secret-internal-key") || strings.Contains(err.Error(), "secret-user-token") {
				t.Fatal("secret leaked in resolution error")
			}
		})
	}
	if fallbackCalls.Load() != 0 {
		t.Fatal("unexpected fallback calls")
	}
}

func TestPrivateAnalyticsAllToolsAndCurrentRecord(t *testing.T) {
	var calls atomic.Int32
	private := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Header.Get("Authorization") != "bEaReR  original-user-token" || r.Header.Get("X-API-KEY") != "discovered-key" || r.Header.Get("X-INTERNAL-NAME") != privateCaller {
			t.Error("request-scoped user or internal authentication headers changed")
		}
		if !strings.Contains(r.URL.EscapedPath(), "/router%2Fa/") || r.URL.Query().Get("timestampTill") != "2026-10-21T10:00:00Z" || r.URL.Query().Get("lookbackHours") != "24" {
			t.Error("summary path escaping or query parameters changed")
		}
		_, _ = w.Write([]byte(`{"private":true}`))
	}))
	defer private.Close()
	available := true
	resolver := privateResolverFunc(func(string) (*common.ServiceInstance, error) {
		if !available {
			return nil, nil
		}
		return &common.ServiceInstance{PrivateEndPoint: private.URL, PublicEndPoint: "http://public.invalid", Key: "discovered-key"}, nil
	})
	client, err := analytics.NewUserScopedClient(analytics.UserScopedConfig{
		EndpointMode: analytics.EndpointModeDiscoveryRequired, InternalName: privateCaller,
		Resolver: resolver, BaseURL: "malformed static URL is ignored in strict mode",
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, call := range []func(context.Context, string, string, int, string) (any, error){
		client.GetGatewayFreeMemory, client.GetGatewayWifiTemp, client.GetDeviceBandwidthConsumption,
		client.GetDeviceRssiQuality, client.GetGatewayOfflineCount,
	} {
		if _, err := call(context.Background(), "router/a", "2026-10-21T10:00:00Z", 24, "bEaReR  original-user-token"); err != nil {
			t.Fatal(err)
		}
	}
	available = false
	if _, err := client.GetGatewayFreeMemory(context.Background(), "router/a", "2026-10-21T10:00:00Z", 24, "Bearer token"); err == nil {
		t.Fatal("expired record was cached")
	}
	available = true
	if _, err := client.GetGatewayFreeMemory(context.Background(), "router/a", "2026-10-21T10:00:00Z", 24, "bEaReR  original-user-token"); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 6 {
		t.Fatalf("expected six private requests, got %d", calls.Load())
	}
}

func TestPrivateAnalyticsConcurrentUserAuthorization(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.Header.Get("X-API-KEY") != "internal-key" || r.Header.Get("X-INTERNAL-NAME") != privateCaller {
			w.WriteHeader(401)
			return
		}
		router := strings.Split(r.URL.Path, "/")[4]
		if r.Header.Get("Authorization") != "Bearer user-"+strings.TrimPrefix(router, "router-") {
			w.WriteHeader(403)
			return
		}
		_, _ = w.Write([]byte(`{"authorized":true}`))
	}))
	defer server.Close()
	client, err := analytics.NewUserScopedClient(analytics.UserScopedConfig{
		EndpointMode: analytics.EndpointModeDiscoveryRequired, InternalName: privateCaller,
		Resolver: privateResolverFunc(func(string) (*common.ServiceInstance, error) {
			return &common.ServiceInstance{PrivateEndPoint: server.URL, Key: "internal-key"}, nil
		}),
	})
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		for _, user := range []string{"a", "b"} {
			for _, router := range []string{"a", "b"} {
				wg.Add(1)
				go func() {
					defer wg.Done()
					_, err := client.GetGatewayFreeMemory(context.Background(), "router-"+router, "2026-10-21T10:00:00Z", 24, "Bearer user-"+user)
					if user == router {
						if err != nil {
							t.Errorf("authorized request failed: %v", err)
						}
						return
					}
					var apiErr *analytics.APIClientError
					if !errors.As(err, &apiErr) || apiErr.StatusCode == nil || *apiErr.StatusCode != 403 {
						t.Errorf("expected 403, got %v", err)
					}
				}()
			}
		}
	}
	wg.Wait()
	if requests.Load() != 40 {
		t.Fatalf("authorization failures must not be retried, got %d requests", requests.Load())
	}
}

func TestPrivateAnalyticsTransportFailuresNeverFallback(t *testing.T) {
	var fallbackCalls atomic.Int32
	fallback := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { fallbackCalls.Add(1) }))
	defer fallback.Close()
	closed := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	closed.Close()
	tlsServer := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer tlsServer.Close()
	timeoutServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { <-r.Context().Done() }))
	defer timeoutServer.Close()
	redirectServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, fallback.URL, 302) }))
	defer redirectServer.Close()
	for _, tt := range []struct{ name, endpoint string }{
		{"connection refused", closed.URL}, {"untrusted TLS", tlsServer.URL}, {"deadline", timeoutServer.URL}, {"redirect", redirectServer.URL},
	} {
		t.Run(tt.name, func(t *testing.T) {
			client, err := analytics.NewUserScopedClient(analytics.UserScopedConfig{
				EndpointMode: analytics.EndpointModeDiscoveryRequired, InternalName: privateCaller, BaseURL: fallback.URL, Timeout: 30 * time.Millisecond,
				Resolver: privateResolverFunc(func(string) (*common.ServiceInstance, error) {
					return &common.ServiceInstance{PrivateEndPoint: tt.endpoint, PublicEndPoint: fallback.URL, Key: "internal-key"}, nil
				}),
			}, analytics.WithMaxRetries(0))
			if err != nil {
				t.Fatal(err)
			}
			if _, err := client.GetGatewayFreeMemory(context.Background(), "router-a", "2026-10-21T10:00:00Z", 24, "Bearer user-a"); err == nil {
				t.Fatal("expected controlled failure")
			}
		})
	}
	if fallbackCalls.Load() != 0 {
		t.Fatal("credentials sent to a fallback/redirect target")
	}
}

type privateRoundTripper func(*http.Request) (*http.Response, error)

func (f privateRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestPrivateAnalyticsErrorsAndLogsDoNotExposeSecrets(t *testing.T) {
	var logs bytes.Buffer
	client, err := analytics.NewUserScopedClient(analytics.UserScopedConfig{
		EndpointMode: analytics.EndpointModeDiscoveryRequired, InternalName: privateCaller,
		Resolver: privateResolverFunc(func(string) (*common.ServiceInstance, error) {
			return &common.ServiceInstance{PrivateEndPoint: "https://private.example", Key: "internal-secret"}, nil
		}),
		HTTPClient: &http.Client{Transport: privateRoundTripper(func(*http.Request) (*http.Response, error) {
			return nil, errors.New("Bearer user-secret internal-secret")
		})},
		Logger: slog.New(slog.NewTextHandler(&logs, nil)),
	}, analytics.WithSleeper(func(context.Context, time.Duration) error { return nil }))
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.GetGatewayFreeMemory(context.Background(), "router-a", "2026-10-21T10:00:00Z", 24, "Bearer user-secret")
	if err == nil {
		t.Fatal("expected failure")
	}
	for _, secret := range []string{"user-secret", "internal-secret"} {
		if strings.Contains(logs.String()+err.Error(), secret) {
			t.Fatal("credential leaked in error or logs")
		}
	}
}

func TestExplicitAnalyticsModesAndAuthorizationValidation(t *testing.T) {
	for _, cfg := range []analytics.UserScopedConfig{
		{EndpointMode: "invalid"},
		{EndpointMode: analytics.EndpointModeStandalone},
		{EndpointMode: analytics.EndpointModeStandalone, BaseURL: "not a URL"},
		{EndpointMode: analytics.EndpointModeDiscoveryRequired},
		{EndpointMode: analytics.EndpointModeDiscoveryRequired, InternalName: "mango-mcp-server"},
	} {
		if _, err := analytics.NewUserScopedClient(cfg); err == nil {
			t.Fatal("expected invalid endpoint mode/configuration to fail")
		}
	}
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.Header.Get("X-API-KEY") != "" || r.Header.Get("X-INTERNAL-NAME") != "" {
			t.Error("standalone leaked internal credentials")
		}
		_, _ = io.WriteString(w, `{}`)
	}))
	defer server.Close()
	client, err := analytics.NewUserScopedClient(analytics.UserScopedConfig{
		EndpointMode: analytics.EndpointModeStandalone, BaseURL: server.URL,
		Resolver: privateResolverFunc(func(string) (*common.ServiceInstance, error) {
			t.Error("standalone attempted discovery")
			return nil, nil
		}),
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, auth := range []string{"", " ", "Basic token", "Bearer", "Bearer ", "raw-token", "Bearer token\r\nInjected: true"} {
		if _, err := client.GetGatewayFreeMemory(context.Background(), "router-a", "2026-10-21T10:00:00Z", 24, auth); err == nil {
			t.Fatal("invalid Bearer accepted")
		}
	}
	if _, err := client.GetGatewayFreeMemory(context.Background(), "router-a", "2026-10-21T10:00:00Z", 24, "Bearer original"); err != nil {
		t.Fatal(err)
	}
	if requests.Load() != 1 {
		t.Fatal("invalid authorization reached HTTP")
	}
}
