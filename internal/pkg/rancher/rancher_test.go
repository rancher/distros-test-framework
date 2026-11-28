package rancher

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// selfSignedPEM returns a CA unrelated to httptest's shared built-in certificate.
func selfSignedPEM(t *testing.T) string {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "other-ca"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour),
		IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}

	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
}

func TestValidateVersion(t *testing.T) {
	cases := []struct {
		product, version string
		ok               bool
	}{
		{"k3s", "v1.37.3-rc1+k3s1", true},
		{"k3s", "v1.36.7+k3s1", true},
		{"rke2", "v1.35.11-rc2+rke2r1", true},
		{"rke2", "v1.37.3+k3s1", false},
		{"k3s", "v1.37.3+rke2r1", false},
		{"k3s", "v1.37.3+k3s1; rm -rf /", false},
		{"k3s", "1.37.3+k3s1", false},
	}
	for _, c := range cases {
		if err := ValidateVersion(c.product, c.version); (err == nil) != c.ok {
			t.Errorf("ValidateVersion(%q, %q) err=%v, want ok=%v", c.product, c.version, err, c.ok)
		}
	}
}

func TestResolveDataURL(t *testing.T) {
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/rancher/kontainer-driver-metadata/pulls/2248":
			_, _ = w.Write([]byte(`{"head":{"sha":"e6a1c51","repo":{"full_name":"rafaelbreno/kontainer-driver-metadata"}}}`))
		case "/repos/rancher/kontainer-driver-metadata/pulls/1":
			_, _ = w.Write([]byte(`{"head":{"sha":"abc","repo":null}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer api.Close()
	ctx := context.Background()

	got, err := ResolveDataURL(ctx, api.URL, "https://github.com/rancher/kontainer-driver-metadata/pull/2248")
	want := "https://raw.githubusercontent.com/rafaelbreno/kontainer-driver-metadata/e6a1c51/data/data.json"
	if err != nil || got != want {
		t.Fatalf("PR resolve = %q, %v; want %q", got, err, want)
	}

	raw := "https://raw.githubusercontent.com/o/r/sha/data/data.json"
	if got, err = ResolveDataURL(ctx, api.URL, raw); err != nil || got != raw {
		t.Fatalf("raw URL passthrough = %q, %v", got, err)
	}

	for _, bad := range []string{
		"http://example.com/data.json",
		"not a url",
		"https://github.com/rancher/kontainer-driver-metadata/pull/1",
		"https://github.com/rancher/kontainer-driver-metadata/pull/404",
	} {
		if _, err = ResolveDataURL(ctx, api.URL, bad); err == nil {
			t.Errorf("ResolveDataURL(%q) succeeded, want error", bad)
		}
	}
}

func TestFetchReleases(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"k3s":{"releases":[{"version":"v1.37.3-rc1+k3s1",
			"minChannelServerVersion":"v2.15.0-alpha1","maxChannelServerVersion":"v2.16.99"}]},
			"rke2":{"releases":[{"version":"v1.37.2+rke2r1"}]}}`))
	}))
	defer srv.Close()

	rels, err := FetchReleases(context.Background(), srv.URL, "k3s")
	if err != nil {
		t.Fatal(err)
	}
	r, ok := FindRelease(rels, "v1.37.3-rc1+k3s1")
	if !ok || r.MaxChannelServerVersion != "v2.16.99" {
		t.Fatalf("FindRelease = %+v, %v", r, ok)
	}
	if _, ok = FindRelease(rels, "v1.37.2+rke2r1"); ok {
		t.Fatal("rke2 release leaked into k3s list")
	}
	if _, err = FetchReleases(context.Background(), srv.URL, "rke1"); err == nil {
		t.Fatal("unsupported product accepted")
	}
}

func rancherServer(t *testing.T) (srv *httptest.Server, caPEM string) {
	t.Helper()
	srv = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v3-public/localProviders/local":
			var in map[string]any
			_ = json.NewDecoder(r.Body).Decode(&in)
			if in["password"] != "s3cret" {
				http.Error(w, `{"message":"unauthorized"}`, http.StatusUnauthorized)
				return
			}
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"token":"token-abc:xyz"}`))
		case "/v3/tokens":
			if r.URL.Query().Get("action") != "logout" || r.Header.Get("Authorization") != "Bearer token-abc:xyz" {
				http.Error(w, "bad logout", http.StatusUnauthorized)
				return
			}
			w.WriteHeader(http.StatusOK)
		case "/v1-k3s-release/releases":
			if r.Header.Get("Authorization") != "Bearer token-abc:xyz" {
				http.Error(w, "no auth", http.StatusUnauthorized)
				return
			}
			_, _ = w.Write([]byte(`{"data":[{"version":"v1.37.2+k3s1"},{"version":"v1.37.3-rc1+k3s1"}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	caPEM = string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw}))

	return srv, caPEM
}

