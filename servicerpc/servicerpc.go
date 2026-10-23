package servicerpc

import (
	"context"
	"io"
	"log/slog"

	"github.com/routerarchitects/ow-common-mods/servicediscovery"
	"github.com/routerarchitects/ow-common-mods/servicerpc/analytics"
	"github.com/routerarchitects/ow-common-mods/servicerpc/common"
	"github.com/routerarchitects/ow-common-mods/servicerpc/owsec"
	"github.com/routerarchitects/ra-common-mods/apperror"
)

type ServiceRpc struct {
	deps *common.ServiceRPCBase
}

// NewServiceRpc constructs service clients backed by discovery + HTTP requester.
//
// All runtime API calls from returned clients expect caller-provided ctx with
// timeout/deadline.
func NewServiceRpc(
	discovery *servicediscovery.Discovery,
	cfg ServiceRpcConfig,
	logger *slog.Logger,
) (*ServiceRpc, error) {
	base, err := common.NewServiceRPCBase(discovery, cfg.TLSRootCA, cfg.InternalName, logger)
	if err != nil {
		return nil, err
	}
	return &ServiceRpc{deps: base}, nil
}

// NewServiceRpcWithBase constructs a ServiceRpc instance backed by an existing ServiceRPCBase.
func NewServiceRpcWithBase(base *common.ServiceRPCBase) *ServiceRpc {
	return &ServiceRpc{deps: base}
}

// Base returns the underlying ServiceRPCBase instance.
func (f *ServiceRpc) Base() *common.ServiceRPCBase {
	if f == nil {
		return nil
	}
	return f.deps
}

// Send dispatches an internal RPC request using the underlying ServiceRPCBase.
func (f *ServiceRpc) Send(ctx context.Context, method, endpoint string, body io.Reader, serviceName string) (common.Response, error) {
	if f == nil || f.deps == nil {
		return nil, apperror.New(apperror.CodeInternal, "service rpc is nil")
	}
	return f.deps.Send(ctx, method, endpoint, body, serviceName)
}

func (f *ServiceRpc) AnalyticsClient() (*analytics.AnalyticsClient, error) {
	return analytics.NewAnalyticsClient(f.deps)
}

func (f *ServiceRpc) SecurityClient() (*owsec.SecurityClient, error) {
	return owsec.NewSecurityClient(f.deps)
}
