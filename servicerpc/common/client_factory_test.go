package common

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/routerarchitects/ra-common-mods/apperror"
)

type mockResolver struct {
	instance *ServiceInstance
	err      error
}

func (m *mockResolver) Resolve(serviceName string) (*ServiceInstance, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.instance, nil
}

type mockResponse struct {
	status int
	body   []byte
}

func (m *mockResponse) StatusCode() int { return m.status }
func (m *mockResponse) Body() []byte    { return m.body }
func (m *mockResponse) Close()          {}

type mockRequester struct {
	resp          Response
	err           error
	lastMethod    string
	lastURL       string
	lastHeaders   map[string]string
	lastBody      []byte
	lastCtxIsNil  bool
	invocationCnt int
}

func (m *mockRequester) Send(ctx context.Context, method, url string, headers map[string]string, body []byte) (Response, error) {
	m.invocationCnt++
	m.lastCtxIsNil = ctx == nil
	m.lastMethod = method
	m.lastURL = url
	m.lastHeaders = headers
	m.lastBody = body
	if m.err != nil {
		return nil, m.err
	}
	return m.resp, nil
}

func TestNewServiceRPCBaseWithDeps_Validation(t *testing.T) {
	validResolver := &mockResolver{instance: &ServiceInstance{Key: "k", PrivateEndPoint: "http://svc"}}
	validRequester := &mockRequester{resp: &mockResponse{status: 200}}

	if _, err := NewServiceRPCBaseWithDeps(nil, validRequester, "svc", slog.Default()); err == nil {
		t.Fatalf("expected error for nil resolver")
	}
	if _, err := NewServiceRPCBaseWithDeps(validResolver, nil, "svc", slog.Default()); err == nil {
		t.Fatalf("expected error for nil requester")
	}
	if _, err := NewServiceRPCBaseWithDeps(validResolver, validRequester, "   ", slog.Default()); err == nil {
		t.Fatalf("expected error for empty internalName")
	}
}

func TestSend_ValidatesInput(t *testing.T) {
	base, err := NewServiceRPCBaseWithDeps(
		&mockResolver{instance: &ServiceInstance{Key: "k", PrivateEndPoint: "http://svc"}},
		&mockRequester{resp: &mockResponse{status: 200}},
		"caller",
		slog.Default(),
	)
	if err != nil {
		t.Fatalf("unexpected constructor error: %v", err)
	}

	cases := []struct {
		name       string
		ctx        context.Context
		method     string
		endpoint   string
		service    string
		wantErrMsg string
	}{
		{name: "nil ctx", ctx: nil, method: "GET", endpoint: "/x", service: "svc", wantErrMsg: "context is required"},
		{name: "empty method", ctx: context.Background(), method: "", endpoint: "/x", service: "svc", wantErrMsg: "http method is required"},
		{name: "empty endpoint", ctx: context.Background(), method: "GET", endpoint: "", service: "svc", wantErrMsg: "endpoint is required"},
		{name: "empty service", ctx: context.Background(), method: "GET", endpoint: "/x", service: "", wantErrMsg: "serviceName is required"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, gotErr := base.Send(tc.ctx, tc.method, tc.endpoint, nil, tc.service)
			if gotErr == nil {
				t.Fatalf("expected error")
			}
			if !strings.Contains(gotErr.Error(), tc.wantErrMsg) {
				t.Fatalf("expected error containing %q, got %v", tc.wantErrMsg, gotErr)
			}
		})
	}
}

