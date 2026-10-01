package releasebot

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/rancher/distros-test-framework/internal/pkg/qase"
)

// qaseRunTitle mirrors scripts/qase-patch-validation.sh; the script hardcodes the rke2r1/k3s1 suffix
// whatever the tag says, so this must stay in sync with it.
func qaseRunTitle(product, tag string, now time.Time) string {
	suffix := map[string]string{"rke2": "rke2r1", "k3s": "k3s1"}[product]

	return fmt.Sprintf("%s %s %d Patch Validation for %s+%s",
		strings.ToUpper(product), now.UTC().Format("January"), now.UTC().Year(), baseVersion(tag), suffix)
}

// QaseRun is the part of a Qase run the bot matches on.
type QaseRun struct {
	ID          int64
	Title       string
	Description string
}

// QaseRunFinder lists runs whose title matches a search.
type QaseRunFinder interface {
	SearchRuns(ctx context.Context, title string) ([]QaseRun, error)
}

// Qase adapts the shared Qase client (internal/pkg/qase, QASE_AUTOMATION_TOKEN) to QaseRunFinder.
type Qase struct {
	client *qase.Client
}

// newQase returns a runs finder backed by the shared Qase client; it fails without QASE_AUTOMATION_TOKEN.
func newQase() (*Qase, error) {
	c, err := qase.AddQase()
	if err != nil {
		return nil, err
	}

	return newQaseFrom(c), nil
}

// newQaseFrom wraps an existing shared client (tests point it at a fake Qase server).
func newQaseFrom(c *qase.Client) *Qase {
	return &Qase{client: c}
}

// SearchRuns returns up to 100 runs matching the title search; callers filter for the exact title.
func (q *Qase) SearchRuns(ctx context.Context, title string) ([]QaseRun, error) {
	runs, err := q.client.SearchRuns(ctx, title)
	if err != nil {
		return nil, err
	}

	out := make([]QaseRun, 0, len(runs))
	for i := range runs {
		out = append(out, QaseRun{
			ID:          runs[i].GetId(),
			Title:       runs[i].GetTitle(),
			Description: runs[i].GetDescription(),
		})
	}

	return out, nil
}

// qaseRequestMarker is what scripts/qase-patch-validation.sh appends to the run description
// when the workflow is dispatched with request_id.
func qaseRequestMarker(requestID string) string {
	return " | Release bot request: " + requestID
}

// matchQaseRun picks the run with the exact title created for requestID (its description ends with
// the request marker). With an empty requestID (runs created by hand) the newest run with the title wins.
func matchQaseRun(runs []QaseRun, title, requestID string) (int64, bool) {
	var best int64
	for _, r := range runs {
		if r.Title != title {
			continue
		}
		if requestID != "" && !strings.HasSuffix(r.Description, qaseRequestMarker(requestID)) {
			continue
		}
		if r.ID > best {
			best = r.ID
		}
	}

	return best, best > 0
}

// waitQaseRuns polls until every title has a run for requestID and returns title -> id.
func waitQaseRuns(ctx context.Context, f QaseRunFinder, titles []string, requestID string,
	poll, timeout time.Duration,
) (map[string]int64, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	found := map[string]int64{}
	var lastErr error
	for {
		for _, title := range titles {
			if _, ok := found[title]; ok {
				continue
			}
			runs, err := f.SearchRuns(ctx, title)
			if err != nil {
				lastErr = err
				continue
			}
			if id, ok := matchQaseRun(runs, title, requestID); ok {
				found[title] = id
			}
		}
		if len(found) == len(titles) {
			return found, nil
		}

		select {
		case <-ctx.Done():
			var missing []string
			for _, t := range titles {
				if _, ok := found[t]; !ok {
					missing = append(missing, t)
				}
			}
			// Wrap ctx.Err() so callers can tell cancellation/deadline from an operational failure.
			err := fmt.Errorf("qase runs for request %q not found: %s: %w",
				requestID, strings.Join(missing, "; "), ctx.Err())
			if lastErr != nil {
				err = fmt.Errorf("%w (last error: %w)", err, lastErr)
			}

			return found, err
		case <-time.After(poll):
		}
	}
}

// applyQaseRunIDs replaces {{QASE_RUN_ID}} in every job's params with the run of its product/version;
// it fails if a job needs a run id that was not resolved, so nothing reports to the wrong run.
func applyQaseRunIDs(jobs []JenkinsJob, ids map[string]int64) error {
	for i := range jobs {
		j := &jobs[i]
		for k, v := range j.Params {
			if !strings.Contains(v, qaseRunPlaceholder) {
				continue
			}
			id, ok := ids[j.QaseTitle]
			if !ok {
				return fmt.Errorf("%s %s: no Qase run for %q", j.Path, j.Version, j.QaseTitle)
			}
			j.Params[k] = strings.ReplaceAll(v, qaseRunPlaceholder, strconv.FormatInt(id, 10))
		}
	}

	return nil
}

// newRequestID returns a unique id for one dispatch, e.g. rb-20260928T134501-9f3a1c2e.
func newRequestID(now time.Time) string {
	b := make([]byte, 4)
	_, _ = rand.Read(b)

	return "rb-" + now.UTC().Format("20060102T150405") + "-" + hex.EncodeToString(b)
}
