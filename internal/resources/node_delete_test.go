package resources

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func fakeRunner(getOutputs []string, deletes *int) hostArgsRunner {
	i := 0

	return func(_ string, args ...string) (string, error) {
		if args[0] == "delete" {
			*deletes++

			return "deleted", nil
		}
		if i >= len(getOutputs) {
			return "", nil
		}
		out := getOutputs[i]
		i++
		if out == "ERR" {
			return "", errors.New("api unavailable")
		}

		return out, nil
	}
}

func TestDeleteNodeUntilGoneSucceeds(t *testing.T) {
	deletes := 0
	run := fakeRunner([]string{"node/n1", "", "", ""}, &deletes)
	if err := deleteNodeUntilGone("n1", run, time.Second, time.Millisecond, 3); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if deletes != 2 {
		t.Errorf("expected 2 deletes (initial + retry once absent reset), got %d", deletes)
	}
}

func TestDeleteNodeUntilGoneRedeletesWhenNodeReregisters(t *testing.T) {
	deletes := 0
	run := fakeRunner([]string{"", "node/n1", "", "", ""}, &deletes)
	if err := deleteNodeUntilGone("n1", run, time.Second, time.Millisecond, 3); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if deletes < 2 {
		t.Errorf("expected a re-delete after re-registration, got %d deletes", deletes)
	}
}

func TestDeleteNodeUntilGoneToleratesGetErrors(t *testing.T) {
	deletes := 0
	run := fakeRunner([]string{"ERR", "", "", ""}, &deletes)
	if err := deleteNodeUntilGone("n1", run, time.Second, time.Millisecond, 3); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDeleteNodeUntilGoneTimesOut(t *testing.T) {
	deletes := 0
	always := func(_ string, args ...string) (string, error) {
		if args[0] == "delete" {
			deletes++

			return "", nil
		}

		return "node/n1", nil
	}
	err := deleteNodeUntilGone("n1", always, 30*time.Millisecond, 5*time.Millisecond, 3)
	if err == nil || !strings.Contains(err.Error(), "not removed") {
		t.Fatalf("expected timeout error, got %v", err)
	}
	if deletes < 2 {
		t.Errorf("expected repeated deletes while node persists, got %d", deletes)
	}
}

func TestDeleteNodeUntilGoneRejectsEmptyName(t *testing.T) {
	if err := deleteNodeUntilGone("", nil, time.Second, time.Millisecond, 3); err == nil {
		t.Fatal("expected error for empty name")
	}
}
