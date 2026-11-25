package releasebot

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"slices"
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

// OpenBackports are the numbers of the open backports of the issue (BackportIssues).
func (g *gitHub) OpenBackports(ctx context.Context, repo string, issue int) ([]int, error) {
	backports, backportsErr := g.BackportIssues(ctx, repo, issue)
	if backportsErr != nil {
		return nil, backportsErr
	}

	numbers := make([]int, 0, len(backports))
	for _, b := range backports {
		numbers = append(numbers, b.Number)
	}

	return numbers, nil
}

// bumpPoster is what posting needs from GitHub (gitHub; a fake in tests).
type bumpPoster interface {
	Comment(ctx context.Context, repo string, issue int, body string) (string, error)
	CloseIssue(ctx context.Context, repo string, issue int) error
	OpenBackports(ctx context.Context, repo string, issue int) ([]int, error)
	IssueRef(ctx context.Context, repo string, n int) (title, body string, pull bool, err error)
	FindComment(ctx context.Context, repo string, issue int, marker string) (string, error)
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

	release, lockErr := AcquireInstanceLock(file + ".lock")
	if lockErr != nil {
		return fmt.Errorf("post %s: %w", file, lockErr)
	}
	defer release()
	if issuesErr := checkSectionIssues(ctx, gh, sections); issuesErr != nil {
		return issuesErr
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

	for i := range sections {
		if postErr := postBumpSection(ctx, gh, &sections[i], closeIssues, done, record, log); postErr != nil {
			return postErr
		}
	}

	return nil
}

// postBumpSection posts s on its main issue, links it on each backport and, when asked, closes them;
// done and record are the ledger of what was already done.
func postBumpSection(
	ctx context.Context, gh bumpPoster, s *BumpSection, closeIssues bool, done map[string]string,
	record func(key, value string) error, log logFunc,
) error {
	// Keyed by the body: new results for the issue are a new comment. The marker finds a comment whose
	// reply was lost, so a rerun does not post it twice.
	sum := sha256.Sum256([]byte(s.Body))
	mainKey := fmt.Sprintf("comment %s#%d %x", s.Repo, s.Issue, sum)
	marker := fmt.Sprintf("<!-- releasebot bump %x -->", sum[:6])
	url := done[mainKey]
	if url == "" {
		posted, postErr := commentOnce(ctx, gh, s.Repo, s.Issue, s.Body+"\n"+marker+"\n", marker)
		if postErr != nil {
			return postErr
		}
		url = posted
		if recordErr := record(mainKey, url); recordErr != nil {
			return recordErr
		}
	}
	log("info", "%s#%d: %s", s.Repo, s.Issue, url)

	for _, bp := range s.Backports {
		link := "Validated here: " + url
		if postErr := once(done, fmt.Sprintf("link %s#%d", s.Repo, bp), record, func() error {
			_, linkErr := commentOnce(ctx, gh, s.Repo, bp, link, link)
			return linkErr
		}); postErr != nil {
			return postErr
		}
	}

	if !closeIssues {
		return nil
	}

	return closeBumpSection(ctx, gh, s, done, record, log)
}

// closeBumpSection closes the section's validated backports, then its main issue once none of the
// main issue's backports is open.
func closeBumpSection(
	ctx context.Context, gh bumpPoster, s *BumpSection, done map[string]string, record func(key, value string) error,
	log logFunc,
) error {
	for _, n := range s.Backports {
		if closeErr := once(done, fmt.Sprintf("close %s#%d", s.Repo, n), record, func() error {
			return gh.CloseIssue(ctx, s.Repo, n)
		}); closeErr != nil {
			return closeErr
		}
	}

	// The main issue closes once no backport of it is open: those of the section not validated, and
	// any GitHub knows that is not on the board (still in progress, say).
	open, openErr := gh.OpenBackports(ctx, s.Repo, s.Issue)
	if openErr != nil {
		return openErr
	}
	for _, n := range open {
		if !slices.Contains(s.Backports, n) && !slices.Contains(s.Open, n) {
			s.Open = append(s.Open, n)
		}
	}
	if len(s.Open) > 0 {
		log("info", "%s#%d stays open (backports %v not validated); its %d validated backports closed",
			s.Repo, s.Issue, s.Open, len(s.Backports))
		return nil
	}

	if closeErr := once(done, fmt.Sprintf("close %s#%d", s.Repo, s.Issue), record, func() error {
		return gh.CloseIssue(ctx, s.Repo, s.Issue)
	}); closeErr != nil {
		return closeErr
	}
	log("info", "%s#%d and its %d backports closed", s.Repo, s.Issue, len(s.Backports))

	return nil
}

// commentOnce posts body unless the issue already has a comment holding marker (a post whose reply was
// lost), and returns the comment's URL.
func commentOnce(ctx context.Context, gh bumpPoster, repo string, issue int, body, marker string) (string, error) {
	url, findErr := gh.FindComment(ctx, repo, issue, marker)
	if findErr != nil {
		return "", fmt.Errorf("comments of %s#%d: %w", repo, issue, findErr)
	}
	if url != "" {
		return url, nil
	}

	return gh.Comment(ctx, repo, issue, body)
}

// checkSectionIssues refuses a file whose sections name a pull request: closing one would close it.
func checkSectionIssues(ctx context.Context, gh bumpPoster, sections []BumpSection) error {
	for _, s := range sections {
		for _, n := range append([]int{s.Issue}, s.Backports...) {
			_, _, pull, refErr := gh.IssueRef(ctx, s.Repo, n)
			if refErr != nil {
				return fmt.Errorf("check %s#%d: %w", s.Repo, n, refErr)
			}
			if pull {
				return fmt.Errorf("%s#%d is a pull request, not an issue: nothing posted", s.Repo, n)
			}
		}
	}

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
