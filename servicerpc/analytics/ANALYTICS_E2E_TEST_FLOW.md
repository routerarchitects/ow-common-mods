# Analytics HTTP test coverage

The included tests use local HTTP/HTTPS servers or injected requesters. They do
not contact a deployed OWANALYTICS service or require Kafka.

## User-scoped request flow

The client in [user_scoped.go](user_scoped.go) and [summary.go](summary.go):

1. Validates the router ID, authorization, RFC3339 timestamp, and positive lookback.
2. Uses the explicit base URL, or resolves the public endpoint through discovery.
3. Escapes the router ID and encodes `timestampTill` and `lookbackHours` as query parameters.
4. Sends the caller's bearer token without internal RPC authentication headers.
5. Parses JSON, preserving numbers, or returns an `APIClientError`.

[User-scoped HTTP tests](user_scoped_test.go) include:

- `TestUserScopedClient_AllFiveEndpoints`: verifies all five summary paths.
- `TestUserScopedClient_CorrectQueryParameters`: checks timestamp and lookback parameters.
- `TestUserScopedClient_BearerTokenAndNoInternalAPIKey`: checks user authentication headers.
- `TestUserScopedClient_URLEscaping`: checks escaped router IDs.
- `TestUserScopedClient_RetryBehavior`: checks recovery from 5xx errors and handling of Retry-After.
- `TestUserScopedClient_HTTP401And403Handling`: checks that authentication failures are not retried.
- `TestFormatAnalyticsErrorMessage_OmitsUpstreamBody`: checks that formatted errors exclude upstream payloads.

[Policy tests](user_scoped_policy_test.go) verify the default and explicit retry
counts, rejection of invalid input before HTTP requests, and accepted timestamp
formats. Two retries (three total attempts) are the default; `WithMaxRetries(0)`
disables them. See the [module README](../README.md#user-scoped-analytics) for configuration.

[Factory integration tests](../servicerpc_test.go) include
`TestUserScopedAnalyticsClient_TLSRootCA`, which verifies HTTPS succeeds with a
custom trusted CA and fails without it, and
`TestUserScopedAnalyticsClient_RetryPolicy`, which checks the factory's default
retry count and explicit opt-out.

## Internal RPC request flow

The internal [AnalyticsClient](analytics.go) sends requests through
[ServiceRPCBase](../common/client_factory.go), which resolves the private endpoint
and adds `X-API-KEY` and `X-INTERNAL-NAME`.

`TestExistingInternalAnalyticsRPC_RemainsFunctional` in
[user_scoped_test.go](user_scoped_test.go) uses an HTTP server and the real Fiber
requester to check `GetDeviceInfo`, response decoding, and internal headers.

[analytics_test.go](analytics_test.go) uses injected resolvers and requesters to
check `GetTimepoints`, `GetDeviceInfo`, and `GetWifiClientHistoryMACs`.
`TestGetDeviceInfo_PathEscaping` and `TestGetTimepoints_PathAndQueryEscaping`
verify escaped board IDs. `GetDeviceInfo` trims and escapes board IDs with
`url.PathEscape` before building the request path.

## Run the included tests

From the `servicerpc` module directory:

```sh
go test ./... -count=1
```

This command runs the package tests described above. Live service validation
requires a separate environment and is outside these tests' coverage.
