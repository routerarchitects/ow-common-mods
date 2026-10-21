package analytics

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/routerarchitects/ow-common-mods/servicerpc/common"
	"github.com/routerarchitects/ra-common-mods/apperror"
)

// UserScopedOption configures userScopedClient.
type UserScopedOption func(*userScopedClient)

// WithHTTPClient overrides the internal http.Client (e.g. for testing with httptest).
func WithHTTPClient(client *http.Client) UserScopedOption {
	return func(c *userScopedClient) {
		c.client = client
	}
}

// WithSleeper sets a custom sleeper function.
func WithSleeper(s Sleeper) UserScopedOption {
	return func(c *userScopedClient) {
		c.sleeper = s
	}
}

// WithJitter sets a custom jitter function.
func WithJitter(j JitterFunc) UserScopedOption {
	return func(c *userScopedClient) {
		c.jitter = j
	}
}

// WithClock sets a custom clock function.
func WithClock(clk ClockFunc) UserScopedOption {
	return func(c *userScopedClient) {
		c.clock = clk
	}
}

// WithBaseURL sets or overrides the base URL.
func WithBaseURL(baseURL string) UserScopedOption {
	return func(c *userScopedClient) {
		c.baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	}
}

// WithResolver sets or overrides the service resolver.
func WithResolver(resolver common.ServiceResolver) UserScopedOption {
	return func(c *userScopedClient) {
		c.resolver = resolver
	}
}

// WithLogger sets the logger.
func WithLogger(logger *slog.Logger) UserScopedOption {
	return func(c *userScopedClient) {
		c.logger = logger
	}
}

type userScopedClient struct {
	cfg           UserScopedConfig
	baseURL       string
	resolver      common.ServiceResolver
	logger        *slog.Logger
	client        *http.Client
	transport     *http.Transport
	sleeper       Sleeper
	jitter        JitterFunc
	clock         ClockFunc
	timeout       time.Duration
	maxRetries    int
	maxRetryDelay time.Duration
	mu            sync.Mutex
	rng           *rand.Rand
	rngMu         sync.Mutex
}

// NewUserScopedClient creates a new UserScopedAnalyticsClient.
func NewUserScopedClient(cfg UserScopedConfig, opts ...UserScopedOption) (UserScopedAnalyticsClient, error) {
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 5 * time.Second
	}

	maxRetries := cfg.MaxRetries
	if maxRetries < 0 {
		maxRetries = 2
	}

	maxRetryDelay := cfg.MaxRetryDelay
	if maxRetryDelay <= 0 {
		maxRetryDelay = 5 * time.Second
	}

	var resolver common.ServiceResolver
	if cfg.Resolver != nil {
		resolver = cfg.Resolver
	} else if cfg.Discovery != nil {
		resolver = common.NewDiscoveryResolver(cfg.Discovery)
	}

	c := &userScopedClient{
		cfg:           cfg,
		baseURL:       strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/"),
		resolver:      resolver,
		logger:        cfg.Logger,
		timeout:       timeout,
		maxRetries:    maxRetries,
		maxRetryDelay: maxRetryDelay,
		clock:         time.Now,
		rng:           rand.New(rand.NewSource(time.Now().UnixNano())),
		sleeper: func(ctx context.Context, d time.Duration) error {
			timer := time.NewTimer(d)
			defer timer.Stop()
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-timer.C:
				return nil
			}
		},
	}

	c.jitter = func(base float64) float64 {
		c.rngMu.Lock()
		defer c.rngMu.Unlock()
		return c.rng.Float64() * (base * 0.25)
	}

	if cfg.HTTPClient != nil {
		c.client = cfg.HTTPClient
	}

	for _, opt := range opts {
		opt(c)
	}

	if c.client == nil {
		if err := c.initHTTP(); err != nil {
			return nil, err
		}
	} else {
		origCheckRedirect := c.client.CheckRedirect
		cloned := *c.client
		cloned.CheckRedirect = func(req *http.Request, via []*http.Request) error {
			if len(via) > 0 {
				initialURL := via[0].URL
				if !strings.EqualFold(initialURL.Scheme, req.URL.Scheme) || !strings.EqualFold(initialURL.Host, req.URL.Host) {
					req.Header.Del("Authorization")
				}
			}
			if origCheckRedirect != nil {
				return origCheckRedirect(req, via)
			}
			if len(via) >= 10 {
				return errors.New("stopped after 10 redirects")
			}
			return nil
		}
		c.client = &cloned
	}

	if strings.TrimSpace(c.baseURL) != "" {
		if _, err := validateEndpointURL(c.baseURL); err != nil {
			return nil, err
		}
	}

	return c, nil
}

