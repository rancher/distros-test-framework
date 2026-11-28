package ui

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const dropdownPage = `<html><body>
<div data-testid="clusterBasics__kubernetesVersions"
     onclick="document.getElementById('m').style.display='block'">Kubernetes Version</div>
<ul id="m" class="vs__dropdown-menu" style="display:none">
  <li>v1.37.2+k3s1</li><li>v1.37.3-rc1+k3s1</li>
</ul>
<script>console.log(document.cookie.includes('R_SESS') ? 'cookie visible' : 'cookie httpOnly')</script>
</body></html>`

// welcomeOverlay mimics Rancher Prime 2.16's first-load popup: it appears late and covers the page.
func welcomeOverlay(closable bool) string {
	closeButton := ""
	if closable {
		closeButton = `<button aria-label="Close" onclick="document.getElementById('w').remove()">x</button>`
	}

	return `<div id="w" role="dialog" style="display:none;position:fixed;inset:0;background:#fff;z-index:9">
<h2>Welcome to Rancher Prime</h2>` + closeButton + `</div>
<script>setTimeout(() => { document.getElementById('w').style.display = 'block'; }, 1500)</script></body>`
}

func requireChrome(t *testing.T) {
	t.Helper()
	for _, name := range []string{"chromium", "chromium-browser", "google-chrome", "headless-shell"} {
		if _, err := exec.LookPath(name); err == nil {
			return
		}
	}
	if _, err := os.Stat("/Applications/Google Chrome.app"); err == nil {
		return
	}
	t.Skip("no Chrome/Chromium installed")
}

func testServer(t *testing.T) (srv *httptest.Server, spki string, gotCookie *string) {
	t.Helper()
	gotCookie = new(string)
	srv = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if c, err := r.Cookie("R_SESS"); err == nil {
			*gotCookie = c.Value
		}
		switch r.URL.Path {
		case "/welcome":
			_, _ = w.Write([]byte(strings.Replace(dropdownPage, "</body>", welcomeOverlay(true), 1)))
		case "/welcome-stuck":
			_, _ = w.Write([]byte(strings.Replace(dropdownPage, "</body>", welcomeOverlay(false), 1)))
		case "/secret":
			_, _ = w.Write([]byte(`<html><body>Registration: curl ... --token token-abc:xyz</body></html>`))
		default:
			_, _ = w.Write([]byte(dropdownPage))
		}
	}))
	sum := sha256.Sum256(srv.Certificate().RawSubjectPublicKeyInfo)

	return srv, base64.StdEncoding.EncodeToString(sum[:]), gotCookie
}

func TestBrowserDropdownAndScreenshots(t *testing.T) {
	requireChrome(t)
	srv, spki, gotCookie := testServer(t)
	defer srv.Close()

	b, err := New(Options{BaseURL: srv.URL, SessionToken: "token-abc:xyz", TrustedSPKI: spki, ArtifactsDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()

	if err = b.Open("/create", "Kubernetes Version", 30*time.Second); err != nil {
		t.Fatal(err)
	}
	if *gotCookie != "token-abc:xyz" {
		t.Fatalf("R_SESS cookie not sent, got %q", *gotCookie)
	}

	opts, err := b.DropdownOptions(`[data-testid="clusterBasics__kubernetesVersions"]`, "v1.37.3-rc1+k3s1", 30*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(opts, ",") != "v1.37.2+k3s1,v1.37.3-rc1+k3s1" {
		t.Fatalf("options = %v", opts)
	}

	path, err := b.Screenshot("create dropdown/k3s")
	if err != nil {
		t.Fatal(err)
	}
	png, err := os.ReadFile(path)
	if err != nil || !bytes.HasPrefix(png, []byte("\x89PNG")) || !strings.HasSuffix(path, "01-create_dropdown_k3s.png") {
		t.Fatalf("screenshot %s invalid: %v", path, err)
	}

	if err = b.Open("/secret", "Registration", 30*time.Second); err != nil {
		t.Fatal(err)
	}
	if _, err = b.Screenshot("secret"); err == nil || !strings.Contains(err.Error(), "refusing screenshot") {
		t.Fatalf("screenshot of a page showing the session token was not refused: %v", err)
	}

	console, err := b.SaveConsole("run")
	if err != nil {
		t.Fatal(err)
	}
	if log, _ := os.ReadFile(console); !strings.Contains(string(log), "cookie httpOnly") {
		t.Fatalf("console log missing page message or cookie not httpOnly: %q", log)
	}
}

func TestBrowserRejectsUnpinnedCertificate(t *testing.T) {
	requireChrome(t)
	srv, _, _ := testServer(t)
	defer srv.Close()

	b, err := New(Options{BaseURL: srv.URL, ArtifactsDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()

	err = b.Open("/create", "Kubernetes Version", 15*time.Second)
	if err == nil || !strings.Contains(err.Error(), "ERR_CERT") {
		t.Fatalf("untrusted self-signed certificate not rejected by Chrome: %v", err)
	}
}

func TestBrowserClosesWelcomeDialog(t *testing.T) {
	requireChrome(t)
	srv, spki, _ := testServer(t)
	defer srv.Close()
	dir := t.TempDir()

	b, err := New(Options{BaseURL: srv.URL, TrustedSPKI: spki, ArtifactsDir: dir, WelcomeWait: 5 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()

	if err = b.Open("/welcome", "Kubernetes Version", 30*time.Second); err != nil {
		t.Fatal(err)
	}
	// The overlay covers the select, so reading the options proves the dialog was closed.
	opts, err := b.DropdownOptions(`[data-testid="clusterBasics__kubernetesVersions"]`, "v1.37.3-rc1+k3s1", 15*time.Second)
	if err != nil || len(opts) != 2 {
		t.Fatalf("dropdown behind the welcome dialog: %v, %v", opts, err)
	}
	if shots, _ := filepath.Glob(filepath.Join(dir, "*-rancher-welcome-dialog.png")); len(shots) != 1 {
		t.Fatalf("expected one screenshot of the welcome dialog, got %v", shots)
	}
}

func TestBrowserReportsUnclosableWelcomeDialog(t *testing.T) {
	requireChrome(t)
	srv, spki, _ := testServer(t)
	defer srv.Close()
	dir := t.TempDir()

	b, err := New(Options{BaseURL: srv.URL, TrustedSPKI: spki, ArtifactsDir: dir, WelcomeWait: 5 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()

	err = b.Open("/welcome-stuck", "Kubernetes Version", 30*time.Second)
	if err == nil || !strings.Contains(err.Error(), "no recognizable close control") {
		t.Fatalf("expected an unclosable-dialog error, got %v", err)
	}
	html, readErr := os.ReadFile(filepath.Join(dir, "welcome-dialog.html"))
	if readErr != nil || !strings.Contains(string(html), "Welcome to Rancher Prime") {
		t.Fatalf("dialog markup not saved: %v", readErr)
	}
}
