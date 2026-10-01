package releasebot

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// TriageFunc runs one triage (the broker's claude-sandbox call, or a fake in tests).
type TriageFunc func(ctx context.Context, req *TriageRequest) (*Verdict, error)

// Broker serves triage requests from Spool one at a time.
type Broker struct {
	Spool  string
	Triage TriageFunc
	Poll   time.Duration
	Logf   func(level, format string, args ...any)
	// KeepVerdicts is how long an uncollected verdict stays in out/ (the bot gave up waiting);
	// default 24h.
	KeepVerdicts time.Duration
}

// Serve claims and answers requests until ctx ends.
func (b *Broker) Serve(ctx context.Context) error {
	if err := verifySpool(b.Spool); err != nil {
		return err
	}
	if err := b.requeue(); err != nil {
		return err
	}
	for ctx.Err() == nil { // a stopping broker claims nothing more; the next one serves what is left
		b.sweep()
		served, err := b.serveOne(ctx)
		if err != nil {
			b.logf("error", "triage broker: %v", err)
		}
		if served {
			continue
		}
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(pollOrDefault(b.Poll)):
		}
	}

	return nil
}

// serveOne answers the oldest request, if any.
func (b *Broker) serveOne(ctx context.Context) (bool, error) {
	id, err := b.claim()
	if id == "" || err != nil {
		return false, err
	}
	work := filepath.Join(b.Spool, spoolWork, id+".json")
	keep := false // a broker stopping mid-triage leaves the request for the next one
	defer func() {
		if !keep {
			_ = os.Remove(work)
		}
	}()

	var req TriageRequest
	raw, err := readSpoolFile(work)
	if err == nil {
		err = json.Unmarshal(raw, &req)
	}
	if err != nil || req.ID != id {
		return true, writeSpoolFile(filepath.Join(b.Spool, spoolOut), id, &Verdict{Error: "unreadable request"})
	}

	b.logf("info", "triaging %s %s (%s)", req.JobName, req.Version, req.BuildURL)
	// The bot leaves in/<id>.cancel when it stops waiting (stop, shutdown, timeout).
	marker := filepath.Join(b.Spool, spoolIn, id+cancelSuffix)
	tctx, cancel := context.WithCancel(ctx)
	defer cancel()
	defer os.Remove(marker)
	go b.watchCancel(tctx, marker, cancel)
	v, err := b.Triage(tctx, &req)
	if ctx.Err() != nil {
		if _, markErr := os.Lstat(marker); markErr != nil {
			keep = true // the broker is stopping, the bot still waits: requeued on the next start
			return true, nil
		}
	}
	if err != nil {
		v = &Verdict{Error: err.Error()}
	}
	b.logf("info", "verdict for %s %s (%s): %s %s ($%.2f, %ds)", req.JobName, req.Version, req.Mode, v.Action,
		redactSecrets(v.Error), v.CostUSD, v.Seconds)

	return true, writeSpoolFile(filepath.Join(b.Spool, spoolOut), id, v)
}

// claim moves the oldest request to work/ and returns its id ("" when there is none).
func (b *Broker) claim() (string, error) {
	entries, err := os.ReadDir(filepath.Join(b.Spool, spoolIn))
	if err != nil {
		return "", err
	}
	type pending struct {
		id  string
		mod time.Time
	}
	var reqs []pending
	for _, e := range entries {
		name := e.Name()
		if strings.HasPrefix(name, ".") || !strings.HasSuffix(name, ".json") || !e.Type().IsRegular() {
			continue
		}
		info, infoErr := e.Info()
		if infoErr != nil {
			if errors.Is(infoErr, os.ErrNotExist) {
				continue
			}
			return "", fmt.Errorf("inspect queued request %s: %w", name, infoErr)
		}
		reqs = append(reqs, pending{strings.TrimSuffix(name, ".json"), info.ModTime()})
	}
	sort.Slice(reqs, func(i, j int) bool { return reqs[i].mod.Before(reqs[j].mod) })
	for _, r := range reqs {
		from := filepath.Join(b.Spool, spoolIn, r.id+".json")
		moved, moveErr := moveRequest(from, filepath.Join(b.Spool, spoolWork, r.id+".json"))
		if moveErr != nil {
			return "", moveErr
		}
		if moved {
			return r.id, nil
		}
	}

	return "", nil
}

// requeue returns requests a previous broker claimed but never answered (it crashed or was
// stopped mid-triage) to in/; only one broker holds the spool lock, so none is in progress.
func (b *Broker) requeue() error {
	dir := filepath.Join(b.Spool, spoolWork)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("read unfinished triage requests: %w", err)
	}
	var problems []error
	for _, e := range entries {
		if !e.Type().IsRegular() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		moved, moveErr := moveRequest(filepath.Join(dir, e.Name()), filepath.Join(b.Spool, spoolIn, e.Name()))
		if moveErr != nil {
			problems = append(problems, moveErr)
		} else if moved {
			b.logf("info", "requeued %s, left by a previous broker", e.Name())
		}
	}

	return errors.Join(problems...)
}

// A withdrawn request is harmless; a missing destination or other filesystem error is not.
func moveRequest(from, to string) (bool, error) {
	err := os.Rename(from, to)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		if _, sourceErr := os.Lstat(from); errors.Is(sourceErr, os.ErrNotExist) {
			return false, nil
		}
	}

	return false, fmt.Errorf("move triage request %s: %w", filepath.Base(from), err)
}

// watchCancel cancels the triage in progress once the bot's cancel marker appears.
func (b *Broker) watchCancel(ctx context.Context, marker string, cancel context.CancelFunc) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(pollOrDefault(b.Poll)):
		}
		if info, err := os.Lstat(marker); err == nil && info.Mode().IsRegular() {
			b.logf("info", "the bot canceled %s; stopping its triage", filepath.Base(marker))
			cancel()

			return
		}
	}
}

// sweep removes verdicts nobody collected (the bot stopped waiting) and stale cancel markers.
func (b *Broker) sweep() {
	keep := b.KeepVerdicts
	if keep <= 0 {
		keep = 24 * time.Hour
	}
	sweepDir(filepath.Join(b.Spool, spoolOut), "", keep)
	sweepDir(filepath.Join(b.Spool, spoolIn), cancelSuffix, keep)
}

// sweepDir removes files older than keep (only names ending in suffix, when set).
func sweepDir(dir, suffix string, keep time.Duration) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if suffix != "" && !strings.HasSuffix(e.Name(), suffix) {
			continue
		}
		if info, infoErr := e.Info(); infoErr == nil && time.Since(info.ModTime()) > keep {
			_ = os.Remove(filepath.Join(dir, e.Name()))
		}
	}
}

func (b *Broker) logf(level, format string, args ...any) {
	if b.Logf != nil {
		b.Logf(level, format, args...)
	}
}