func checkRedirectPolicy(req *http.Request, via []*http.Request) error {
	if len(via) > 0 {
		initialURL := via[0].URL
		if !strings.EqualFold(initialURL.Scheme, req.URL.Scheme) || !strings.EqualFold(initialURL.Host, req.URL.Host) {
			req.Header.Del("Authorization")
		}
	}
	if len(via) >= 10 {
		return errors.New("stopped after 10 redirects")
	}
	return nil
}

func validateEndpointURL(rawURL string) (*url.URL, error) {
	trimmed := strings.TrimSpace(rawURL)
	if trimmed == "" {
		return nil, apperror.New(apperror.CodeInvalidInput, "endpoint URL is empty")
	}

	if strings.Contains(trimmed, "#") {
		return nil, apperror.New(apperror.CodeInvalidInput, fmt.Sprintf("invalid endpoint URL %q: URL must not contain fragments", rawURL))
	}

	u, err := url.Parse(trimmed)
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeInvalidInput, fmt.Sprintf("invalid endpoint URL %q", rawURL), err)
	}

	scheme := strings.ToLower(u.Scheme)
	if scheme != "http" && scheme != "https" {
		return nil, apperror.New(apperror.CodeInvalidInput, fmt.Sprintf("invalid endpoint URL %q: scheme must be http or https", rawURL))
	}

	if u.Hostname() == "" {
		return nil, apperror.New(apperror.CodeInvalidInput, fmt.Sprintf("invalid endpoint URL %q: missing hostname", rawURL))
	}

	if u.User != nil {
		return nil, apperror.New(apperror.CodeInvalidInput, fmt.Sprintf("invalid endpoint URL %q: embedded credentials are not allowed", rawURL))
	}

	if u.Fragment != "" {
		return nil, apperror.New(apperror.CodeInvalidInput, fmt.Sprintf("invalid endpoint URL %q: fragments are not allowed", rawURL))
	}

	return u, nil
}

func (c *userScopedClient) initHTTP() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.client != nil {
		return nil
	}

	tlsConfig := &tls.Config{
		InsecureSkipVerify: c.cfg.InsecureSkipVerify, //nolint:gosec
	}

	if strings.TrimSpace(c.cfg.TLSRootCA) != "" {
		pemBytes, err := os.ReadFile(c.cfg.TLSRootCA)
		if err != nil {
			return apperror.Wrap(apperror.CodeInternal, fmt.Sprintf("read TLS root CA %q", c.cfg.TLSRootCA), err)
		}

		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(pemBytes) {
			return apperror.New(apperror.CodeInternal, fmt.Sprintf("parse TLS root CA %q: invalid PEM", c.cfg.TLSRootCA))
		}
		tlsConfig.RootCAs = pool
	}

	c.transport = &http.Transport{
		TLSClientConfig:     tlsConfig,
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 20,
		IdleConnTimeout:     90 * time.Second,
	}

	c.client = &http.Client{
		Transport:     c.transport,
		CheckRedirect: checkRedirectPolicy,
	}

	return nil
}

// Close closes idle HTTP connections.
func (c *userScopedClient) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.transport != nil {
		c.transport.CloseIdleConnections()
	}
	return nil
}

