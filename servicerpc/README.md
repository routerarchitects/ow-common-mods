# servicerpc

`servicerpc` provides service-to-service clients built on:
- service discovery (`servicediscovery`)
- a pluggable HTTP requester abstraction
- shared application errors (`apperrors`)

The module currently exposes:
- `AnalyticsClient` for `owanalytics`
- `SecurityClient` for `owsec`

## Core Design

- Interface-based dependency injection for transport boundaries:
  - `common.ServiceResolver`
  - `common.Requester`
  - `common.Response`
- Default production wiring:
  - discovery-backed resolver via `common.NewDiscoveryResolver(...)`
  - Fiber HTTP requester via `common.NewFiberRequester(...)`
- Errors are returned (no panics) using `apperrors`.

## Timeout Contract (Important)

This module **does not apply an internal request timeout**.

Caller context is the single source of cancellation and timeout behavior.
Always pass a context with deadline/timeout (for example `context.WithTimeout`).

If `ctx` is `nil`, APIs return `apperrors.CodeInvalidInput`.

## Quick Start

```go
ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
defer cancel()

rpc, err := servicerpc.NewServiceRpc(discovery, servicerpc.ServiceRpcConfig{
    TLSRootCA:    "/etc/ssl/custom-root-ca.pem", // optional
    InternalName: "owmyservice",                 // required
}, logger)
if err != nil {
    return err
}

timepoints, err := rpc.AnalyticsClient().GetTimepoints(ctx, analytics.TimepointRequest{
    BoardID:    "board-1",
    FromDate:   1710000000,
    EndDate:    1710003600,
    MaxRecords: 500,
})
if err != nil {
    return err
}
_ = timepoints
```

## API Summary

### Top-level package (`servicerpc`)
- `NewServiceRpc(discovery, cfg, logger) (*ServiceRpc, error)`
- `NewServiceRpcWithBase(base) *ServiceRpc` for dependency injection
- `(*ServiceRpc).Base()`
- `(*ServiceRpc).Send(ctx, method, endpoint, body, serviceName)`
- `(*ServiceRpc).AnalyticsClient()`
- `(*ServiceRpc).SecurityClient()`

### Analytics (`servicerpc/analytics`)
- `GetTimepoints(ctx, req)`
- `GetDeviceInfo(ctx, boardID)`
- `GetWifiClientHistoryMACs(ctx, boardID, limit, offset)`

### Security (`servicerpc/owsec`)
- `ValidateToken(ctx, rawToken)`
  - strictly validates regular/administrator tokens by calling:
  - `GET /api/v1/validateToken?token=...`
  - returns success if the endpoint returns `200`
  - rejects subscriber tokens by default (does not call `/api/v1/validateSubToken`)

### Analytics input notes
- `GetWifiClientHistoryMACs(ctx, boardID, limit, offset)` requires:
  - non-empty `boardID`
  - `limit > 0`
  - `offset >= 0`

### Common transport (`servicerpc/common`)
- `NewServiceRPCBase(...)`
- `NewServiceRPCBaseWithDeps(...)`
- `(*ServiceRPCBase).Send(...)`
- `(*ServiceRPCBase).Logger()`
- `(*ServiceRPCBase).Resolver()`

`common.ServiceInstance` exposes `Key`, `PrivateEndPoint`, and `PublicEndPoint`.
Internal RPC uses the private endpoint and service key. Applications can reuse
`common.NewDiscoveryResolver` for service metadata while owning their own
service-specific HTTP clients, authorization policies, and response handling.

## Extension Points

Use custom DI when needed:
- custom resolver (DNS/static/etc.) by implementing `common.ServiceResolver`
- custom HTTP transport (retries, tracing, mock transport) by implementing `common.Requester`

This is especially useful for unit testing client behavior without live discovery/network.

## Request Behavior

`ServiceRPCBase.Send(...)`:
- validates required inputs
- resolves target service by name
- sets headers:
  - `X-API-KEY` (from resolved instance key)
  - `X-INTERNAL-NAME` (from config)