func TestClientLoginAndReleases(t *testing.T) {
	srv, caPEM := rancherServer(t)
	defer srv.Close()
	ctx := context.Background()

	c, err := NewClient(srv.URL, caPEM)
	if err != nil {
		t.Fatal(err)
	}
	if err = c.Login(ctx, "admin", "wrong"); err == nil {
		t.Fatal("login with wrong password succeeded")
	}
	if err = c.Login(ctx, "admin", "s3cret"); err != nil {
		t.Fatal(err)
	}
	if c.Token() != "token-abc:xyz" {
		t.Fatalf("token = %q", c.Token())
	}

	versions, err := c.ChannelReleases(ctx, "k3s")
	if err != nil || strings.Join(versions, ",") != "v1.37.2+k3s1,v1.37.3-rc1+k3s1" {
		t.Fatalf("ChannelReleases = %v, %v", versions, err)
	}

	if err = c.Logout(ctx); err != nil || c.Token() != "" {
		t.Fatalf("Logout = %v, token left %q", err, c.Token())
	}
	if err = c.Logout(ctx); err != nil {
		t.Fatalf("second Logout should be a no-op: %v", err)
	}
}

func TestClientRejectsUntrustedCertificate(t *testing.T) {
	srv, _ := rancherServer(t)
	defer srv.Close()
	otherCA := selfSignedPEM(t)

	c, err := NewClient(srv.URL, otherCA)
	if err != nil {
		t.Fatal(err)
	}
	if err = c.Login(context.Background(), "admin", "s3cret"); err == nil {
		t.Fatal("login succeeded against a certificate outside the pinned CA")
	}
	if _, err = LeafSPKI(context.Background(), srv.URL, otherCA); err == nil {
		t.Fatal("LeafSPKI accepted a certificate outside the pinned CA")
	}
	if _, err = NewClient(srv.URL, "garbage"); err == nil {
		t.Fatal("NewClient accepted a CA without PEM")
	}
}

func TestLeafSPKI(t *testing.T) {
	srv, caPEM := rancherServer(t)
	defer srv.Close()

	got, err := LeafSPKI(context.Background(), srv.URL, caPEM)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(srv.Certificate().RawSubjectPublicKeyInfo)
	if want := base64.StdEncoding.EncodeToString(sum[:]); got != want {
		t.Fatalf("LeafSPKI = %s, want %s", got, want)
	}
}

func TestControlPlanePhases(t *testing.T) {
	cp, err := ParseControlPlane(`{"status":{"conditions":[{"type":"Ready","status":"True"}],
		"etcdSnapshotCreate":{"generation":2},"etcdSnapshotCreatePhase":"Finished",
		"etcdSnapshotRestore":{"generation":1},"etcdSnapshotRestorePhase":"Restore"}}`)
	if err != nil {
		t.Fatal(err)
	}
	if !cp.SnapshotCreated(2) || cp.SnapshotCreated(3) {
		t.Error("SnapshotCreated generation match wrong")
	}
	if cp.SnapshotRestored(1) {
		t.Error("restore reported finished while phase is Restore")
	}

	notReady, _ := ParseControlPlane(`{"status":{"conditions":[{"type":"Ready","status":"False"}],
		"etcdSnapshotCreate":{"generation":2},"etcdSnapshotCreatePhase":"Finished"}}`)
	if notReady.SnapshotCreated(2) {
		t.Error("snapshot reported created while control plane is not Ready")
	}
}

func TestNewSnapshot(t *testing.T) {
	raw := `{"items":[
		{"metadata":{"name":"old"},"snapshotFile":{"size":10,"createdAt":"2026-11-27T10:00:00Z"}},
		{"metadata":{"name":"failed"},"snapshotFile":{"size":10,"status":"failed","createdAt":"2026-11-27T12:00:00Z"}},
		{"metadata":{"name":"empty"},"snapshotFile":{"size":0,"createdAt":"2026-11-27T12:00:00Z"}},
		{"metadata":{"name":"new-a"},"snapshotFile":{"size":10,"createdAt":"2026-11-27T11:00:00Z"}},
		{"metadata":{"name":"new-b"},"snapshotFile":{"name":"on-demand-b","size":10,"createdAt":"2026-11-27T11:30:00Z"}}]}`
	before, err := SnapshotNames(`{"items":[{"metadata":{"name":"old"}}]}`)
	if err != nil {
		t.Fatal(err)
	}

	got, file, err := NewSnapshot(raw, before)
	if err != nil || got != "new-b" || file != "on-demand-b" {
		t.Fatalf("NewSnapshot = %q, %q, %v; want new-b, on-demand-b", got, file, err)
	}
	if _, _, err = NewSnapshot(`{"items":[{"metadata":{"name":"old"},"snapshotFile":{"size":1}}]}`, before); err == nil {
		t.Fatal("NewSnapshot returned a snapshot that existed before the request")
	}
}

