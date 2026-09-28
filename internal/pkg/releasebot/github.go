package releasebot

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// GitHub talks to the GitHub REST API; Token may be empty for read-only public calls.
type GitHub struct {
	BaseURL string
	Token   string
	HTTP    *http.Client
}

// NewGitHub returns a client for api.github.com.
func NewGitHub(token string) *GitHub {
	return &GitHub{BaseURL: "https://api.github.com", Token: token, HTTP: &http.Client{Timeout: 30 * time.Second}}
}

var productRepo = map[string]string{"k3s": "k3s-io/k3s", "rke2": "rancher/rke2"}

// TagExists checks that a release tag exists in the product repository.
func (g *GitHub) TagExists(ctx context.Context, product, tag string) (bool, error) {
	repo, ok := productRepo[product]
	if !ok {
		return false, fmt.Errorf("unknown product %q", product)
	}

	resp, err := g.do(ctx, http.MethodGet, fmt.Sprintf("/repos/%s/git/ref/tags/%s", repo, url.PathEscape(tag)), nil)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
		return true, nil
	case http.StatusNotFound:
		return false, nil
	default:
		return false, fmt.Errorf("github tag lookup %s@%s: %s", repo, tag, resp.Status)
	}
}

// Dispatch fires a workflow_dispatch event; GitHub answers 204 with no run id.
func (g *GitHub) Dispatch(ctx context.Context, w WorkflowDispatch) error {
	if g.Token == "" {
		return fmt.Errorf("GITHUB_TOKEN is required to dispatch %s", w.Workflow)
	}

	inputs := map[string]string{}
	for k, v := range w.Inputs {
		if v != "" {
			inputs[k] = v
		}
	}
	body, err := json.Marshal(map[string]any{"ref": w.Ref, "inputs": inputs})
	if err != nil {
		return err
	}

	resp, err := g.do(ctx, http.MethodPost,
		fmt.Sprintf("/repos/%s/actions/workflows/%s/dispatches", w.Repo, w.Workflow), bytes.NewReader(body))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("dispatch %s: %s: %s", w.Workflow, resp.Status, bytes.TrimSpace(msg))
	}

	return nil
}

func (g *GitHub) do(ctx context.Context, method, path string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, g.BaseURL+path, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	if g.Token != "" {
		req.Header.Set("Authorization", "Bearer "+g.Token)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	return g.HTTP.Do(req)
}
