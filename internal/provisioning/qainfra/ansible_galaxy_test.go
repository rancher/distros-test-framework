package qainfra

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func fakeGalaxy(t *testing.T, failures int) *[]string {
	t.Helper()
	var calls []string
	origRun, origDelay := galaxyRun, galaxyRetryDelay
	t.Cleanup(func() { galaxyRun, galaxyRetryDelay = origRun, origDelay })
	galaxyRetryDelay = time.Millisecond
	galaxyRun = func(_ string, _ time.Duration, name string, args ...string) error {
		calls = append(calls, name+" "+strings.Join(args, " "))
		if len(calls) <= failures {
			return errors.New("urlopen error [Errno 104] Connection reset by peer")
		}

		return nil
	}

	return &calls
}

func TestInstallAnsibleCollectionRetriesTransientFailures(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "requirements.yml"), []byte("collections: []"), 0o600); err != nil {
		t.Fatal(err)
	}
	calls := fakeGalaxy(t, 2)

	if err := installAnsibleCollection(dir); err != nil {
		t.Fatalf("expected success on the third attempt, got %v", err)
	}
	if len(*calls) != 3 || (*calls)[2] != "ansible-galaxy collection install -r requirements.yml --force" {
		t.Fatalf("calls = %q", *calls)
	}
}

func TestInstallAnsibleCollectionGivesUpAfterAttempts(t *testing.T) {
	calls := fakeGalaxy(t, 99)

	err := installAnsibleCollection(t.TempDir()) // no requirements.yml: cloud.terraform path
	if err == nil || !strings.Contains(err.Error(), "Connection reset") {
		t.Fatalf("expected the last galaxy error, got %v", err)
	}
	if len(*calls) != int(galaxyAttempts) || (*calls)[0] != "ansible-galaxy collection install cloud.terraform --force" {
		t.Fatalf("calls = %q", *calls)
	}
}
