package releasebot

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestMoveRequestDistinguishesWithdrawalFromFailure(t *testing.T) {
	for _, present := range []bool{false, true} {
		t.Run(map[bool]string{false: "withdrawn", true: "destination missing"}[present], func(t *testing.T) {
			dir := t.TempDir()
			from := filepath.Join(dir, "request.json")
			if present {
				if err := os.WriteFile(from, []byte("request"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			moved, err := moveRequest(from, filepath.Join(dir, "missing", "request.json"))
			if moved || (err != nil) != present {
				t.Fatalf("moved=%v err=%v, source present=%v", moved, err, present)
			}
			if present {
				if _, err = os.Stat(from); err != nil {
					t.Fatalf("request lost after failed move: %v", err)
				}
			}
		})
	}
}

func obstructRequest(t *testing.T, spool, dir, id string) {
	t.Helper()
	path := filepath.Join(spool, dir, id+".json")
	if err := os.Mkdir(path, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(path, "keep"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestBrokerClaimReportsMoveFailure(t *testing.T) {
	spool := newSpool(t)
	if err := writeSpoolFile(filepath.Join(spool, spoolIn), "blocked", quickRequest()); err != nil {
		t.Fatal(err)
	}
	obstructRequest(t, spool, spoolWork, "blocked")
	id, err := (&Broker{Spool: spool}).claim()
	if err == nil || id != "" {
		t.Fatalf("failed claim reported as empty queue: id=%q err=%v", id, err)
	}
	if _, err = os.Stat(filepath.Join(spool, spoolIn, "blocked.json")); err != nil {
		t.Fatalf("queued request lost: %v", err)
	}
}

func TestBrokerRecoveryReportsFailureAndRecoversOtherRequests(t *testing.T) {
	spool := newSpool(t)
	for _, id := range []string{"blocked", "healthy"} {
		if err := writeSpoolFile(filepath.Join(spool, spoolWork), id, quickRequest()); err != nil {
			t.Fatal(err)
		}
	}
	obstructRequest(t, spool, spoolIn, "blocked")
	b := &Broker{Spool: spool}
	if err := b.requeue(); err == nil {
		t.Fatal("recovery silently ignored the blocked move")
	}
	if _, err := os.Stat(filepath.Join(spool, spoolWork, "blocked.json")); err != nil {
		t.Fatalf("unfinished request lost: %v", err)
	}
	if _, err := os.Stat(filepath.Join(spool, spoolIn, "healthy.json")); err != nil {
		t.Fatalf("healthy request not recovered: %v", err)
	}
	if err := b.Serve(testContext(t)); err == nil || errors.Is(err, os.ErrNotExist) {
		t.Fatalf("startup did not report the failed recovery: %v", err)
	}
}
