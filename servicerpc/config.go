package servicerpc

import (
	"time"
)

type ServiceRpcConfig struct {
	TLSRootCA    string
	Timeout      time.Duration
	InternalName string
}
