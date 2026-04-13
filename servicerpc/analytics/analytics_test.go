package analytics

import (
	"context"
	"errors"
	"log/slog"
	"testing"

	"github.com/routerarchitects/ow-common-mods/servicerpc/common"
	"github.com/routerarchitects/ra-common-mods/apperror"
)

type mockResolver struct {
	instance *common.ServiceInstance
	err      error
}

func (m *mockResolver) Resolve(serviceName string) (*common.ServiceInstance, error) {
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
	resp common.Response
	err  error
}

func (m *mockRequester) Send(ctx context.Context, method, url string, headers map[string]string, body []byte) (common.Response, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.resp, nil
}

func newClient(t *testing.T, requester common.Requester) *AnalyticsClient {
	t.Helper()
	base, err := common.NewServiceRPCBaseWithDeps(
		&mockResolver{instance: &common.ServiceInstance{Key: "k", PrivateEndPoint: "http://owanalytics.local"}},
		requester,
		"caller",
		slog.Default(),
	)
	if err != nil {
		t.Fatalf("unexpected deps error: %v", err)
	}
	return NewAnalyticsClient(base)
}

func TestGetTimepoints_Success(t *testing.T) {
	client := newClient(t, &mockRequester{
		resp: &mockResponse{
			status: 200,
			body: []byte(`{
				"points":[
					[
						{"id":"1","boardId":"b1","timestamp":100}
					],
					[
						{"id":"2","boardId":"b1","timestamp":200}
					]
				]
			}`),
		},
	})

	out, err := client.GetTimepoints(context.Background(), TimepointRequest{
		BoardID:    "b1",
		FromDate:   1,
		EndDate:    2,
		MaxRecords: 10,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out) != 2 {
		t.Fatalf("expected 2 timepoints, got %d", len(out))
	}
}

func TestGetTimepoints_NotFound(t *testing.T) {
	client := newClient(t, &mockRequester{
		resp: &mockResponse{status: 404, body: []byte(`{}`)},
	})

	_, err := client.GetTimepoints(context.Background(), TimepointRequest{BoardID: "b1"})
	if err == nil {
		t.Fatalf("expected error")
	}
	if apperror.CodeOf(err) != apperror.CodeNotFound {
		t.Fatalf("expected not found, got %s", apperror.CodeOf(err))
	}
}

func TestGetTimepoints_InvalidJSON(t *testing.T) {
	client := newClient(t, &mockRequester{
		resp: &mockResponse{status: 200, body: []byte(`{not-json`)},
	})

	_, err := client.GetTimepoints(context.Background(), TimepointRequest{BoardID: "b1"})
	if err == nil {
		t.Fatalf("expected error")
	}
	if apperror.CodeOf(err) != apperror.CodeInternal {
		t.Fatalf("expected internal error, got %s", apperror.CodeOf(err))
	}
}

func TestGetDeviceInfo_RequestError(t *testing.T) {
	client := newClient(t, &mockRequester{err: errors.New("network down")})

	_, err := client.GetDeviceInfo(context.Background(), "b1")
	if err == nil {
		t.Fatalf("expected error")
	}
	if apperror.CodeOf(err) != apperror.CodeInternal {
		t.Fatalf("expected internal error, got %s", apperror.CodeOf(err))
	}
}
