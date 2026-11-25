package releasebot

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"
)

// The bump mode's reads: a project's "To Test" issues (needs a token with read:project), the merged
// PRs of an issue, their files, and whether a tag holds a commit.

// projectIssue is an issue a project shows as "To Test".
type projectIssue struct {
	Repo, Title, URL, Body, State string
	Number                        int
}

const toTestQuery = `query($org: String!, $number: Int!, $after: String) {
  organization(login: $org) { projectV2(number: $number) {
    items(first: 100, after: $after) { pageInfo { hasNextPage endCursor } nodes {
      fieldValueByName(name: "Status") { ... on ProjectV2ItemFieldSingleSelectValue { name } }
      content { ... on Issue { number title url body state repository { nameWithOwner } } } } } } } }`

// toTestPage is one page of toTestQuery's answer.
type toTestPage struct {
	Organization struct {
		ProjectV2 *struct {
			Items struct {
				PageInfo struct {
					HasNextPage bool   `json:"hasNextPage"`
					EndCursor   string `json:"endCursor"`
				} `json:"pageInfo"`
				Nodes []struct {
					Status *struct {
						Name string `json:"name"`
					} `json:"fieldValueByName"`
					Content struct {
						Number     int    `json:"number"`
						Title      string `json:"title"`
						URL        string `json:"url"`
						Body       string `json:"body"`
						State      string `json:"state"`
						Repository struct {
							NameWithOwner string `json:"nameWithOwner"`
						} `json:"repository"`
					} `json:"content"`
				} `json:"nodes"`
			} `json:"items"`
		} `json:"projectV2"`
	} `json:"organization"`
}

// ToTestIssues lists the issues of an organization project whose Status is "To Test".
func (g *gitHub) ToTestIssues(ctx context.Context, org string, number int) ([]projectIssue, error) {
	var issues []projectIssue
	after := ""
	for {
		var page toTestPage
		vars := map[string]any{"org": org, "number": number}
		if after != "" {
			vars["after"] = after
		}
		if queryErr := g.graphql(ctx, toTestQuery, vars, &page); queryErr != nil {
			return nil, fmt.Errorf("project %s/%d: %w", org, number, queryErr)
		}
		project := page.Organization.ProjectV2
		if project == nil {
			return nil, fmt.Errorf("project %s/%d not found (does the token have read:project?)", org, number)
		}

		for _, n := range project.Items.Nodes {
			if n.Status == nil || n.Status.Name != "To Test" || n.Content.Number == 0 {
				continue
			}
			c := n.Content
			issues = append(issues, projectIssue{
				Repo: c.Repository.NameWithOwner, Number: c.Number, Title: c.Title, URL: c.URL, Body: c.Body, State: c.State,
			})
		}
		if !project.Items.PageInfo.HasNextPage {
			return issues, nil
		}
		after = project.Items.PageInfo.EndCursor
	}
}

const issuePRsQuery = `query($owner: String!, $name: String!, $number: Int!) {
  repository(owner: $owner, name: $name) { issue(number: $number) {
    timelineItems(first: 100, itemTypes: [CROSS_REFERENCED_EVENT, CONNECTED_EVENT]) { nodes {
      ... on CrossReferencedEvent { source { ... on PullRequest { number merged baseRefName
        mergeCommit { oid } repository { nameWithOwner } } } }
      ... on ConnectedEvent { subject { ... on PullRequest { number merged baseRefName
        mergeCommit { oid } repository { nameWithOwner } } } } } } } } }`

const crossRefIssuesQuery = `query($owner: String!, $name: String!, $number: Int!) {
  repository(owner: $owner, name: $name) { issueOrPullRequest(number: $number) {
    ... on Issue { timelineItems(first: 100, itemTypes: [CROSS_REFERENCED_EVENT]) { nodes {
      ... on CrossReferencedEvent { source { ... on Issue { number title url body state
        repository { nameWithOwner } } } } } } }
    ... on PullRequest { timelineItems(first: 100, itemTypes: [CROSS_REFERENCED_EVENT]) { nodes {
      ... on CrossReferencedEvent { source { ... on Issue { number title url body state
        repository { nameWithOwner } } } } } } }
  } } }`