func TestRegistrationAndImportCommand(t *testing.T) {
	if _, err := ParseRegistrationToken(`{"items":[{"status":{"token":"t"}}]}`); err == nil {
		t.Fatal("token without commands accepted")
	}

	// Rancher 2.14 (observed): status.token is empty and the commands carry a {token} placeholder.
	tok, err := ParseRegistrationToken(`{"items":[{"status":{"token":"","tokenSecretName":"crt-abc",
		"insecureNodeCommand":"curl --insecure -fL https://r/system-agent-install.sh | sudo  sh -s - --server https://r ` +
		`--label 'cattle.io/os=linux' --token {token} --ca-checksum ff",
		"insecureCommand":"curl --insecure -sfL https://r/v3/import/{token}_c-m-1.yaml | kubectl apply -f -"}}]}`)
	if err != nil || !tok.NeedsSecret() || tok.TokenSecretName != "crt-abc" {
		t.Fatalf("templated token: %+v, %v", tok, err)
	}
	tok.Fill("realtoken54")
	if tok.NeedsSecret() || !strings.Contains(tok.InsecureNodeCommand, "--token realtoken54 --ca-checksum") ||
		!strings.Contains(tok.InsecureCommand, "/v3/import/realtoken54_c-m-1.yaml") {
		t.Fatalf("Fill left templates: %+v", tok)
	}
	if got := tok.Secrets(); got[0] != "realtoken54" {
		t.Fatalf("Secrets() = %q", got)
	}

	templatedOnly := `{"items":[{"status":{"insecureCommand":"x/v3/import/{token}_c.yaml | kubectl apply -f -"}}]}`
	if _, err = ParseRegistrationToken(templatedOnly); err == nil {
		t.Fatal("templated commands without a token secret accepted")
	}

	legacy, err := ParseRegistrationToken(`{"items":[{"status":{"token":"statustok",
		"insecureNodeCommand":"curl ... | sh -s - --token statustok"}}]}`)
	if err != nil || legacy.NeedsSecret() || legacy.Token != "statustok" {
		t.Fatalf("pre-2.14 token: %+v, %v", legacy, err)
	}

	cmd, err := ImportCommand(tok.InsecureCommand, "sudo k3s kubectl")
	if err != nil || !strings.HasSuffix(cmd, "| sudo k3s kubectl apply -f -") {
		t.Fatalf("ImportCommand = %q, %v", cmd, err)
	}
	if _, err = ImportCommand("curl https://r/x.yaml | oc apply -f -", "kubectl"); err == nil {
		t.Fatal("ImportCommand accepted an unexpected format")
	}
}

func TestClusterManifest(t *testing.T) {
	custom, err := ClusterManifest("day1-custom", "v1.37.2+k3s1")
	if err != nil {
		t.Fatal(err)
	}
	var m struct {
		Metadata struct{ Name, Namespace string }
		Spec     map[string]any
	}
	if err = json.Unmarshal([]byte(custom), &m); err != nil {
		t.Fatal(err)
	}
	if m.Metadata.Namespace != FleetNamespace || m.Spec["kubernetesVersion"] != "v1.37.2+k3s1" ||
		m.Spec["rkeConfig"] == nil {
		t.Fatalf("custom manifest = %s", custom)
	}

	imported, _ := ClusterManifest("day1-import", "")
	if strings.Contains(imported, "rkeConfig") || strings.Contains(imported, "kubernetesVersion") {
		t.Fatalf("imported manifest must have an empty spec: %s", imported)
	}
}

func TestVersionUnderTest(t *testing.T) {
	const rc = "v1.35.11-rc1+rke2r1"
	for _, k8s := range []string{"", rc} {
		if got, err := VersionUnderTest(rc, k8s); err != nil || got != rc {
			t.Errorf("VersionUnderTest(%q, %q) = %q, %v; want %q", rc, k8s, got, err, rc)
		}
	}
	if _, err := VersionUnderTest(rc, "v1.35.10+rke2r1"); err == nil {
		t.Error("a k8sVersion different from INSTALL_VERSION was accepted")
	}
}

func TestReportedVersion(t *testing.T) {
	for in, want := range map[string]string{
		"v1.35.11-rc1+rke2r1": "v1.35.11+rke2r1", // node VERSION observed on a v1.35.11-rc1 install
		"v1.37.3-rc12+k3s1":   "v1.37.3+k3s1",
		"v1.35.10+rke2r1":     "v1.35.10+rke2r1",
	} {
		if got := ReportedVersion(in); got != want {
			t.Errorf("ReportedVersion(%q) = %q, want %q", in, got, want)
		}
	}
}
