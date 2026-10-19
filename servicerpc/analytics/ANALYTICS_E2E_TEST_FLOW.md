# `analytics_e2e_test.go` API Call Flow

This document explains what [`analytics_e2e_test.go`](/home/uttam/openwifi_workspace/uttam-repos/ow-common-mods/servicerpc/analytics/analytics_e2e_test.go) is testing and how the analytics client calls another service over HTTP.

## Purpose

The file is an end-to-end style test for the analytics RPC client.

It does not start the real `owanalytics` service.
Instead, it starts a local mock HTTP server with `httptest.NewServer(...)` and verifies that the client sends the same HTTP requests that a real caller service would send to `owanalytics`.

## Components Involved

The test wires together these pieces:

1. `analyticsE2EResolver`
   Returns a fixed `common.ServiceInstance`.
   This replaces real service discovery.

2. `common.ServiceRPCBase`
   Shared transport layer used by all service RPC clients.

3. `AnalyticsClient`
   The package client that builds analytics endpoints and parses responses.

4. `httptest` server
   Fake upstream service that receives HTTP requests and returns JSON.

## How the Call Reaches Another Service

For every client method, the flow is:

1. The caller creates `AnalyticsClient` with `NewAnalyticsClient(base)`.
2. The client method calls `base.Send(..., serviceName)`.
3. `ServiceRPCBase.Send` resolves the target service name `owanalytics`.
4. The resolver returns:
   - `PrivateEndPoint`: base URL of the target service
   - `Key`: API key for that service
5. `ServiceRPCBase.Send` builds the final URL as:
   `strings.TrimSuffix(service.PrivateEndPoint, "/") + endpoint`
6. It adds headers:
   - `X-API-KEY`
   - `X-INTERNAL-NAME`
7. The configured requester sends the HTTP request.
8. `AnalyticsClient` parses the JSON response and returns Go structs/slices.

Code references:

- RPC base send logic: [client_factory.go](/home/uttam/openwifi_workspace/uttam-repos/ow-common-mods/servicerpc/common/client_factory.go:167)
- Analytics client methods: [analytics.go](/home/uttam/openwifi_workspace/uttam-repos/ow-common-mods/servicerpc/analytics/analytics.go:32)

## What `TestAnalyticsClient_E2E_HTTPCalls` Verifies

The main test proves that three analytics client methods generate the expected outgoing HTTP calls.

### 1. `GetTimepoints`

Caller code:

```go
timepoints, err := client.GetTimepoints(context.Background(), TimepointRequest{
    BoardID:    "board-1",
    FromDate:   10,
    EndDate:    20,
    MaxRecords: 2,
})
```

Client method:
- [analytics.go](/home/uttam/openwifi_workspace/uttam-repos/ow-common-mods/servicerpc/analytics/analytics.go:32)

Outgoing request:

```http
GET /api/v1/board/board-1/timepoints?fromDate=10&endDate=20&maxRecords=2&statsOnly=false&pointsOnly=true&pointStatsOnly=true&LatestPerDevice=true
X-API-KEY: e2e-api-key
X-INTERNAL-NAME: e2e-caller
```

What the test checks:
- path is `/api/v1/board/board-1/timepoints`
- query contains at least:
  - `fromDate=10`
  - `endDate=20`
  - `maxRecords=2`
- required internal headers are present

Mock upstream response:

```json
{
  "points": [
    [{"id":"1","boardId":"board-1","timestamp":10}],
    [{"id":"2","boardId":"board-1","timestamp":20}]
  ]
}
```

Returned value:
- The client flattens the nested `points` buckets into a single `[]TimepointsData`.

### 2. `GetDeviceInfo`

Caller code:

```go
devices, err := client.GetDeviceInfo(context.Background(), "board-1")
```

Client method:
- [analytics.go](/home/uttam/openwifi_workspace/uttam-repos/ow-common-mods/servicerpc/analytics/analytics.go:104)

Outgoing request:

```http
GET /api/v1/board/board-1/devices
X-API-KEY: e2e-api-key
X-INTERNAL-NAME: e2e-caller
```

Mock upstream response:

```json
{
  "devices": [
    {
      "boardId": "board-1",
      "serialNumber": "SER1"
    }
  ]
}
```

Returned value:
- The client unmarshals `devices` into `[]DeviceInfo`.

### 3. `GetWifiClientHistoryMACs`

Caller code:

```go
macs, err := client.GetWifiClientHistoryMACs(context.Background(), "board-1", 2, 0)
```

Client method:
- [analytics.go](/home/uttam/openwifi_workspace/uttam-repos/ow-common-mods/servicerpc/analytics/analytics.go:134)

Outgoing request:

```http
GET /api/v1/wifiClientHistory?macsOnly=true&boardId=board-1&limit=2&offset=0
X-API-KEY: e2e-api-key
X-INTERNAL-NAME: e2e-caller
```

Mock upstream response:

```json
{
  "entries": ["aa:bb:cc:dd:ee:ff", "11:22:33:44:55:66"]
}
```

Returned value:
- The client unmarshals `entries` into `[]string`.

## What `TestAnalyticsClient_E2E_PathEscaping` Verifies

This test checks URL safety for path parameters.

Input board ID:

```text
 board ?x=1 
```

Expected outgoing path:

```text
/api/v1/board/board%20%3Fx=1/devices
```

Why this matters:
- spaces are trimmed before escaping
- special characters do not break the URL path
- a board ID cannot inject extra query parameters or path segments

Current code status:
- `GetTimepoints` explicitly uses `url.PathEscape(strings.TrimSpace(req.BoardID))`
- `GetDeviceInfo` currently builds `"/api/v1/board/"+boardID+"/devices"` directly
- because of that, this E2E test currently exposes a mismatch between expected behavior and implementation

Observed test result from `go test ./analytics`:

```text
expected path "/api/v1/board/board%20%3Fx=1/devices", got "/api/v1/board/%20board%20"
```

So this second test is useful as a regression test, but at the moment it is documenting a behavior that `GetDeviceInfo` does not yet fully implement.

## Important Clarification

This test shows how a service using `AnalyticsClient` calls the `owanalytics` service.

It does not directly call multiple downstream services from inside the test itself.
The "other service" here is the target analytics service resolved by service discovery and reached through `ServiceRPCBase.Send(...)`.

## Request Sequence Summary

```text
Caller service
  -> AnalyticsClient.GetTimepoints / GetDeviceInfo / GetWifiClientHistoryMACs
  -> ServiceRPCBase.Send(...)
  -> resolve("owanalytics")
  -> build URL = PrivateEndPoint + endpoint
  -> add X-API-KEY and X-INTERNAL-NAME
  -> HTTP GET to owanalytics
  -> parse JSON response
  -> return typed Go value
```

## Files to Read Together

- Test flow: [analytics_e2e_test.go](/home/uttam/openwifi_workspace/uttam-repos/ow-common-mods/servicerpc/analytics/analytics_e2e_test.go:22)
- Client implementation: [analytics.go](/home/uttam/openwifi_workspace/uttam-repos/ow-common-mods/servicerpc/analytics/analytics.go:17)
- Shared transport: [client_factory.go](/home/uttam/openwifi_workspace/uttam-repos/ow-common-mods/servicerpc/common/client_factory.go:19)
