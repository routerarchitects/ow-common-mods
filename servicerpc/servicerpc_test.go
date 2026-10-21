package servicerpc

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/routerarchitects/ow-common-mods/servicediscovery"
	"github.com/routerarchitects/ow-common-mods/servicerpc/analytics"
)

func TestNewServiceRpc_Validation(t *testing.T) {
	_, err := NewServiceRpc(&servicediscovery.Discovery{}, ServiceRpcConfig{
		TLSRootCA:    "",
		InternalName: "   ",
	}, slog.Default())
	if err == nil {
		t.Fatalf("expected error for empty internal name")
	}
}

func TestNewServiceRpc_TLSPathError(t *testing.T) {
	_, err := NewServiceRpc(&servicediscovery.Discovery{}, ServiceRpcConfig{
		TLSRootCA:    "/path/that/does/not/exist.pem",
		InternalName: "caller",
	}, slog.Default())
	if err == nil {
		t.Fatalf("expected error for invalid TLS root CA path")
	}
}

func TestNewServiceRpc_Wiring(t *testing.T) {
	rpc, err := NewServiceRpc(&servicediscovery.Discovery{}, ServiceRpcConfig{
		InternalName: "caller",
	}, nil)
	if err != nil {
		t.Fatalf("unexpected constructor error: %v", err)
	}
	if rpc == nil {
		t.Fatalf("expected rpc instance")
	}
	if client, err := rpc.AnalyticsClient(); err != nil || client == nil {
		t.Fatalf("expected analytics client")
	}
	if client, err := rpc.SecurityClient(); err != nil || client == nil {
		t.Fatalf("expected security client")
	}
}

func TestUserScopedAnalyticsClient_TLSRootCA(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/devices/gw-1/memory-summary" {
			t.Errorf("unexpected request path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"trusted":true}`))
	}))
	defer server.Close()

	caPath := filepath.Join(t.TempDir(), "root-ca.pem")
	caPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw})
	if err := os.WriteFile(caPath, caPEM, 0600); err != nil {
		t.Fatalf("write test CA: %v", err)
	}

	for _, tc := range []struct {
		name      string
		tlsRootCA string
		trusted   bool
	}{
		{name: "configured CA", tlsRootCA: caPath, trusted: true},
		{name: "unconfigured CA"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rpc, err := NewServiceRpc(&servicediscovery.Discovery{}, ServiceRpcConfig{
				TLSRootCA:    tc.tlsRootCA,
				InternalName: "caller",
			}, nil)
			if err != nil {
				t.Fatalf("create service RPC: %v", err)
			}

			// Use the explicit URL without consulting the uninitialized discovery store.
			client, err := rpc.UserScopedAnalyticsClient(server.URL, analytics.WithResolver(nil))
			if err != nil {
				t.Fatalf("create user-scoped client: %v", err)
			}
			defer client.(io.Closer).Close()

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			result, err := client.GetGatewayFreeMemory(ctx, "gw-1", "2026-10-21T10:00:00Z", 1, "Bearer test-token")
			if !tc.trusted {
				var unknownAuthority x509.UnknownAuthorityError
				if !errors.As(err, &unknownAuthority) {
					t.Fatalf("expected untrusted CA error, got %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("request with configured CA failed: %v", err)
			}
			if body, ok := result.(map[string]any); !ok || body["trusted"] != true {
				t.Fatalf("unexpected response: %v", result)
			}
		})
	}
}

func TestUserScopedAnalyticsClient_RetryPolicy(t *testing.T) {
	for _, tc := range []struct {
		name     string
		opts     []analytics.UserScopedOption
		attempts int32
	}{
		{name: "default", attempts: 3},
		{name: "disabled", opts: []analytics.UserScopedOption{analytics.WithMaxRetries(0)}, attempts: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var attempts atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				attempts.Add(1)
				w.WriteHeader(http.StatusServiceUnavailable)
			}))
			defer server.Close()
			rpc, err := NewServiceRpc(&servicediscovery.Discovery{}, ServiceRpcConfig{InternalName: "caller"}, nil)
			if err != nil {
				t.Fatalf("create service RPC: %v", err)
			}
			opts := append([]analytics.UserScopedOption{
				analytics.WithSleeper(func(context.Context, time.Duration) error { return nil }),
			}, tc.opts...)
			client, err := rpc.UserScopedAnalyticsClient(server.URL, opts...)
			if err != nil {
				t.Fatalf("create user-scoped client: %v", err)
			}
			defer client.(io.Closer).Close()
			if _, err := client.GetGatewayFreeMemory(context.Background(), "gw-1", "2026-10-21T10:00:00Z", 1, "Bearer tok"); err == nil {
				t.Fatal("expected error after retry exhaustion")
			}
			if got := attempts.Load(); got != tc.attempts {
				t.Errorf("HTTP attempts = %d, want %d", got, tc.attempts)
			}
		})
	}
}