- sets `Content-Type: application/json` for non-empty request body
- returns wrapped `apperrors` on failures

## Configuration

`ServiceRpcConfig`:
- `TLSRootCA` (optional): PEM file path for custom root CA trust
- `InternalName` (required): sent in `X-INTERNAL-NAME` header

## Breaking Changes (Current)

- Client APIs now require `context.Context`.
- Internal module timeout config was removed; caller context controls timeout.
- Construction can fail early with validation/TLS errors (`error` return).
- `Validator` was replaced by `SecurityClient`.

## Testing Guidance

Recommended unit tests:
- resolver failures (`not found`, `invalid resolver`)
- requester failures and status-code mapping
- context timeout/cancellation propagation from caller
- single-endpoint behavior in `SecurityClient.ValidateToken`

## Next release: compatibility with v0.5.0

The published `servicerpc/v0.5.0` tag includes a user-scoped Analytics HTTP
client that is absent from this branch. This is a source-breaking API removal
for consumers of `analytics.UserScopedAnalyticsClient`, `NewUserScopedClient`,
`UserScopedConfig`, `UserScopedOption`, its `With*` options, `APIClientError`
(including `ApiClientError`), `FormatAnalyticsErrorMessage`, `Sleeper`,
`JitterFunc`, `ClockFunc`, or `ServiceRpc.UserScopedAnalyticsClient`. Relative
to v0.5.0, the Analytics-only `EndpointMode` type/constants and
`ValidatePrivateIdentity` are also absent. Existing internal RPC clients and
generic discovery/factory APIs are retained.

Consumer audit on 2026-10-23:

| Consumer checked | Declared servicerpc version | Finding |
| --- | --- | --- |
| [ra-openlan-nw-topology](https://github.com/routerarchitects/ra-openlan-nw-topology/blob/main/go.mod) | v0.2.0 | No removed-client references found on the indexed default branch |
| [mango-go-foundation-service](https://github.com/routerarchitects/mango-go-foundation-service/blob/main/go.mod) | v0.3.0 | No removed-client references found on the indexed default branch |
| [mango-parental-control](https://github.com/routerarchitects/mango-parental-control/blob/main/go.mod) | v0.3.0 | No removed-client references found on the indexed default branch |
| [mango-mdu-service](https://github.com/routerarchitects/mango-mdu-service/blob/main/go.mod) | v0.2.0 | No removed-client references found on the indexed default branch |
| [mango-billing-service](https://github.com/routerarchitects/mango-billing-service/blob/main/go.mod) | v0.2.0 | No removed-client references found on the indexed default branch |
| [mango-mcp-server PR #5](https://github.com/routerarchitects/mango-mcp-server/pull/5) at `9497b38a9153b4b6fa30e10a718d6223046f5a54` | v0.5.0 | Inspected Analytics and internal RPC clients use MCP-local Analytics APIs and retained shared infrastructure |

The audit searched accessible indexed `routerarchitects` default branches for
`UserScopedAnalyticsClient`, `NewUserScopedClient`, and `UserScopedConfig`, and
searched the available local service checkouts. No consumer of the removed
shared client was found. The dependency versions above were read from GitHub
`go.mod` files; they are declared requirements, not verified deployment module
graphs. This audit does not cover every active branch, unindexed/private
repository, external consumer, or deployed version.

Before publishing the next module version, repeat the consumer search at the
release candidate, check active service branches and effective module graphs,
and record service-owner confirmation for consumers outside the search scope.
Keep this release prerequisite open until those checks are complete.

Do not publish this removal as a v0.5.x compatible patch or rewrite an existing
tag. Use a new pre-v1 minor version (recommended `servicerpc/v0.6.0`) and explicit
breaking-change release notes listing the removed APIs. Existing consumers can
remain pinned to v0.5.0 until migrated. Move any remaining users to an
application-owned Analytics client before upgrading; the MCP implementation is
local to MCP and is not a replacement shared-library import. Its five summary
operations, authentication, transport, and error handling are owned by MCP.