// BackportIssues are the open backport issues ("[Release-1.37] ...") of repo that reference the issue
// or a pull request that fixed it (a backport's body names one or the other), on any board column.
func (g *gitHub) BackportIssues(ctx context.Context, repo string, issue int) ([]projectIssue, error) {
	prs, prsErr := g.MergedPRs(ctx, repo, issue)
	if prsErr != nil {
		return nil, prsErr
	}

	refs := []int{issue}
	for _, pr := range prs {
		refs = append(refs, pr.Number)
	}

	owner, name, _ := strings.Cut(repo, "/")
	var backports []projectIssue
	for _, n := range refs {
		var out struct {
			Repository struct {
				Item struct {
					TimelineItems struct {
						Nodes []struct {
							Source *struct {
								Number     int    `json:"number"`
								Title      string `json:"title"`
								URL        string `json:"url"`
								Body       string `json:"body"`
								State      string `json:"state"`
								Repository struct {
									NameWithOwner string `json:"nameWithOwner"`
								} `json:"repository"`
							} `json:"source"`
						} `json:"nodes"`
					} `json:"timelineItems"`
				} `json:"issueOrPullRequest"`
			} `json:"repository"`
		}
		vars := map[string]any{"owner": owner, "name": name, "number": n}
		if queryErr := g.graphql(ctx, crossRefIssuesQuery, vars, &out); queryErr != nil {
			return nil, fmt.Errorf("issues referencing %s#%d: %w", repo, n, queryErr)
		}

		for _, node := range out.Repository.Item.TimelineItems.Nodes {
			s := node.Source
			seen := slices.ContainsFunc(backports, func(b projectIssue) bool { return s != nil && b.Number == s.Number })
			if s == nil || s.Number == 0 || s.State != "OPEN" || s.Repository.NameWithOwner != repo ||
				BranchOf(s.Title) == "" || seen {
				continue
			}
			backports = append(backports, projectIssue{Repo: repo, Number: s.Number, Title: s.Title, URL: s.URL,
				Body: s.Body, State: s.State})
		}
	}

	return backports, nil
}

// MergedPRs lists the merged pull requests of repo that reference or close the issue, with their base.
func (g *gitHub) MergedPRs(ctx context.Context, repo string, number int) ([]BumpPR, error) {
	owner, name, _ := strings.Cut(repo, "/")
	type pr struct {
		Number      int    `json:"number"`
		Merged      bool   `json:"merged"`
		BaseRefName string `json:"baseRefName"`
		MergeCommit *struct {
			OID string `json:"oid"`
		} `json:"mergeCommit"`
		Repository struct {
			NameWithOwner string `json:"nameWithOwner"`
		} `json:"repository"`
	}
	var out struct {
		Repository struct {
			Issue struct {
				TimelineItems struct {
					Nodes []struct {
						Source  *pr `json:"source"`
						Subject *pr `json:"subject"`
					} `json:"nodes"`
				} `json:"timelineItems"`
			} `json:"issue"`
		} `json:"repository"`
	}
	vars := map[string]any{"owner": owner, "name": name, "number": number}
	if queryErr := g.graphql(ctx, issuePRsQuery, vars, &out); queryErr != nil {
		return nil, fmt.Errorf("pull requests of %s#%d: %w", repo, number, queryErr)
	}

	seen := map[int]bool{}
	var prs []BumpPR
	for _, n := range out.Repository.Issue.TimelineItems.Nodes {
		p := n.Source
		if p == nil {
			p = n.Subject
		}
		if p == nil || !p.Merged || p.MergeCommit == nil || p.Repository.NameWithOwner != repo || seen[p.Number] {
			continue
		}
		seen[p.Number] = true
		prs = append(prs, BumpPR{Number: p.Number, Base: p.BaseRefName, MergeCommit: p.MergeCommit.OID})
	}

	return prs, nil
}

