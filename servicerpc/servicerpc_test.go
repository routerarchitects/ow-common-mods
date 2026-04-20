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
	if rpc.AnalyticsClient() == nil {
		t.Fatalf("expected analytics client")
	}
	if rpc.SecurityClient() == nil {
		t.Fatalf("expected security client")
	}
}
