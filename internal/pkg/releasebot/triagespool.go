package releasebot

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

const (
	spoolIn   = "in"
	spoolWork = "work"
	spoolOut  = "out"
	// spoolMode lets both the bot and the broker (one shared group) read and write the files.
	spoolMode = 0o660
)

// BrokerClient asks the triage broker through a spool directory.
type BrokerClient struct {
	Dir     string
	Timeout time.Duration // how long a request waits for its verdict
	Poll    time.Duration
}

// Ask returns the broker's decision; ok is false when triage could not run (no broker, timeout,
// bad answer), and the decision then asks a person.
func (c *BrokerClient) Ask(ctx context.Context, req *TriageRequest) (d Decision, ok bool) {
	if !brokerRunning(c.Dir) {
		return Decision{Summary: "automatic triage unavailable: the triage broker is not running"}, false
	}
	req.ID = newID()
	v, err := askBroker(ctx, c.Dir, req, c.Timeout, pollOrDefault(c.Poll))
	if err != nil {
		return Decision{Summary: "automatic triage unavailable: " + err.Error()}, false
	}

	return v.Decision(), v.Error == ""
}

// pollOrDefault keeps a zero Poll from turning a wait loop into a busy loop.
func pollOrDefault(p time.Duration) time.Duration {
	if p <= 0 {
		return 5 * time.Second
	}

	return p
}

func askBroker(ctx context.Context, dir string, req *TriageRequest, timeout, poll time.Duration) (*Verdict, error) {
	if err := writeSpoolFile(filepath.Join(dir, spoolIn), req.ID, req); err != nil {
		return nil, fmt.Errorf("queue request: %w", err)
	}
	out := filepath.Join(dir, spoolOut, req.ID+".json")
	defer os.Remove(out)

	parent := ctx
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	for {
		raw, err := readSpoolFile(out)
		if err == nil {
			// Checked again here: only a verdict that fully matches the contract may rerun.
			v, parseErr := decodeVerdict(raw)
			if parseErr != nil {
				return nil, fmt.Errorf("bad verdict: %w", parseErr)
			}

			return v, nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
		select {
		case <-ctx.Done():
			// Withdraw the request; once the broker has claimed it, ask it to stop that triage.
			if err := os.Remove(filepath.Join(dir, spoolIn, req.ID+".json")); errors.Is(err, os.ErrNotExist) {
				_ = writeSpoolRaw(filepath.Join(dir, spoolIn), req.ID+cancelSuffix, nil)
			}
			if parent.Err() != nil {
				return nil, fmt.Errorf("triage canceled: %w", parent.Err())
			}

			return nil, fmt.Errorf("no verdict within %s", timeout)
		case <-time.After(poll):
		}
	}
}

// writeSpoolFile writes v as <dir>/<id>.json atomically for the group; the temp file is created
// exclusively under a random name, so a planted symlink is replaced by the rename, never followed.
func writeSpoolFile(dir, id string, v any) error {
	raw, err := json.Marshal(v)
	if err != nil {
		return err
	}

	return writeSpoolRaw(dir, id+".json", raw)
}

// cancelSuffix names the marker the bot leaves in in/ to stop a triage the broker has claimed.
const cancelSuffix = ".cancel"

func writeSpoolRaw(dir, name string, raw []byte) error {
	f, err := os.CreateTemp(dir, ".spool-*.tmp")
	if err != nil {
		return err
	}
	tmp := f.Name()
	// The bot runs with UMask=0077; the broker (another user in the group) must still read it.
	if err = f.Chmod(spoolMode); err == nil {
		_, err = f.Write(raw)
	}
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		_ = os.Remove(tmp)
		return err
	}

	return os.Rename(tmp, filepath.Join(dir, name))
}

// readSpoolFile reads a regular spool file without following a symlink.
func readSpoolFile(path string) ([]byte, error) {
	f, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	if info, statErr := f.Stat(); statErr != nil || !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%s is not a regular file", filepath.Base(path))
	}

	return io.ReadAll(io.LimitReader(f, 1<<20))
}

// BrokerLock is the broker's instance lock in the spool; group-readable, so the bot can see
// whether a broker holds it.
const BrokerLock = ".broker.lock"

// brokerRunning reports whether a broker holds the spool's lock, without waiting.
func brokerRunning(dir string) bool {
	f, err := os.OpenFile(filepath.Join(dir, BrokerLock), os.O_RDONLY|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return false
	}
	defer f.Close()
	if err = syscall.Flock(int(f.Fd()), syscall.LOCK_SH|syscall.LOCK_NB); err != nil {
		return errors.Is(err, syscall.EWOULDBLOCK)
	}
	_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)

	return false
}

func newID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)

	return hex.EncodeToString(b)
}
