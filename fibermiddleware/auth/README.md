# Auth Middleware (`fibermiddleware/auth`)

This package provides reusable Fiber middlewares for:

- Internal/private API key authentication
- Public auth with either bearer token or API key authentication

## Installation

```bash
go get github.com/routerarchitects/ow-common-mods/fibermiddleware
```

## Internal Auth (`RequireInternalAPIKey`)

Use this middleware for service-to-service/private endpoints.

### Validation Flow

1. Reads internal caller header (`X-INTERNAL-NAME` by default)
2. Optionally validates caller name against `AllowedInternalName`
3. Reads API key header (`X-API-KEY` by default)
4. Compares with configured `ExpectedAPIKey` (constant-time comparison)

If any step fails, request is rejected with `401` (or custom `OnUnauthorized` handler).

### Config

- `InternalNameHeader` (default: `X-INTERNAL-NAME`)
- `APIKeyHeader` (default: `X-API-KEY`)
- `AllowedInternalName` (optional single allowed caller name)
- `ExpectedAPIKey` (**required**)
- `OnUnauthorized` (optional custom unauthorized writer)

### Example

```go
app.Use(auth.RequireInternalAPIKey(auth.InternalAPIKeyConfig{
    ExpectedAPIKey:      os.Getenv("TOPOLOGY_PRIVATE_API_KEY"),
    AllowedInternalName: "topology-service",
}))
```

## Public Auth (`RequirePublicAuth`)

Use this middleware for public endpoints with dynamic credential handling:

1. If `X-API-KEY` is present, API-key validation is attempted first.
2. If `Authorization: Bearer <token>` is present, bearer validation is attempted.
3. If both are present, both are attempted in order until one succeeds.

### Validation Flow

1. Reads API key header (`X-API-KEY` by default)
2. Reads authorization header (`Authorization` by default)
3. Validates credentials against:
   - `Validator.ValidateAPIKey(ctx, apiKey)`
   - `Validator.ValidateToken(ctx, token)`

If validation fails, request is rejected with `401` (or custom `OnValidationError`/`OnUnauthorized`).

### Config

- `AuthorizationHeader` (default: `Authorization`)
- `BearerPrefix` (default: `Bearer `)
- `APIKeyHeader` (default: `X-API-KEY`)
- `Validator` (**required**)
- `OnUnauthorized` (optional custom unauthorized writer)
- `OnValidationError` (optional custom validator error mapper)

### Example

```go
type authServiceValidator struct {
    client AuthRPCClient
}

func (v authServiceValidator) ValidateToken(ctx context.Context, token string) error {
    return v.client.ValidateToken(ctx, token)
}

func (v authServiceValidator) ValidateAPIKey(ctx context.Context, apiKey string) error {
    return v.client.ValidateAPIKey(ctx, apiKey)
}

app.Use(auth.RequirePublicAuth(auth.PublicAuthConfig{
    Validator: authServiceValidator{client: authClient},
}))
```

## Important Notes

- `RequireInternalAPIKey` panics if `ExpectedAPIKey` is empty.
- `RequirePublicAuth` panics if `Validator` is nil.
- Raw `Authorization: <token>` is rejected; bearer auth requires `Authorization: Bearer <token>`.
