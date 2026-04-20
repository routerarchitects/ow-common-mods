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
- `(*ServiceRpc).AnalyticsClient()`
- `(*ServiceRpc).SecurityClient()`

### Analytics (`servicerpc/analytics`)
- `GetTimepoints(ctx, req)`
- `GetDeviceInfo(ctx, boardID)`
- `GetWifiClientHistoryMACs(ctx, boardID, limit, offset)`

### Security (`servicerpc/owsec`)
- `ValidateToken(ctx, rawToken)`
  - validates by trying both owsec APIs in sequence:
  - `GET /api/v1/validateSubToken?token=...`
  - `GET /api/v1/validateToken?token=...`
  - returns success if either endpoint returns `200`
  - if any error occurs in `validateSubToken`, `validateToken` is not attempted

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
- dual-endpoint behavior in `SecurityClient.ValidateToken`
