package servicerpc

import (
	"log/slog"
	"testing"

	"github.com/routerarchitects/ow-common-mods/servicediscovery"
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
