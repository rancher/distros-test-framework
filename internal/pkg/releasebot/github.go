package releasebot

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type GitHub struct {
	BaseURL string
	Token   string
	HTTP    *http.Client
}

func newGitHub(token string) *GitHub {
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

// LatestGA returns the newest GA release of rc's minor that is older than rc: the version an
// upgrade to rc starts from. A .0 RC has none yet, so the previous minor is used and note says so.
func (g *GitHub) LatestGA(ctx context.Context, product, rc string) (version, note string, err error) {
	target, ok := parseRelease(rc)
	if !ok {
		return "", "", fmt.Errorf("not a release tag: %q", rc)
	}

	for minor := target.minor; minor >= 0 && minor >= target.minor-1; minor-- {
		tags, tagErr := g.matchingTags(ctx, product, fmt.Sprintf("v%d.%d.", target.major, minor))
		if tagErr != nil {
			return "", "", tagErr
		}

		best, bestRel := "", release{}
		for _, t := range tags {
			r, parsed := parseRelease(t)
			if !parsed || r.rc != 0 || r.minor != minor || !r.less(target) {
				continue
			}
			if best == "" || bestRel.less(r) {
				best, bestRel = t, r
			}
		}
		if best != "" {
			if minor != target.minor {
				note = fmt.Sprintf("%s: no v%d.%d GA yet, upgrading from %s", rc, target.major, target.minor, best)
			}

			return best, note, nil
		}
	}

	return "", "", fmt.Errorf("%s: no GA release of v%d.%d or the previous minor", rc, target.major, target.minor)
}

// matchingTags lists every tag starting with prefix in one unpaged call (git matching-refs).
func (g *GitHub) matchingTags(ctx context.Context, product, prefix string) ([]string, error) {
	repo, ok := productRepo[product]
	if !ok {
		return nil, fmt.Errorf("unknown product %q", product)
	}

	resp, err := g.do(ctx, http.MethodGet, fmt.Sprintf("/repos/%s/git/matching-refs/tags/%s", repo, prefix), nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("github tags %s@%s*: %s", repo, prefix, resp.Status)
	}

	// A truncated answer could hide the newest GA; refuse it rather than pick an older one.
	if strings.Contains(resp.Header.Get("Link"), `rel="next"`) {
		return nil, fmt.Errorf("github tags %s@%s*: the answer is paged; not all tags were read", repo, prefix)
	}
	var refs []struct {
		Ref string `json:"ref"`
	}
	if err = json.NewDecoder(resp.Body).Decode(&refs); err != nil {
		return nil, fmt.Errorf("github tags %s@%s*: %w", repo, prefix, err)
	}

	tags := make([]string, 0, len(refs))
	for _, r := range refs {
		tags = append(tags, strings.TrimPrefix(r.Ref, "refs/tags/"))
	}

	return tags, nil
}

// ErrDispatchRejected means GitHub answered the dispatch with a 4xx: no workflow run was created.
var ErrDispatchRejected = errors.New("dispatch rejected")

// ErrDispatchNotSent means the dispatch never left the bot (no token, a canceled run, no connection).
var ErrDispatchNotSent = errors.New("dispatch not sent")

func (g *GitHub) Dispatch(ctx context.Context, w WorkflowDispatch) error {
	if g.Token == "" {
		return fmt.Errorf("%w: GITHUB_TOKEN is required to dispatch %s", ErrDispatchNotSent, w.Workflow)
	}

	inputs := map[string]string{}
	for k, v := range w.Inputs {
		if v != "" {
			inputs[k] = v
		}
	}

	body, err := json.Marshal(map[string]any{"ref": w.Ref, "inputs": inputs})
	if err != nil {
		return fmt.Errorf("%w: %w", ErrDispatchNotSent, err)
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		return fmt.Errorf("%w: %w", ErrDispatchNotSent, ctxErr)
	}

	resp, err := g.do(ctx, http.MethodPost,
		fmt.Sprintf("/repos/%s/actions/workflows/%s/dispatches", w.Repo, w.Workflow), bytes.NewReader(body))
	if err != nil {
		if notSent(err) {
			return fmt.Errorf("%w: %w", ErrDispatchNotSent, err)
		}

		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		err = fmt.Errorf("dispatch %s: %s: %s", w.Workflow, resp.Status, bytes.TrimSpace(msg))
		if resp.StatusCode >= 400 && resp.StatusCode < 500 {
			err = fmt.Errorf("%w: %w", ErrDispatchRejected, err)
		}

		return err
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
