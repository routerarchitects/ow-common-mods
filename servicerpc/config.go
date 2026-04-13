package servicerpc

type ServiceRpcConfig struct {
	TLSRootCA string
	// InternalName is sent as X-INTERNAL-NAME header on every request.
	InternalName string
}
