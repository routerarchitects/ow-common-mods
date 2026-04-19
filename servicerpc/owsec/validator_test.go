package owsec

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"github.com/routerarchitects/ow-common-mods/servicerpc/common"
	"github.com/routerarchitects/ra-common-mods/apperror"
)

type mockResolver struct {
	instance *common.ServiceInstance
}

func (m *mockResolver) Resolve(serviceName string) (*common.ServiceInstance, error) {
	return m.instance, nil
}

type mockResponse struct {
	status int
	body   []byte
}

func (m *mockResponse) StatusCode() int { return m.status }
func (m *mockResponse) Body() []byte    { return m.body }
func (m *mockResponse) Close()          {}

type scriptedRequester struct {
	callIdx int
	calls   []reqCall
}

type reqCall struct {
	matchURL string
	resp     common.Response
	err      error
}

func (s *scriptedRequester) Send(ctx context.Context, method, url string, headers map[string]string, body []byte) (common.Response, error) {
	if s.callIdx >= len(s.calls) {
		return nil, errors.New("unexpected call")
	}
	cur := s.calls[s.callIdx]
	s.callIdx++
	if cur.matchURL != "" && !strings.Contains(url, cur.matchURL) {
		return nil, errors.New("unexpected url: " + url)
	}
	return cur.resp, cur.err
}

func newSecurityClient(t *testing.T, requester common.Requester) *SecurityClient {
	t.Helper()
	base, err := common.NewServiceRPCBaseWithDeps(
		&mockResolver{instance: &common.ServiceInstance{Key: "k", PrivateEndPoint: "http://owsec.local"}},
		requester,
		"caller",
		slog.Default(),
	)
	if err != nil {
		t.Fatalf("unexpected deps error: %v", err)
	}
	return NewSecurityClient(base)
}

func TestValidateToken_PrimaryEndpointSuccess(t *testing.T) {
	client := newSecurityClient(t, &scriptedRequester{
		calls: []reqCall{
			{matchURL: "/validateSubToken", resp: &mockResponse{status: 200}},
		},
	})

	if err := client.ValidateToken(context.Background(), "abc"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateToken_SecondEndpointSuccess(t *testing.T) {
	client := newSecurityClient(t, &scriptedRequester{
		calls: []reqCall{
			{matchURL: "/validateSubToken", resp: &mockResponse{status: 404}},
			{matchURL: "/validateToken", resp: &mockResponse{status: 200}},
		},
	})

	if err := client.ValidateToken(context.Background(), "abc"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateToken_SecondEndpointTransportFailure(t *testing.T) {
	client := newSecurityClient(t, &scriptedRequester{
		calls: []reqCall{
			{matchURL: "/validateSubToken", resp: &mockResponse{status: 404}},
			{matchURL: "/validateToken", err: errors.New("fallback failed")},
		},
	})

	err := client.ValidateToken(context.Background(), "abc")
	if err == nil {
		t.Fatalf("expected error")
	}
	if apperror.CodeOf(err) != apperror.CodeInternal {
		t.Fatalf("expected internal, got %s", apperror.CodeOf(err))
	}
}

func TestValidateToken_BothEndpointsRejectToken(t *testing.T) {
	client := newSecurityClient(t, &scriptedRequester{
		calls: []reqCall{
			{matchURL: "/validateSubToken", resp: &mockResponse{status: 404}},
			{matchURL: "/validateToken", resp: &mockResponse{status: 401}},
		},
	})

	err := client.ValidateToken(context.Background(), "abc")
	if err == nil {
		t.Fatalf("expected error")
	}
	if apperror.CodeOf(err) != apperror.CodeUnauthorized {
		t.Fatalf("expected unauthorized, got %s", apperror.CodeOf(err))
	}
}

func TestValidateToken_MixedEndpointFailureReturnsInternal(t *testing.T) {
	client := newSecurityClient(t, &scriptedRequester{
		calls: []reqCall{
			{matchURL: "/validateSubToken", resp: &mockResponse{status: 500}},
			{matchURL: "/validateToken", resp: &mockResponse{status: 401}},
		},
	})

	err := client.ValidateToken(context.Background(), "abc")
	if err == nil {
		t.Fatalf("expected error")
	}
	if got := apperror.CodeOf(err); got != apperror.CodeInternal {
		t.Fatalf("expected internal, got %s", got)
	}
}

func TestValidateToken_ContextCanceledReturnsTimeout(t *testing.T) {
	req := &scriptedRequester{
		calls: []reqCall{
			{matchURL: "/validateSubToken", err: context.Canceled},
			{matchURL: "/validateToken", resp: &mockResponse{status: 200}},
		},
	}
	client := newSecurityClient(t, req)

	err := client.ValidateToken(context.Background(), "abc")
	if err == nil {
		t.Fatalf("expected error")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context canceled error, got %v", err)
	}
	if got := apperror.CodeOf(err); got != apperror.CodeTimeout {
		t.Fatalf("expected timeout code, got %s", got)
	}
	if req.callIdx != 1 {
		t.Fatalf("expected second endpoint not to run, call count=%d", req.callIdx)
	}
}

func TestValidateToken_DeadlineExceededReturnsTimeout(t *testing.T) {
	req := &scriptedRequester{
		calls: []reqCall{
			{matchURL: "/validateSubToken", err: context.DeadlineExceeded},
			{matchURL: "/validateToken", resp: &mockResponse{status: 200}},
		},
	}
	client := newSecurityClient(t, req)

	err := client.ValidateToken(context.Background(), "abc")
	if err == nil {
		t.Fatalf("expected error")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected deadline exceeded error, got %v", err)
	}
	if got := apperror.CodeOf(err); got != apperror.CodeTimeout {
		t.Fatalf("expected timeout code, got %s", got)
	}
	if req.callIdx != 1 {
		t.Fatalf("expected second endpoint not to run, call count=%d", req.callIdx)
	}
}
