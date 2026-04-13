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

func (f *ServiceRpc) AnalyticsClient() *analytics.AnalyticsClient {
	return analytics.NewAnalyticsClient(f.deps)
}

func (f *ServiceRpc) SecurityClient() *owsec.SecurityClient {
	return owsec.NewSecurityClient(f.deps)
}
