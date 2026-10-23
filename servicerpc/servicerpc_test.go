package servicerpc

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/routerarchitects/ow-common-mods/servicediscovery"
	"github.com/routerarchitects/ow-common-mods/servicerpc/common"
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

type factoryResolver struct {
	instance    common.ServiceInstance
	serviceName string
}

func (r *factoryResolver) Resolve(serviceName string) (*common.ServiceInstance, error) {
	r.serviceName = serviceName
	return &r.instance, nil
}

func TestNewServiceRpcWithBase_InternalRequest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/internal/test" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("X-API-KEY") != "service-key" || r.Header.Get("X-INTERNAL-NAME") != "caller" {
			t.Error("internal authentication headers were not preserved")
		}
		body, err := io.ReadAll(r.Body)
		if err != nil || string(body) != `{"value":1}` {
			t.Errorf("unexpected request body: %s, error: %v", body, err)
		}
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()
	resolver := &factoryResolver{instance: common.ServiceInstance{
		Key: "service-key", PrivateEndPoint: server.URL, PublicEndPoint: "https://public.invalid",
	}}
	requester, err := common.NewFiberRequester("")
	if err != nil {
		t.Fatal(err)
	}
	base, err := common.NewServiceRPCBaseWithDeps(resolver, requester, "caller", nil)
	if err != nil {
		t.Fatal(err)
	}
	rpc := NewServiceRpcWithBase(base)
	if rpc.Base() != base || rpc.Base().Resolver() != resolver {
		t.Fatal("injected base and resolver were not preserved")
	}
	if client, err := rpc.AnalyticsClient(); err != nil || client == nil {
		t.Fatalf("create analytics client: %v", err)
	}
	if client, err := rpc.SecurityClient(); err != nil || client == nil {
		t.Fatalf("create security client: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	response, err := rpc.Send(ctx, http.MethodPost, "/internal/test", strings.NewReader(`{"value":1}`), "example-service")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Close()
	if resolver.serviceName != "example-service" || response.StatusCode() != http.StatusAccepted || string(response.Body()) != `{"ok":true}` {
		t.Fatalf("unexpected response from %q: %d %s", resolver.serviceName, response.StatusCode(), response.Body())
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
