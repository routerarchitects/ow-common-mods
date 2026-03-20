# Service Discovery Module

This module provides a robust, Kafka-based service discovery mechanism for Go microservices. It allows services to broadcast their presence, discover other services, and maintain a local implementation of a service registry.

## Features

- **Kafka-based**: Uses Kafka topics for broadcasting `join`, `leave`, and `keep-alive` events.
- **Decentralized**: Each service instance maintains its own local view of the network.
- **Load Balancing Strategies**: Supports `round-robin`, `last-seen`, and `latest-version` ordering for client-side load balancing.
- **Failover**: Automatically removes instances that fail to send heartbeats within a configured timeout.

## Installation

```bash
go get github.com/routerarchitects/ow-common-mods/servicediscovery
```

## Usage

### 1. Configuration

The module uses environment variables. Ensure your configuration struct embeds or loads the necessary values.

### 2. Initialization

```go
package main

import (
	"context"
	"log/slog"
	"os"
	"time"

	"github.com/caarlos0/env/v11"
	"github.com/routerarchitects/ra-common-mods/kafka"
	sd "github.com/routerarchitects/ow-common-mods/servicediscovery"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	// 1. Load Config
	var dcfg sd.Config
	if err := env.Parse(&dcfg); err != nil {
		panic(err)
	}

	// 2. Setup Kafka Config (Standard ra-common-mods/kafka)
	kcfg := kafka.Config{
		Brokers: []string{"localhost:9092"},
		// ... other kafka settings
	}

	// 3. Create Discovery Agent via interface
	discovery, err := sd.NewService(dcfg, kcfg, logger)
	if err != nil {
		panic(err)
	}

	// 4. Start Discovery (Background)
	ctx := context.Background()
	if err := discovery.Start(ctx); err != nil {
		panic(err)
	}
	defer discovery.Stop(ctx)

	// 5. Lookup one service instance (selected by configured ordering strategy)
	ticker := time.NewTicker(5 * time.Second)
	for range ticker.C {
		inst := discovery.Store().GetServiceInstances("payment-service")
		if inst != nil {
			logger.Info("Selected service", "id", inst.ID, "endpoint", inst.PrivateEndPoint)
		}
	}
}
```

## Environment Variables

| Variable | Description | Default | Required |
|----------|-------------|---------|:--------:|
| `DISCOVERY_TOPIC` | Kafka topic used for discovery events. | - | **Yes** |
| `DISCOVERY_SERVICE_TYPE` | Name of the service (e.g., `auth-service`). | - | **Yes** |
| `DISCOVERY_SERVICE_VERSION` | Semantic version of the service (e.g., `1.0.0`). | - | **Yes** |
| `DISCOVERY_PRIVATE_ENDPOINT` | Internal address (`ip:port`) reachable by other services. | - | **Yes** |
| `DISCOVERY_PUBLIC_ENDPOINT` | External address (`ip:port`) reachable by clients. | - | **Yes** |
| `DISCOVERY_INSTANCE_ID` | Stable ID (int64). Generated randomly if omitted. | - | No |
| `DISCOVERY_INSTANCE_KEY` | Stable secret key. Generated randomly if omitted. | - | No |
| `DISCOVERY_KEEPALIVE_INTERVAL` | Heartbeat frequency. | `10s` | No |
| `DISCOVERY_EXPIRY_MULTIPLIER` | Multiplier for timeout calculation (`Interval * Multiplier`). | `2` | No |
| `DISCOVERY_SWEEP_INTERVAL` | How often to sweep for expired instances. | `5s` | No |
| `DISCOVERY_ORDERING` | Strategy for list ordering: `last-seen`, `round-robin`, `latest-version`, `none`. | `last-seen` | No |

## Ordering Strategies

- **`last-seen`**: Sorts instances by the most recent heartbeat. Useful for active-active where you prefer the freshest instance.
- **`round-robin`**: Rotates the selected instance on every call to `GetServiceInstances`, providing simple client-side load balancing.
- **`latest-version`**: prioritizing instances with the highest semantic version (e.g., traffic shifting during blue-green deployments).
- **`none`**: No ordering. Returned order is not guaranteed (implementation-dependent).

## Store API

- `GetServiceInstances(serviceType)` returns one selected instance according to `DISCOVERY_ORDERING`.
- `GetAllServiceInstances(serviceType)` returns all discovered instances according to `DISCOVERY_ORDERING`.

## Interfaces

- `Service` is the public interface for lifecycle, publishing, identity, and store access.
- `Store` is the public interface for instance lookup methods.
- Use `NewService(...)` to get the module as the `Service` interface.

## Singleton Behavior

- This module is singleton per process.
- Calling `New(...)` or `NewService(...)` more than once without stopping the existing instance returns `ErrSingletonAlreadyCreated`.
- If `Start()` fails, applications are expected to treat that as a fatal startup error for the created singleton instance (do not create another instance as a retry strategy).
