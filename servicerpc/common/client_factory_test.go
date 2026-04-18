package common

import (
	"context"
	"errors"
	"io"
	"log/slog"
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
			expectCode: apperror.CodeUnknown,
		},
		{
			name:       "deadline exceeded",
			reqErr:     context.DeadlineExceeded,
			expectIs:   context.DeadlineExceeded,
			expectCode: apperror.CodeUnknown,
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