func (c *userScopedClient) resolveBaseURL() (string, error) {
	// 1. If an explicit BaseURL is configured, use it.
	if strings.TrimSpace(c.baseURL) != "" {
		if _, err := validateEndpointURL(c.baseURL); err != nil {
			return "", err
		}
		return strings.TrimRight(strings.TrimSpace(c.baseURL), "/"), nil
	}

	// 2. Otherwise, resolve OWANALYTICS using the existing service discovery resolver.
	if c.resolver != nil {
		inst, err := c.resolver.Resolve(serviceName)
		if err != nil {
			return "", err
		}
		if inst != nil {
			// 3. If discovery returns a valid PublicEndPoint, use it.
			// 5. Never substitute PrivateEndPoint for user-scoped requests.
			publicEndpoint := strings.TrimSpace(inst.PublicEndPoint)
			if publicEndpoint != "" {
				if _, err := validateEndpointURL(publicEndpoint); err != nil {
					return "", err
				}
				return strings.TrimRight(publicEndpoint, "/"), nil
			}
			return "", apperror.New(apperror.CodeInvalidInput, "discovered owanalytics service does not expose a verified public endpoint for user-scoped requests")
		}
		return "", apperror.New(apperror.CodeInvalidInput, "discovered owanalytics service instance is nil")
	}

	// 4. If neither source provides a valid endpoint, return an appropriate error.
	return "", apperror.New(apperror.CodeInvalidInput, "no baseURL configured and no service resolver available for user-scoped analytics client")
}

func (c *userScopedClient) getJSON(
	ctx context.Context,
	path string,
	params map[string]string,
	authorization string,
	operation string,
) (any, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	baseURL, err := c.resolveBaseURL()
	if err != nil {
		return nil, err
	}

	reqURL, err := buildURL(baseURL, path, params)
	if err != nil {
		return nil, fmt.Errorf("failed to construct request URL: %w", err)
	}

	authHeader := strings.TrimSpace(authorization)
	if authHeader != "" && !strings.HasPrefix(strings.ToLower(authHeader), "bearer ") {
		authHeader = "Bearer " + authHeader
	}

	maxAttempts := c.maxRetries + 1
	var lastErr error

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		reqCtx, reqCancel := context.WithTimeout(ctx, c.timeout)

		req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, reqURL.String(), nil)
		if err != nil {
			reqCancel()
			return nil, err
		}

		if authHeader != "" {
			req.Header.Set("Authorization", authHeader)
		}
		req.Header.Set("Accept", "application/json")
		req.Header.Set("Content-Type", "application/json")

		resp, err := c.client.Do(req)
		if err != nil {
			reqCancel()
			lastErr = err

			if ctx.Err() != nil {
				return nil, ctx.Err()
			}

			if attempt >= maxAttempts {
				if c.logger != nil {
					c.logger.Error(fmt.Sprintf("%s request failed after %d attempt(s): %v", operation, attempt, err))
				}
				return nil, &APIClientError{
					Operation: operation,
					Err:       err,
				}
			}

			delay := c.calculateRetryDelay(attempt)
			if c.logger != nil {
				c.logger.Warn(fmt.Sprintf("%s retrying after request_error=%v attempt=%d/%d delay=%.2fs",
					operation, err, attempt, maxAttempts, delay.Seconds()))
			}

			if sleepErr := c.sleeper(ctx, delay); sleepErr != nil {
				return nil, sleepErr
			}
			continue
		}

		bodyBytes, readErr := io.ReadAll(io.LimitReader(resp.Body, 10*1024*1024))
		_ = resp.Body.Close()
		reqCancel()

		if readErr != nil {
			lastErr = readErr
			if attempt >= maxAttempts {
				return nil, &APIClientError{Operation: operation, Err: readErr}
			}
			delay := c.calculateRetryDelay(attempt)
			if sleepErr := c.sleeper(ctx, delay); sleepErr != nil {
				return nil, sleepErr
			}
			continue
		}

		if resp.StatusCode == http.StatusOK {
			dec := json.NewDecoder(bytes.NewReader(bodyBytes))
			dec.UseNumber()
			var decoded any
			if err := dec.Decode(&decoded); err != nil {
				code := resp.StatusCode
				return nil, &APIClientError{
					Operation:  operation,
					StatusCode: &code,
					Err:        err,
				}
			}
			return decoded, nil
		}

		statusCode := resp.StatusCode
		respBody := parseResponseBody(bodyBytes)

		if isRetryableStatus(statusCode) && attempt < maxAttempts {
			var delay time.Duration
			if statusCode == http.StatusTooManyRequests {
				retryAfterDelay := c.parseRetryAfter(resp.Header.Get("Retry-After"))
				if retryAfterDelay != nil {
					if retryAfterDelay.Seconds() > c.maxRetryDelay.Seconds() {
						if c.logger != nil {
							c.logger.Warn(fmt.Sprintf("%s not retrying status=429 because Retry-After=%.2fs exceeds maximum retry delay %.2fs",
								operation, retryAfterDelay.Seconds(), c.maxRetryDelay.Seconds()))
						}
						return nil, &APIClientError{
							Operation:    operation,
							StatusCode:   &statusCode,
							ResponseBody: respBody,
						}
					}
					delay = *retryAfterDelay
				} else {
					delay = c.calculateRetryDelay(attempt)
				}
			} else {
				delay = c.calculateRetryDelay(attempt)
			}

			if c.logger != nil {
				c.logger.Warn(fmt.Sprintf("%s retrying after status=%d attempt=%d/%d delay=%.2fs",
					operation, statusCode, attempt, maxAttempts, delay.Seconds()))
			}

			if sleepErr := c.sleeper(ctx, delay); sleepErr != nil {
				return nil, sleepErr
			}
			continue
		}

		return nil, &APIClientError{
			Operation:    operation,
			StatusCode:   &statusCode,
			ResponseBody: respBody,
		}
	}

	return nil, &APIClientError{
		Operation: operation,
		Err:       lastErr,
	}
}

