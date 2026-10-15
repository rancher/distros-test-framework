package releasebot

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGitHubTagExists(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.EscapedPath() {
		case "/repos/rancher/rke2/git/ref/tags/v1.37.1-rc1+rke2r1":
			w.WriteHeader(http.StatusOK)
		case "/repos/rancher/rke2/git/ref/tags/v9.9.9-rc1+rke2r1":
			w.WriteHeader(http.StatusNotFound)
		default:
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer srv.Close()
	gh := &gitHub{BaseURL: srv.URL, HTTP: srv.Client()}

	if ok, err := gh.TagExists(context.Background(), "rke2", "v1.37.1-rc1+rke2r1"); !ok || err != nil {
		t.Fatalf("existing tag: %v %v", ok, err)
	}
	if ok, err := gh.TagExists(context.Background(), "rke2", "v9.9.9-rc1+rke2r1"); ok || err != nil {
		t.Fatalf("missing tag: %v %v", ok, err)
	}
	if _, err := gh.TagExists(context.Background(), "k3s", "v1.37.1-rc1+k3s1"); err == nil {
		t.Fatal("500 must be an error, not a missing tag")
	}
	if _, err := gh.TagExists(context.Background(), "nope", "v1"); err == nil {
		t.Fatal("unknown product must be an error")
	}
}

func TestGitHubDispatch(t *testing.T) {
	var got struct {
		Ref    string            `json:"ref"`
		Inputs map[string]string `json:"inputs"`
	}
	status := http.StatusNoContent
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/repos/o/r/actions/workflows/wf.yaml/dispatches" ||
			r.Header.Get("Authorization") != "Bearer tok" {
			t.Errorf("unexpected request %s %s auth=%q", r.Method, r.URL.Path, r.Header.Get("Authorization"))
		}
		_ = json.NewDecoder(r.Body).Decode(&got)
		w.WriteHeader(status)
		_, _ = w.Write([]byte(`{"message":"Unexpected inputs provided: [\"request_id\"]"}`))
	}))
	defer srv.Close()

	w := workflowDispatch{
		Repo: "o/r", Workflow: "wf.yaml", Ref: "main",
		Inputs: map[string]string{"rcs": "v1.37.1-rc1", "request_id": "rb-1", "empty": ""},
	}
	gh := &gitHub{BaseURL: srv.URL, Token: "tok", HTTP: srv.Client()}

	if err := gh.Dispatch(context.Background(), w); err != nil {
		t.Fatal(err)
	}
	if got.Ref != "main" || got.Inputs["rcs"] != "v1.37.1-rc1" || got.Inputs["request_id"] != "rb-1" {
		t.Fatalf("body = %+v", got)
	}
	if _, sent := got.Inputs["empty"]; sent {
		t.Fatal("empty inputs must be omitted")
	}

	// A workflow without the input answers 422: the error must carry gitHub's message.
	status = http.StatusUnprocessableEntity
	if err := gh.Dispatch(context.Background(), w); err == nil || !strings.Contains(err.Error(), "Unexpected inputs") {
		t.Fatalf("422: %v", err)
	}

	if err := (&gitHub{BaseURL: srv.URL, HTTP: srv.Client()}).Dispatch(context.Background(), w); err == nil {
		t.Fatal("dispatch without a token must fail")
	}
}
