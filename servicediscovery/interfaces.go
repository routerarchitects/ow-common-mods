package servicediscovery

import "context"

// Store provides read access to discovered instances.
type Store interface {
	// GetServiceInstances returns one selected instance based on ordering.
	GetServiceInstances(serviceType string) *Instance
	// GetAllServiceInstances returns all discovered instances based on ordering.
	GetAllServiceInstances(serviceType string) []Instance
}

// Service defines the external contract for service discovery.
type Service interface {
	Start(parent context.Context) error
	Stop(ctx context.Context) error
	Store() Store
	Self() Instance
	PublishNow(ctx context.Context, event EventType) error
	SetOrdering(ordering OrderingStrategy)
}

var _ Service = (*Discovery)(nil)
var _ Store = (*store)(nil)