// ClosingIssue is the first issue a pull request closes (0 when none): a backport may name the
// main issue's pull request instead of the issue.
func (g *gitHub) ClosingIssue(ctx context.Context, repo string, pull int) (int, error) {
	owner, name, _ := strings.Cut(repo, "/")
	var out struct {
		Repository struct {
			PullRequest *struct {
				ClosingIssuesReferences struct {
					Nodes []struct {
						Number int `json:"number"`
					} `json:"nodes"`
				} `json:"closingIssuesReferences"`
			} `json:"pullRequest"`
		} `json:"repository"`
	}
	q := `query($owner: String!, $name: String!, $number: Int!) { repository(owner: $owner, name: $name) {
	  pullRequest(number: $number) { closingIssuesReferences(first: 5) { nodes { number } } } } }`
	if queryErr := g.graphql(ctx, q, map[string]any{"owner": owner, "name": name, "number": pull}, &out); queryErr != nil {
		return 0, fmt.Errorf("issues %s#%d closes: %w", repo, pull, queryErr)
	}

	pr := out.Repository.PullRequest
	if pr == nil || len(pr.ClosingIssuesReferences.Nodes) == 0 {
		return 0, nil
	}

	return pr.ClosingIssuesReferences.Nodes[0].Number, nil
}

// PRFiles returns a pull request's changed files with their patches.
func (g *gitHub) PRFiles(ctx context.Context, repo string, number int) ([]PRFile, error) {
	var files []struct {
		Filename string `json:"filename"`
		Patch    string `json:"patch"`
	}
	paged, getErr := g.getJSON(ctx, fmt.Sprintf("/repos/%s/pulls/%d/files?per_page=100", repo, number), &files)
	if getErr != nil {
		return nil, getErr
	}
	if paged {
		return nil, fmt.Errorf("%s#%d changes more than 100 files: not a bump", repo, number)
	}

	out := make([]PRFile, 0, len(files))
	for _, f := range files {
		out = append(out, PRFile{Name: f.Filename, Patch: f.Patch})
	}

	return out, nil
}

// TagContains reports whether tag (or a branch) already holds commit.
func (g *gitHub) TagContains(ctx context.Context, repo, tag, commit string) (bool, error) {
	var cmp struct {
		Status string `json:"status"`
	}
	if _, getErr := g.getJSON(ctx, fmt.Sprintf("/repos/%s/compare/%s...%s", repo, commit, tag), &cmp); getErr != nil {
		return false, getErr
	}

	// commit...tag is "ahead" (or "identical") when the tag holds the commit.
	return cmp.Status == "ahead" || cmp.Status == "identical", nil
}

// BranchHead is the branch's latest commit.
func (g *gitHub) BranchHead(ctx context.Context, repo, branch string) (string, error) {
	var b struct {
		Commit struct {
			SHA string `json:"sha"`
		} `json:"commit"`
	}
	if _, getErr := g.getJSON(ctx, fmt.Sprintf("/repos/%s/branches/%s", repo, branch), &b); getErr != nil {
		return "", getErr
	}
	if b.Commit.SHA == "" {
		return "", fmt.Errorf("branch %s of %s has no commit", branch, repo)
	}

	return b.Commit.SHA, nil
}

// getJSON decodes a GET; paged reports a next page (the caller decides whether that is an error).
func (g *gitHub) getJSON(ctx context.Context, path string, out any) (paged bool, err error) {
	resp, doErr := g.do(ctx, http.MethodGet, path, nil)
	if doErr != nil {
		return false, doErr
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 256))
		return false, fmt.Errorf("github %s: %s: %s", path, resp.Status, bytes.TrimSpace(msg))
	}
	if decodeErr := json.NewDecoder(resp.Body).Decode(out); decodeErr != nil {
		return false, fmt.Errorf("github %s: %w", path, decodeErr)
	}

	return strings.Contains(resp.Header.Get("Link"), `rel="next"`), nil
}

func (g *gitHub) graphql(ctx context.Context, query string, vars map[string]any, out any) error {
	body, marshalErr := json.Marshal(map[string]any{"query": query, "variables": vars})
	if marshalErr != nil {
		return marshalErr
	}

	resp, doErr := g.do(ctx, http.MethodPost, "/graphql", bytes.NewReader(body))
	if doErr != nil {
		return doErr
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 256))
		return fmt.Errorf("github graphql: %s: %s", resp.Status, bytes.TrimSpace(msg))
	}

	var envelope struct {
		Data   json.RawMessage `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if decodeErr := json.NewDecoder(resp.Body).Decode(&envelope); decodeErr != nil {
		return fmt.Errorf("github graphql: %w", decodeErr)
	}
	if len(envelope.Errors) > 0 {
		return errors.New("github graphql: " + envelope.Errors[0].Message)
	}

	return json.Unmarshal(envelope.Data, out)
}
