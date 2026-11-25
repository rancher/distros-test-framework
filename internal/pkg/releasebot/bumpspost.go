package releasebot

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
)

// Posting a bump MD, only when the captain runs -bumps-post after reading it: each section on its
// main issue, a link to that comment on each backport, and (with -bumps-close) the issues closed.

// Comment posts body on an issue and returns the comment's URL.
func (g *gitHub) Comment(ctx context.Context, repo string, issue int, body string) (string, error) {
	raw, marshalErr := json.Marshal(map[string]string{"body": body})
	if marshalErr != nil {
		return "", marshalErr
	}

	resp, doErr := g.do(ctx, http.MethodPost, fmt.Sprintf("/repos/%s/issues/%d/comments", repo, issue), bytes.NewReader(raw))
	if doErr != nil {
		return "", doErr
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 256))
		return "", fmt.Errorf("comment on %s#%d: %s: %s", repo, issue, resp.Status, bytes.TrimSpace(msg))
	}
	var c struct {
		HTMLURL string `json:"html_url"`
	}
	if decodeErr := json.NewDecoder(resp.Body).Decode(&c); decodeErr != nil {
		return "", fmt.Errorf("comment on %s#%d: %w", repo, issue, decodeErr)
	}

	return c.HTMLURL, nil
}

// CloseIssue closes an issue as completed.
func (g *gitHub) CloseIssue(ctx context.Context, repo string, issue int) error {
	raw := []byte(`{"state":"closed","state_reason":"completed"}`)
	resp, doErr := g.do(ctx, http.MethodPatch, fmt.Sprintf("/repos/%s/issues/%d", repo, issue), bytes.NewReader(raw))
	if doErr != nil {
		return doErr
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 256))
		return fmt.Errorf("close %s#%d: %s: %s", repo, issue, resp.Status, bytes.TrimSpace(msg))
	}

	return nil
}

// bumpPoster is what posting needs from GitHub (gitHub; a fake in tests).
type bumpPoster interface {
	Comment(ctx context.Context, repo string, issue int, body string) (string, error)
	CloseIssue(ctx context.Context, repo string, issue int) error
}

// PostBumpMD posts the file's sections. What was done is kept in <file>.posted.json, so a second run
// after a failure goes on where the first stopped instead of posting twice.
func PostBumpMD(ctx context.Context, gh bumpPoster, file string, closeIssues bool, log logFunc) error {
	raw, readErr := os.ReadFile(file)
	if readErr != nil {
		return readErr
	}
	sections, parseErr := ParseBumpMD(string(raw))
	if parseErr != nil {
		return parseErr
	}
	if len(sections) == 0 {
		return fmt.Errorf("%s has no bump section to post", file)
	}

	ledger := file + ".posted.json"
	done := map[string]string{}
	if prev, prevErr := os.ReadFile(ledger); prevErr == nil {
		if decodeErr := json.Unmarshal(prev, &done); decodeErr != nil {
			return fmt.Errorf("read %s: %w", ledger, decodeErr)
		}
	}
	record := func(key, value string) error {
		done[key] = value
		out, _ := json.MarshalIndent(done, "", " ")

		return writeDurable(ledger, out)
	}

	for _, s := range sections {
		if postErr := postBumpSection(ctx, gh, s, closeIssues, done, record, log); postErr != nil {
			return postErr
		}
	}

	return nil
}

// postBumpSection posts s on its main issue, a link to it on each backport and, when asked, closes
// them all; done and record are the ledger of what was already done.
func postBumpSection(
	ctx context.Context, gh bumpPoster, s BumpSection, closeIssues bool, done map[string]string,
	record func(key, value string) error, log logFunc,
) error {
	mainKey := fmt.Sprintf("comment %s#%d", s.Repo, s.Issue)
	url := done[mainKey]
	if url == "" {
		posted, commentErr := gh.Comment(ctx, s.Repo, s.Issue, s.Body)
		if commentErr != nil {
			return commentErr
		}
		url = posted
		if recordErr := record(mainKey, url); recordErr != nil {
			return recordErr
		}
	}
	log("info", "%s#%d: %s", s.Repo, s.Issue, url)

	for _, bp := range s.Backports {
		if postErr := once(done, fmt.Sprintf("link %s#%d", s.Repo, bp), record, func() error {
			_, linkErr := gh.Comment(ctx, s.Repo, bp, "Validated here: "+url)
			return linkErr
		}); postErr != nil {
			return postErr
		}
	}

	if !closeIssues {
		return nil
	}
	for _, n := range append(append([]int{}, s.Backports...), s.Issue) {
		if closeErr := once(done, fmt.Sprintf("close %s#%d", s.Repo, n), record, func() error {
			return gh.CloseIssue(ctx, s.Repo, n)
		}); closeErr != nil {
			return closeErr
		}
	}
	log("info", "%s#%d and its %d backports closed", s.Repo, s.Issue, len(s.Backports))

	return nil
}

// once runs step unless the ledger already has key, and records it when it succeeds.
func once(done map[string]string, key string, record func(string, string) error, step func() error) error {
	if done[key] != "" {
		return nil
	}
	if stepErr := step(); stepErr != nil {
		return stepErr
	}

	return record(key, "done")
}

// PostBumpMD posts with the app's GitHub token (it needs to comment on and close the issues).
func (a *App) PostBumpMD(ctx context.Context, file string, closeIssues bool) error {
	return PostBumpMD(ctx, a.cfg.NewGitHub(a.cfg.GitHubToken), file, closeIssues, a.cfg.Log)
}
