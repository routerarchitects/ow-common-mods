package analytics

import (
	"context"
	"errors"
	"log/slog"
	"net/url"
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
	resp    common.Response
	err     error
	lastURL string
}

func (m *mockRequester) Send(ctx context.Context, method, url string, headers map[string]string, body []byte) (common.Response, error) {
	m.lastURL = url
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

	_, err := client.GetTimepoints(context.Background(), TimepointRequest{
		BoardID:    "b1",
		FromDate:   1,
		EndDate:    2,
		MaxRecords: 10,
	})
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

	_, err := client.GetTimepoints(context.Background(), TimepointRequest{
		BoardID:    "b1",
		FromDate:   1,
		EndDate:    2,
		MaxRecords: 10,
	})
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

func TestGetDeviceInfo_PathEscaping(t *testing.T) {
	reqMock := &mockRequester{
		resp: &mockResponse{status: 200, body: []byte(`{"devices":[]}`)},
	}
	client := newClient(t, reqMock)

	_, err := client.GetDeviceInfo(context.Background(), " board /id?x=1 ")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	parsed, err := url.Parse(reqMock.lastURL)
	if err != nil {
		t.Fatalf("failed to parse requested URL: %v", err)
	}

	expectedPath := "/api/v1/board/" + url.PathEscape("board /id?x=1") + "/devices"
	if parsed.EscapedPath() != expectedPath {
		t.Fatalf("expected escaped path %q, got %q", expectedPath, parsed.EscapedPath())
	}
}

func TestGetDeviceInfo_Validation(t *testing.T) {
	client := newClient(t, &mockRequester{
		resp: &mockResponse{status: 200, body: []byte(`{"devices":[]}`)},
	})

	_, err := client.GetDeviceInfo(context.Background(), "   ")
	if err == nil {
		t.Fatalf("expected error")
	}
	if got := apperror.CodeOf(err); got != apperror.CodeInvalidInput {
		t.Fatalf("expected invalid input, got %s", got)
	}
}

func TestGetWifiClientHistoryMACs_SuccessAndEscaping(t *testing.T) {
	reqMock := &mockRequester{
		resp: &mockResponse{status: 200, body: []byte(`{"entries":["aa:bb","cc:dd"]}`)},
	}
	client := newClient(t, reqMock)

	entries, err := client.GetWifiClientHistoryMACs(context.Background(), " board /id?x=1 ", 50, 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}

	parsed, err := url.Parse(reqMock.lastURL)
	if err != nil {
		t.Fatalf("failed to parse requested URL: %v", err)
	}
	if parsed.Path != "/api/v1/wifiClientHistory" {
		t.Fatalf("unexpected path: %s", parsed.Path)
	}
	query := parsed.Query()
	if got := query.Get("macsOnly"); got != "true" {
		t.Fatalf("macsOnly mismatch: %q", got)
	}
	if got := query.Get("boardId"); got != "board /id?x=1" {
		t.Fatalf("boardId mismatch: %q", got)
	}
	if got := query.Get("limit"); got != "50" {
		t.Fatalf("limit mismatch: %q", got)
	}
	if got := query.Get("offset"); got != "10" {
		t.Fatalf("offset mismatch: %q", got)
	}
}

func TestGetWifiClientHistoryMACs_NotFound(t *testing.T) {
	client := newClient(t, &mockRequester{
		resp: &mockResponse{status: 404, body: []byte(`{}`)},
	})

	_, err := client.GetWifiClientHistoryMACs(context.Background(), "b1", 10, 0)
	if err == nil {
		t.Fatalf("expected error")
	}
	if got := apperror.CodeOf(err); got != apperror.CodeNotFound {
		t.Fatalf("expected not found, got %s", got)
	}
}

func TestGetWifiClientHistoryMACs_NonOK(t *testing.T) {
	client := newClient(t, &mockRequester{
		resp: &mockResponse{status: 500, body: []byte(`{}`)},
	})

	_, err := client.GetWifiClientHistoryMACs(context.Background(), "b1", 10, 0)
	if err == nil {
		t.Fatalf("expected error")
	}
	if got := apperror.CodeOf(err); got != apperror.CodeInternal {
		t.Fatalf("expected internal, got %s", got)
	}
}

func TestGetWifiClientHistoryMACs_InvalidJSON(t *testing.T) {
	client := newClient(t, &mockRequester{
		resp: &mockResponse{status: 200, body: []byte(`{not-json`)},
	})

	_, err := client.GetWifiClientHistoryMACs(context.Background(), "b1", 10, 0)
	if err == nil {
		t.Fatalf("expected error")
	}
	if got := apperror.CodeOf(err); got != apperror.CodeInternal {
		t.Fatalf("expected internal, got %s", got)
	}
}

func TestGetWifiClientHistoryMACs_Validation(t *testing.T) {
	client := newClient(t, &mockRequester{
		resp: &mockResponse{status: 200, body: []byte(`{"entries":[]}`)},
	})

	testCases := []struct {
		name    string
		boardID string
		limit   int
		offset  int
	}{
		{name: "missing board id", boardID: "   ", limit: 10, offset: 0},
		{name: "non positive limit", boardID: "b1", limit: 0, offset: 0},
		{name: "negative limit", boardID: "b1", limit: -1, offset: 0},
		{name: "negative offset", boardID: "b1", limit: 10, offset: -1},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := client.GetWifiClientHistoryMACs(context.Background(), tc.boardID, tc.limit, tc.offset)
			if err == nil {
				t.Fatalf("expected error")
			}
			if got := apperror.CodeOf(err); got != apperror.CodeInvalidInput {
				t.Fatalf("expected invalid input, got %s", got)
			}
		})
	}
}