func TestSend_UsesResolverAndRequester(t *testing.T) {
	reqMock := &mockRequester{resp: &mockResponse{status: 200, body: []byte(`{}`)}}
	base, err := NewServiceRPCBaseWithDeps(
		&mockResolver{instance: &ServiceInstance{Key: "api-key", PrivateEndPoint: "http://service.local/"}},
		reqMock,
		"internal-caller",
		slog.Default(),
	)
	if err != nil {
		t.Fatalf("unexpected constructor error: %v", err)
	}

	resp, sendErr := base.Send(context.Background(), "POST", "/v1/test", io.NopCloser(strings.NewReader(`{"a":1}`)), "analytics")
	if sendErr != nil {
		t.Fatalf("unexpected send error: %v", sendErr)
	}
	if resp == nil {
		t.Fatalf("expected response")
	}

	if reqMock.lastMethod != "POST" {
		t.Fatalf("expected POST, got %s", reqMock.lastMethod)
	}
	if reqMock.lastURL != "http://service.local/v1/test" {
		t.Fatalf("unexpected url: %s", reqMock.lastURL)
	}
	if reqMock.lastHeaders["X-API-KEY"] != "api-key" {
		t.Fatalf("missing X-API-KEY")
	}
	if reqMock.lastHeaders["X-INTERNAL-NAME"] != "internal-caller" {
		t.Fatalf("missing X-INTERNAL-NAME")
	}
	if reqMock.lastHeaders["Content-Type"] == "" {
		t.Fatalf("expected Content-Type for non-empty body")
	}
	if reqMock.lastCtxIsNil {
		t.Fatalf("expected non-nil ctx passed to requester")
	}
	if string(reqMock.lastBody) != `{"a":1}` {
		t.Fatalf("unexpected body: %s", string(reqMock.lastBody))
	}
}

func TestSend_WrapsRequesterError(t *testing.T) {
	base, err := NewServiceRPCBaseWithDeps(
		&mockResolver{instance: &ServiceInstance{Key: "k", PrivateEndPoint: "http://svc"}},
		&mockRequester{err: errors.New("dial failure")},
		"caller",
		slog.Default(),
	)
	if err != nil {
		t.Fatalf("unexpected constructor error: %v", err)
	}

	_, sendErr := base.Send(context.Background(), "GET", "/x", nil, "owanalytics")
	if sendErr == nil {
		t.Fatalf("expected error")
	}
	if apperror.CodeOf(sendErr) != apperror.CodeInternal {
		t.Fatalf("expected internal code, got %s", apperror.CodeOf(sendErr))
	}
}

func TestSend_PreservesCancellationAndDeadlineErrors(t *testing.T) {
	testCases := []struct {
		name       string
		reqErr     error
		expectIs   error
		expectCode apperror.Code
	}{
		{
			name:       "context canceled",
			reqErr:     context.Canceled,
			expectIs:   context.Canceled,
			expectCode: apperror.CodeTimeout,
		},
		{
			name:       "deadline exceeded",
			reqErr:     context.DeadlineExceeded,
			expectIs:   context.DeadlineExceeded,
			expectCode: apperror.CodeTimeout,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			base, err := NewServiceRPCBaseWithDeps(
				&mockResolver{instance: &ServiceInstance{Key: "k", PrivateEndPoint: "http://svc"}},
				&mockRequester{err: tc.reqErr},
				"caller",
				slog.Default(),
			)
			if err != nil {
				t.Fatalf("unexpected constructor error: %v", err)
			}

			_, sendErr := base.Send(context.Background(), "GET", "/x", nil, "owanalytics")
			if sendErr == nil {
				t.Fatalf("expected error")
			}
			if !errors.Is(sendErr, tc.expectIs) {
				t.Fatalf("expected errors.Is(..., %v) to be true, got %v", tc.expectIs, sendErr)
			}
			if got := apperror.CodeOf(sendErr); got != tc.expectCode {
				t.Fatalf("expected code %s, got %s", tc.expectCode, got)
			}
		})
	}
}

func TestSend_PreservesAppErrorFromRequester(t *testing.T) {
	upstreamErr := apperror.New(apperror.CodeForbidden, "upstream denied")
	base, err := NewServiceRPCBaseWithDeps(
		&mockResolver{instance: &ServiceInstance{Key: "k", PrivateEndPoint: "http://svc"}},
		&mockRequester{err: upstreamErr},
		"caller",
		slog.Default(),
	)
	if err != nil {
		t.Fatalf("unexpected constructor error: %v", err)
	}

	_, sendErr := base.Send(context.Background(), "GET", "/x", nil, "owanalytics")
	if sendErr == nil {
		t.Fatalf("expected error")
	}
	if !errors.Is(sendErr, upstreamErr) {
		t.Fatalf("expected upstream error to be preserved")
	}
	if got := apperror.CodeOf(sendErr); got != apperror.CodeForbidden {
		t.Fatalf("expected forbidden code, got %s", got)
	}
}

