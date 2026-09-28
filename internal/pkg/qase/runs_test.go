package qase

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	qaseclient "github.com/qase-tms/qase-go/qase-api-client"
)

// testClient points the shared client at a fake Qase API with the same auth wiring as AddQase.
func testClient(t *testing.T, h http.HandlerFunc) *Client {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)

	cfg := qaseclient.NewConfiguration()
	cfg.Servers = qaseclient.ServerConfigurations{{URL: srv.URL + "/v1"}}
	ctx := context.WithValue(context.Background(), qaseclient.ContextAPIKeys,
		map[string]qaseclient.APIKey{"TokenAuth": {Key: "tok"}})

	return &Client{QaseAPI: qaseclient.NewAPIClient(cfg), Ctx: ctx}
}

func TestSearchRuns(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/run/K3SRKE2" || r.Header.Get("Token") != "tok" ||
			r.URL.Query().Get("search") != "RKE2 September 2026 Patch Validation for v1.35.9+rke2r1" ||
			r.URL.Query().Get("limit") != "100" {
			t.Errorf("unexpected request %s %v token=%q", r.URL.Path, r.URL.Query(), r.Header.Get("Token"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":true,"result":{"entities":[
			{"id":1016,"title":"RKE2 September 2026 Patch Validation for v1.35.9+rke2r1",
			 "description":"Version: v1.35.9-rc1"},
			{"id":1017,"title":"other","description":null}]}}`))
	})

	runs, err := c.SearchRuns(context.Background(), "RKE2 September 2026 Patch Validation for v1.35.9+rke2r1")
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 2 || runs[0].GetId() != 1016 || runs[0].GetDescription() != "Version: v1.35.9-rc1" ||
		runs[1].GetDescription() != "" {
		t.Fatalf("runs = %+v", runs)
	}
}

func TestSearchRunsErrors(t *testing.T) {
	for name, h := range map[string]http.HandlerFunc{
		"http 500": func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusInternalServerError) },
		"status false": func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"status":false}`))
		},
	} {
		if _, err := testClient(t, h).SearchRuns(context.Background(), "x"); err == nil {
			t.Fatalf("%s: expected error", name)
		}
	}
}

func TestSearchRunsHonoursContext(t *testing.T) {
	c := testClient(t, func(http.ResponseWriter, *http.Request) {})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := c.SearchRuns(ctx, "x"); err == nil {
		t.Fatal("expected error for canceled context")
	}
}