func TestGetWifiClientHistoryMACs_PreservesTimeoutError(t *testing.T) {
	client := newClient(t, &mockRequester{err: context.DeadlineExceeded})

	_, err := client.GetWifiClientHistoryMACs(context.Background(), "b1", 10, 0)
	if err == nil {
		t.Fatalf("expected error")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected deadline exceeded, got %v", err)
	}
	if got := apperror.CodeOf(err); got != apperror.CodeTimeout {
		t.Fatalf("expected timeout, got %s", got)
	}
}

func TestGetTimepoints_PathAndQueryEscaping(t *testing.T) {
	statsOnly := true
	pointsOnly := false
	pointStatsOnly := false
	latestPerDevice := false

	reqMock := &mockRequester{
		resp: &mockResponse{status: 200, body: []byte(`{"points":[]}`)},
	}
	client := newClient(t, reqMock)

	_, err := client.GetTimepoints(context.Background(), TimepointRequest{
		BoardID:         " board /id?x=1 ",
		FromDate:        11,
		EndDate:         22,
		MaxRecords:      33,
		StatsOnly:       &statsOnly,
		PointsOnly:      &pointsOnly,
		PointStatsOnly:  &pointStatsOnly,
		LatestPerDevice: &latestPerDevice,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	parsed, err := url.Parse(reqMock.lastURL)
	if err != nil {
		t.Fatalf("failed to parse requested URL: %v", err)
	}

	expectedPath := "/api/v1/board/" + url.PathEscape("board /id?x=1") + "/timepoints"
	if parsed.EscapedPath() != expectedPath {
		t.Fatalf("expected escaped path %q, got %q", expectedPath, parsed.EscapedPath())
	}

	query := parsed.Query()
	if got := query.Get("fromDate"); got != "11" {
		t.Fatalf("fromDate mismatch: %q", got)
	}
	if got := query.Get("endDate"); got != "22" {
		t.Fatalf("endDate mismatch: %q", got)
	}
	if got := query.Get("maxRecords"); got != "33" {
		t.Fatalf("maxRecords mismatch: %q", got)
	}
	if got := query.Get("statsOnly"); got != "true" {
		t.Fatalf("statsOnly mismatch: %q", got)
	}
	if got := query.Get("pointsOnly"); got != "false" {
		t.Fatalf("pointsOnly mismatch: %q", got)
	}
	if got := query.Get("pointStatsOnly"); got != "false" {
		t.Fatalf("pointStatsOnly mismatch: %q", got)
	}
	if got := query.Get("LatestPerDevice"); got != "false" {
		t.Fatalf("LatestPerDevice mismatch: %q", got)
	}
}

func TestGetTimepoints_Validation(t *testing.T) {
	client := newClient(t, &mockRequester{
		resp: &mockResponse{status: 200, body: []byte(`{"points":[]}`)},
	})

	testCases := []struct {
		name string
		req  TimepointRequest
	}{
		{
			name: "missing board id",
			req: TimepointRequest{
				FromDate:   1,
				EndDate:    2,
				MaxRecords: 10,
			},
		},
		{
			name: "missing from date",
			req: TimepointRequest{
				BoardID:    "b1",
				EndDate:    2,
				MaxRecords: 10,
			},
		},
		{
			name: "end before from",
			req: TimepointRequest{
				BoardID:    "b1",
				FromDate:   3,
				EndDate:    2,
				MaxRecords: 10,
			},
		},
		{
			name: "max records invalid",
			req: TimepointRequest{
				BoardID:    "b1",
				FromDate:   1,
				EndDate:    2,
				MaxRecords: 0,
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := client.GetTimepoints(context.Background(), tc.req)
			if err == nil {
				t.Fatalf("expected error")
			}
			if got := apperror.CodeOf(err); got != apperror.CodeInvalidInput {
				t.Fatalf("expected invalid input, got %s", got)
			}
		})
	}
}
