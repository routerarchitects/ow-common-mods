package servicerpc

import (
	"log/slog"

	"github.com/routerarchitects/ow-common-mods/servicediscovery"
	"github.com/routerarchitects/ow-common-mods/servicerpc/analytics"
	"github.com/routerarchitects/ow-common-mods/servicerpc/common"
	"github.com/routerarchitects/ow-common-mods/servicerpc/owsec"
)

type ServiceRpc struct {
	deps *common.ServiceRPCBase
}

func NewServiceRpc(
	discovery *servicediscovery.Discovery,
	cfg ServiceRpcConfig,
	logger *slog.Logger,
) *ServiceRpc {
	return &ServiceRpc{
		deps: common.NewServiceRPCBase(discovery, cfg.TLSRootCA, cfg.Timeout, cfg.InternalName, logger),
	}
}

func (f *ServiceRpc) AnalyticsClient() *analytics.AnalyticsClient {
	return analytics.NewAnalyticsClient(f.deps)
}

func (f *ServiceRpc) Validator() *owsec.Validator {
	return owsec.NewValidator(f.deps)
}
