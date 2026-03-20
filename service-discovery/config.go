package servicediscovery

import (
	"time"
)

// OrderingStrategy controls how instances are ordered in GetServiceInstances.
type OrderingStrategy string

const (
	OrderingRoundRobin    OrderingStrategy = "round-robin"
	OrderingLastSeen      OrderingStrategy = "last-seen"
	OrderingLatestVersion OrderingStrategy = "latest-version"
	OrderingNone          OrderingStrategy = "none"
)

// Config holds environment-based configuration for service discovery.
type Config struct {
	// Topic is the Kafka topic name used for broadcasting and consuming service discovery messages.
	Topic string `env:"DISCOVERY_TOPIC,required"`

	// ServiceType is the name of the service (e.g., "auth-service", "payment-service").
	ServiceType string `env:"DISCOVERY_SERVICE_TYPE,required"`
	// ServiceVersion indicates the version of the service deployment (e.g., "v1.0.0").
	ServiceVersion string `env:"DISCOVERY_SERVICE_VERSION,required"`
	// PrivateEndpoint is the internal network address (host:port) where the service listens.
	PrivateEndpoint string `env:"DISCOVERY_PRIVATE_ENDPOINT,required"`
	// PublicEndpoint is the external network address (host:port) accessible to clients, if applicable.
	PublicEndpoint string `env:"DISCOVERY_PUBLIC_ENDPOINT,required"`

	// InstanceID is an optional stable unique identifier for the instance.
	// If not provided, a random ID is generated at startup.
	InstanceID int64 `env:"DISCOVERY_INSTANCE_ID"`
	// InstanceKey is an optional stable key string for the instance.
	// If not provided, a random key is generated at startup.
	InstanceKey string `env:"DISCOVERY_INSTANCE_KEY"`

	// KeepAliveInterval is the duration between sending heartbeat messages to the discovery topic.
	KeepAliveInterval time.Duration `env:"DISCOVERY_KEEPALIVE_INTERVAL" envDefault:"10s"`
	// ExpiryMultiplier determines the timeout for considering an instance offline.
	// Timeout = KeepAliveInterval * ExpiryMultiplier.
	ExpiryMultiplier int `env:"DISCOVERY_EXPIRY_MULTIPLIER" envDefault:"2"`
	// SweepInterval is the frequency at which the local registry removes expired instances.
	SweepInterval time.Duration `env:"DISCOVERY_SWEEP_INTERVAL" envDefault:"5s"`

	// Ordering defines the strategy for sorting service instances when identifying the "best" instance.
	Ordering OrderingStrategy `env:"DISCOVERY_ORDERING" envDefault:"last-seen"`
}