func TestSend_PreservesUnknownAppErrorFromRequester(t *testing.T) {
	upstreamErr := apperror.New(apperror.CodeUnknown, "upstream unknown")
	base, err := NewServiceRPCBaseWithDeps(
		&mockResolver{instance: &ServiceInstance{Key: "k", PrivateEndPoint: "http://svc"}},
		&mockRequester{err: upstreamErr},
		"caller",
		slog.Default(),
	)
	if err != nil {
		t.Fatalf("unexpected constructor error: %v", err)
	}

	_, sendErr := base.Send(context.Background(), "GET", "/x", nil, "owanalytics")
	if sendErr == nil {
		t.Fatalf("expected error")
	}
	if !errors.Is(sendErr, upstreamErr) {
		t.Fatalf("expected upstream app error to be preserved")
	}
	if got := apperror.CodeOf(sendErr); got != apperror.CodeUnknown {
		t.Fatalf("expected unknown code, got %s", got)
	}
}

func TestNewDiscoveryResolver_ResolveValidation(t *testing.T) {
	resolver := NewDiscoveryResolver(nil)
	_, err := resolver.Resolve("svc")
	if err == nil {
		t.Fatalf("expected error for nil discovery")
	}
	if got := apperror.CodeOf(err); got != apperror.CodeInternal {
		t.Fatalf("expected internal code, got %s", got)
	}
}

func TestNewFiberRequester_Validation(t *testing.T) {
	if _, err := NewFiberRequester("/path/that/does/not/exist.pem"); err == nil {
		t.Fatalf("expected error for missing pem file")
	}

	tempDir := t.TempDir()
	invalidPemPath := filepath.Join(tempDir, "invalid.pem")
	if err := os.WriteFile(invalidPemPath, []byte("not-a-pem"), 0o600); err != nil {
		t.Fatalf("failed to write invalid pem: %v", err)
	}
	if _, err := NewFiberRequester(invalidPemPath); err == nil {
		t.Fatalf("expected error for invalid pem file")
	}
}

func TestFiberRequester_Send(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if got := r.Header.Get("X-Test-Header"); got != "ok" {
			t.Errorf("expected X-Test-Header=ok, got %q", got)
		}
		if r.URL.Path != "/endpoint" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("failed to read request body: %v", err)
		}
		if string(body) != `{"hello":"world"}` {
			t.Errorf("unexpected request body: %s", string(body))
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	requester, err := NewFiberRequester("")
	if err != nil {
		t.Fatalf("unexpected requester constructor error: %v", err)
	}

	resp, sendErr := requester.Send(
		context.Background(),
		http.MethodPost,
		srv.URL+"/endpoint",
		map[string]string{"X-Test-Header": "ok"},
		[]byte(`{"hello":"world"}`),
	)
	if sendErr != nil {
		t.Fatalf("unexpected send error: %v", sendErr)
	}
	defer resp.Close()

	if resp.StatusCode() != http.StatusCreated {
		t.Fatalf("expected status 201, got %d", resp.StatusCode())
	}
	if string(resp.Body()) != `{"ok":true}` {
		t.Fatalf("unexpected response body: %s", string(resp.Body()))
	}
}

func TestNewServiceRPCBase(t *testing.T) {
	if _, err := NewServiceRPCBase(nil, "/path/that/does/not/exist.pem", "caller", slog.Default()); err == nil {
		t.Fatalf("expected tls path error")
	}

	base, err := NewServiceRPCBase(nil, "", "caller", nil)
	if err != nil {
		t.Fatalf("unexpected constructor error: %v", err)
	}
	if base == nil {
		t.Fatalf("expected base instance")
	}
}

func TestLogger_DefaultFallback(t *testing.T) {
	var nilBase *ServiceRPCBase
	if nilBase.Logger() == nil {
		t.Fatalf("expected default logger for nil receiver")
	}

	emptyBase := &ServiceRPCBase{}
	if emptyBase.Logger() == nil {
		t.Fatalf("expected default logger for empty base")
	}
}
