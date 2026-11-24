package releasebot

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

// The real client must tell a confirmed rejection from a trigger that may have been accepted.
func TestJenkinsTriggerErrorClassification(t *testing.T) {
	job := &JenkinsJob{Path: "f/j"}
	cases := map[string]*struct {
		handler func(w http.ResponseWriter, r *http.Request)
		unknown bool
	}{
		"accepted then connection dropped": {func(w http.ResponseWriter, _ *http.Request) {
			hj, _ := w.(http.Hijacker)
			conn, _, _ := hj.Hijack()
			_ = conn.Close()
		}, true},
		"502 from proxy": {func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusBadGateway) }, true},
		"201 without Location": {func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusCreated)
		}, true},
		"400 rejected": {func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusBadRequest) }, false},
	}
	for name, c := range cases {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if strings.Contains(r.URL.Path, "crumbIssuer") {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			c.handler(w, r)
		}))
		_, err := NewJenkins(srv.URL, "u", "t").Trigger(context.Background(), job)
		srv.Close()
		if err == nil || errors.Is(err, errTriggerUnknown) != c.unknown {
			t.Fatalf("%s: err=%v, want unknown=%v", name, err, c.unknown)
		}
	}

	// Nothing listening: the request never left, so it is a confirmed rejection.
	srv := httptest.NewServer(http.NotFoundHandler())
	addr := srv.URL
	srv.Close()
	if _, err := NewJenkins(addr, "u", "t").Trigger(context.Background(), job); err == nil ||
		errors.Is(err, errTriggerUnknown) {
		t.Fatalf("connection refused: %v", err)
	}
}

// A canceled context sends no POST, and the error is not "maybe sent".
func TestJenkinsTriggerCanceledSendsNothing(t *testing.T) {
	var posts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			posts.Add(1)
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := NewJenkins(srv.URL, "u", "t").Trigger(ctx, &JenkinsJob{Path: "f/j"})
	if err == nil || errors.Is(err, errTriggerUnknown) || posts.Load() != 0 {
		t.Fatalf("err %v, posts %d", err, posts.Load())
	}
}

// A redirect after the POST must not be read as "never sent", even when the redirect
// target refuses the connection.
func TestJenkinsTriggerRedirectIsUnknown(t *testing.T) {
	dead := httptest.NewServer(http.NotFoundHandler())
	deadURL := dead.URL
	dead.Close()

	posts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "crumbIssuer") {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		posts++
		http.Redirect(w, r, deadURL+"/queue/item/1/", http.StatusFound)
	}))
	defer srv.Close()

	_, err := NewJenkins(srv.URL, "u", "t").Trigger(context.Background(), &JenkinsJob{Path: "f/j"})
	if !errors.Is(err, errTriggerUnknown) || posts != 1 {
		t.Fatalf("err=%v posts=%d, want errTriggerUnknown after one POST", err, posts)
	}
}

// The credential reaches only the controller's own scheme and host: a queue or build URL from
// jenkins' answer that names another host, plain http, or a redirect elsewhere gets nothing.
func TestJenkinsKeepsTheCredentialOnItsController(t *testing.T) {
	var leaked atomic.Int32
	other := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			leaked.Add(1)
		}
	}))
	defer other.Close()
	controller := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, other.URL+"/job/x/1/api/json", http.StatusFound)
	}))
	defer controller.Close()

	j := NewJenkins(controller.URL, "u", "secret")
	j.HTTP.Transport = controller.Client().Transport

	if _, err := j.BuildFromQueue(t.Context(), other.URL+"/queue/item/1/"); err == nil ||
		!strings.Contains(err.Error(), "refusing to send the jenkins credential") {
		t.Fatalf("queue URL on another host: %v", err)
	}
	if _, err := j.ConsoleText(t.Context(), strings.Replace(controller.URL, "https:", "http:", 1)+"/job/x/1/"); err == nil {
		t.Fatal("a build URL over plain http was allowed")
	}
	if _, _, err := j.Finished(t.Context(), controller.URL+"/job/x/1/"); err == nil {
		t.Fatal("a redirect off the controller was followed")
	}
	if leaked.Load() != 0 {
		t.Fatalf("the credential reached another host %d times", leaked.Load())
	}
}
