package releasebot

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func writeTestMatrix(t *testing.T, baseURL string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "matrix.yaml")
	raw := fmt.Sprintf(`dtfRef: matrix-ref
controllers:
  mower: {url: %q, maxConcurrent: 1}
defaultParams:
  INSTALL_VERSION: "{{VERSION}}"
jobs:
  - {name: smoke, product: rke2, controller: mower, path: smoke, code: vc}
`, baseURL)
	if err := os.WriteFile(path, []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}

	return path
}

func fakeController(t *testing.T, triggers *atomic.Int32) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	mux.HandleFunc("/job/smoke/api/json", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprint(w, `{"property":[{"parameterDefinitions":[{"name":"INSTALL_VERSION"}]}]}`)
	})
	mux.HandleFunc("/job/smoke/buildWithParameters", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.FormValue("INSTALL_VERSION") != "v1.37.1-rc2+rke2r1" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		triggers.Add(1)
		w.Header().Set("Location", srv.URL+"/queue/item/1/")
		w.WriteHeader(http.StatusCreated)
	})
	mux.HandleFunc("/queue/item/1/api/json", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprintf(w, `{"executable":{"url":%q}}`, srv.URL+"/job/smoke/1/")
	})
	mux.HandleFunc("/job/smoke/1/api/json", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprint(w, `{"building":false,"result":"SUCCESS"}`)
	})

	return srv
}

func TestAppUsesInjectedClientsFromPlanThroughExecution(t *testing.T) {
	var ghCalls, jenkinsCalls, triggers atomic.Int32
	jenkins := fakeController(t, &triggers)
	github := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			w.WriteHeader(http.StatusNoContent)
		}
	}))
	t.Cleanup(github.Close)
	var client *Jenkins
	app := New(&Config{
		MatrixPath: writeTestMatrix(t, jenkins.URL), Poll: time.Millisecond, GitHubToken: "dummy",
		JenkinsAuth: func(string) (string, string, bool) { return "qa", "dummy", true },
		NewGitHub: func(token string) *GitHub {
			ghCalls.Add(1)
			return &GitHub{BaseURL: github.URL, Token: token, HTTP: github.Client()}
		},
		NewJenkins: func(baseURL, user, token string) *Jenkins {
			jenkinsCalls.Add(1)
			client = &Jenkins{BaseURL: baseURL, User: user, Token: token, HTTP: jenkins.Client()}
			return client
		},
	})
	p, err := app.Prepare(testContext(t), Request{RKE2: []string{"v1.37.1-rc2+rke2r1"}})
	if err != nil {
		t.Fatal(err)
	}
	if err = app.Execute(testContext(t), p, nil); err != nil {
		t.Fatal(err)
	}
	got, err := app.JenkinsForBuild(jenkins.URL + "/job/smoke/1/")
	if err != nil || got != client || ghCalls.Load() != 1 || jenkinsCalls.Load() != 1 || triggers.Load() != 1 {
		t.Fatalf("client=%p want=%p err=%v factories=%d/%d triggers=%d",
			got, client, err, ghCalls.Load(), jenkinsCalls.Load(), triggers.Load())
	}
}

func TestControllerClientsReuseAndRefreshCredentials(t *testing.T) {
	var calls atomic.Int32
	token := "first"
	app := New(&Config{
		JenkinsAuth: func(string) (string, string, bool) { return "qa", token, token != "" },
		NewJenkins: func(url, user, token string) *Jenkins {
			calls.Add(1)
			return NewJenkins(url, user, token)
		},
	})
	m := &Matrix{Controller: map[string]Limits{"mower": {URL: "https://mower.example/"}}}
	first := app.controllerClients(m)["mower"]
	if got := app.controllerClients(m)["mower"]; got != first || calls.Load() != 1 {
		t.Fatal("unchanged configuration created another client")
	}
	token = "second"
	second := app.controllerClients(m)["mower"]
	if second == first || second.Token != token || calls.Load() != 2 {
		t.Fatal("changed credentials did not replace the client")
	}
	token = ""
	if _, err := buildersFor(m, app.controllerClients(m)); err == nil {
		t.Fatal("removed credentials retained an authenticated builder")
	}
}

func TestConcurrentControllerLookupReusesClient(t *testing.T) {
	var calls atomic.Int32
	app := New(&Config{
		MatrixPath:  writeTestMatrix(t, "https://mower.example"),
		JenkinsAuth: func(string) (string, string, bool) { return "qa", "dummy", true },
		NewJenkins: func(url, user, token string) *Jenkins {
			calls.Add(1)
			return NewJenkins(url, user, token)
		},
	})
	clients := make(chan *Jenkins, 8)
	var wg sync.WaitGroup
	for range cap(clients) {
		wg.Go(func() {
			j, err := app.JenkinsForBuild("https://mower.example/job/smoke/1/")
			if err != nil {
				t.Error(err)
			}
			clients <- j
		})
	}
	waitGroup(t, &wg)
	close(clients)
	first := receive(t, clients)
	for j := range clients {
		if j != first {
			t.Fatal("concurrent lookups created different clients")
		}
	}
	if first == nil || calls.Load() != 1 {
		t.Fatalf("client=%p factory calls=%d", first, calls.Load())
	}
}

func TestControllerLookupRetainsOlderPlans(t *testing.T) {
	app := New(&Config{
		JenkinsAuth: func(string) (string, string, bool) { return "qa", "dummy", true },
	})
	m := &Matrix{Controller: map[string]Limits{"mower": {URL: "https://old.example"}}}
	old := app.controllerClients(m)["mower"]
	m.Controller["mower"] = Limits{URL: "https://new.example"}
	current := app.controllerClients(m)["mower"]
	for url, want := range map[string]*Jenkins{"https://old.example": old, "https://new.example": current} {
		got, err := app.JenkinsForBuild(url + "/job/smoke/1/")
		if err != nil || got != want {
			t.Fatalf("lookup %s: client=%p want=%p err=%v", url, got, want, err)
		}
	}
}