func (c *userScopedClient) calculateRetryDelay(attempt int) time.Duration {
	base := 0.25 * float64(int(1)<<uint(attempt-1))
	if base > 2.0 {
		base = 2.0
	}

	jitter := 0.0
	if c.jitter != nil {
		jitter = c.jitter(base)
	}

	delaySec := base + jitter
	maxDelaySec := c.maxRetryDelay.Seconds()
	if delaySec > maxDelaySec {
		delaySec = maxDelaySec
	}
	if delaySec < 0 {
		delaySec = 0
	}

	return time.Duration(delaySec * float64(time.Second))
}

func (c *userScopedClient) parseRetryAfter(value string) *time.Duration {
	val := strings.TrimSpace(value)
	if val == "" {
		return nil
	}

	if sec, err := strconv.ParseFloat(val, 64); err == nil {
		if sec < 0 {
			return nil
		}
		d := time.Duration(sec * float64(time.Second))
		return &d
	}

	formats := []string{
		http.TimeFormat,
		time.RFC850,
		time.ANSIC,
	}

	for _, format := range formats {
		if t, err := time.Parse(format, val); err == nil {
			now := c.clock()
			diff := t.Sub(now)
			if diff < 0 {
				diff = 0
			}
			return &diff
		}
	}

	return nil
}

func isRetryableStatus(code int) bool {
	switch code {
	case 429, 500, 502, 503, 504:
		return true
	default:
		return false
	}
}

func parseResponseBody(body []byte) any {
	if len(body) == 0 {
		return nil
	}

	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber()
	var jsonVal any
	if err := dec.Decode(&jsonVal); err == nil {
		return jsonVal
	}

	return string(body)
}

func buildURL(baseURL, path string, params map[string]string) (*url.URL, error) {
	base := strings.TrimRight(baseURL, "/")
	full := base + path

	u, err := url.Parse(full)
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeInvalidInput, "failed to parse URL", err)
	}

	if len(params) > 0 {
		q := u.Query()
		for k, v := range params {
			q.Set(k, v)
		}
		u.RawQuery = q.Encode()
	}

	return u, nil
}

