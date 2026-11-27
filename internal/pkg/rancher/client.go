package rancher

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"time"
)

// Client talks to the Rancher API over TLS verified against the Rancher CA.
type Client struct {
	baseURL string
	http    *http.Client
	token   string
}

// NewClient builds a client for baseURL; an empty caPEM means Rancher serves a publicly trusted certificate.
func NewClient(baseURL, caPEM string) (*Client, error) {
	tlsCfg, err := tlsConfig(caPEM)
	if err != nil {
		return nil, err
	}

	return &Client{
		baseURL: baseURL,
		http: &http.Client{
			Timeout:   60 * time.Second,
			Transport: &http.Transport{TLSClientConfig: tlsCfg, Proxy: http.ProxyFromEnvironment},
		},
	}, nil
}

// Token returns the session token from Login. Treat it as a secret.
func (c *Client) Token() string { return c.token }

// Login exchanges local credentials for a session token.
func (c *Client) Login(ctx context.Context, username, password string) error {
	body := map[string]any{
		"username":     username,
		"password":     password,
		"responseType": "token",
		"description":  "distros-test-framework rancherday1",
		"ttl":          (8 * time.Hour).Milliseconds(),
	}
	var out struct {
		Token string `json:"token"`
	}
	if err := c.do(ctx, http.MethodPost, "/v3-public/localProviders/local?action=login", body, &out); err != nil {
		return fmt.Errorf("rancher login as %s: %w", username, err)
	}
	if out.Token == "" {
		return errors.New("rancher login returned an empty token")
	}
	c.token = out.Token

	return nil
}

// ChannelReleases lists the versions Rancher offers for product, the same list the UI dropdowns use.
func (c *Client) ChannelReleases(ctx context.Context, product string) ([]string, error) {
	var out struct {
		Data []struct {
			Version string `json:"version"`
		} `json:"data"`
	}
	if err := c.do(ctx, http.MethodGet, "/v1-"+product+"-release/releases", nil, &out); err != nil {
		return nil, fmt.Errorf("list %s releases: %w", product, err)
	}

	versions := make([]string, 0, len(out.Data))
	for _, d := range out.Data {
		versions = append(versions, d.Version)
	}

	return versions, nil
}

func (c *Client) do(ctx context.Context, method, path string, in, out any) error {
	var body io.Reader = http.NoBody
	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			return err
		}
		body = bytes.NewReader(b)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, body)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, maxMetadataBytes))
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return fmt.Errorf("%s %s: status %d: %s", method, path, resp.StatusCode, truncate(respBody))
	}
	if out == nil {
		return nil
	}

	return json.Unmarshal(respBody, out)
}

// LeafSPKI dials baseURL, verifies its certificate against caPEM, and returns the
// leaf's base64 SPKI sha256 so a browser can trust exactly that key and nothing else.
func LeafSPKI(ctx context.Context, baseURL, caPEM string) (string, error) {
	u, err := url.Parse(baseURL)
	if err != nil {
		return "", err
	}
	tlsCfg, err := tlsConfig(caPEM)
	if err != nil {
		return "", err
	}
	tlsCfg.ServerName = u.Hostname()

	addr := u.Host
	if u.Port() == "" {
		addr = net.JoinHostPort(u.Hostname(), "443")
	}
	dialer := &tls.Dialer{NetDialer: &net.Dialer{Timeout: 30 * time.Second}, Config: tlsCfg}
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return "", fmt.Errorf("verify rancher certificate at %s: %w", addr, err)
	}
	defer conn.Close()

	leaf := conn.(*tls.Conn).ConnectionState().PeerCertificates[0]
	sum := sha256.Sum256(leaf.RawSubjectPublicKeyInfo)

	return base64.StdEncoding.EncodeToString(sum[:]), nil
}

func tlsConfig(caPEM string) (*tls.Config, error) {
	cfg := &tls.Config{MinVersion: tls.VersionTLS12}
	if caPEM == "" {
		return cfg, nil
	}

	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM([]byte(caPEM)) {
		return nil, errors.New("rancher cacerts setting holds no valid PEM certificate")
	}
	cfg.RootCAs = pool

	return cfg, nil
}
