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

func TestValidateToken_FallbackSuccess(t *testing.T) {
	client := newSecurityClient(t, &scriptedRequester{
		calls: []reqCall{
			{matchURL: "/validateSubToken", err: errors.New("primary failed")},
			{matchURL: "/validateToken", resp: &mockResponse{status: 200}},
		},
	})

	if err := client.ValidateToken(context.Background(), "abc"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateToken_FallbackTransportFailure(t *testing.T) {
	client := newSecurityClient(t, &scriptedRequester{
		calls: []reqCall{
			{matchURL: "/validateSubToken", err: errors.New("primary failed")},
			{matchURL: "/validateToken", err: errors.New("fallback failed")},
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

func TestValidateToken_FallbackNon200(t *testing.T) {
	client := newSecurityClient(t, &scriptedRequester{
		calls: []reqCall{
			{matchURL: "/validateSubToken", err: errors.New("primary failed")},
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
