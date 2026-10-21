# servicerpc

`servicerpc` provides service-to-service clients built on:
- service discovery (`servicediscovery`)
- a pluggable HTTP requester abstraction
- shared application errors (`apperrors`)

The module currently exposes:
- `AnalyticsClient` for `owanalytics`
- `SecurityClient` for `owsec`
- `UserScopedAnalyticsClient` for user-authorized analytics summary requests

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

Internal RPC clients **do not apply an internal request timeout**.

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

### User-scoped analytics

Create the client with `rpc.UserScopedAnalyticsClient(baseURL, opts...)` or
`analytics.NewUserScopedClient(cfg, opts...)`. All five summary methods require
a nonempty router ID and authorization, an RFC3339 `timestampTill` (for example
`2026-10-21T10:00:00Z`), and `lookbackHours > 0`. Timestamps may include fractional
seconds and timezone offsets. Invalid inputs return `CodeInvalidInput` before
any HTTP request; relative values such as `"now"` are rejected.

Retryable failures get two retries by default (three total attempts), matching
the Python MCP default. A positive `UserScopedConfig.MaxRetries` changes that
count; zero or negative configuration values use the default. Options override
the configuration. Disable retries explicitly through either constructor:

```go
client, err := rpc.UserScopedAnalyticsClient(baseURL, analytics.WithMaxRetries(0))
```

`WithMaxRetries(n)` selects an explicit retry count; nonpositive values disable
retries. Only transport/read failures and HTTP 429, 500, 502, 503, and 504 are
retried. HTTP 401 and 403 are not retried. Retries honor context cancellation,
use exponential backoff with jitter, and respect Retry-After up to MaxRetryDelay.
The per-attempt timeout defaults to five seconds; callers can bound the entire
operation with a context deadline. `FormatAnalyticsErrorMessage` exposes only
the HTTP status or a generic service-unavailable message.

See [Analytics HTTP test coverage](analytics/ANALYTICS_E2E_TEST_FLOW.md) for the
included regression and integration tests.

### Common transport (`servicerpc/common`)
- `NewServiceRPCBase(...)`
- `NewServiceRPCBaseWithDeps(...)`
- `(*ServiceRPCBase).Send(...)`
- `(*ServiceRPCBase).Logger()`

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
