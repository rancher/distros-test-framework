package resources

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func fakeRunner(getOutputs []string, deletes *int) hostArgsRunner {
	i := 0

	return func(_ context.Context, _ string, args ...string) (string, error) {
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
	if err := deleteNodeUntilGone("n1", run, time.Second, time.Second, time.Millisecond, 3, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// First poll still sees node/n1, so the loop deletes a second time before 3 clean polls.
	if deletes != 2 {
		t.Errorf("expected 2 deletes, got %d", deletes)
	}
}

func TestDeleteNodeUntilGoneRedeletesWhenNodeReregisters(t *testing.T) {
	deletes := 0
	run := fakeRunner([]string{"", "node/n1", "", "", ""}, &deletes)
	if err := deleteNodeUntilGone("n1", run, time.Second, time.Second, time.Millisecond, 3, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if deletes < 2 {
		t.Errorf("expected a re-delete after re-registration, got %d deletes", deletes)
	}
}

func TestDeleteNodeUntilGoneToleratesGetErrors(t *testing.T) {
	deletes := 0
	run := fakeRunner([]string{"ERR", "", "", ""}, &deletes)
	if err := deleteNodeUntilGone("n1", run, time.Second, time.Second, time.Millisecond, 3, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDeleteNodeUntilGoneTimesOut(t *testing.T) {
	deletes := 0
	always := func(_ context.Context, _ string, args ...string) (string, error) {
		if args[0] == "delete" {
			deletes++

			return "", nil
		}

		return "node/n1", nil
	}
	err := deleteNodeUntilGone("n1", always, 30*time.Millisecond, 30*time.Millisecond, 5*time.Millisecond, 3, nil)
	if err == nil || !strings.Contains(err.Error(), "not removed") {
		t.Fatalf("expected timeout error, got %v", err)
	}
	if deletes < 2 {
		t.Errorf("expected repeated deletes while node persists, got %d", deletes)
	}
}

func TestDeleteNodeUntilGoneRejectsEmptyName(t *testing.T) {
	if err := deleteNodeUntilGone("", nil, time.Second, time.Second, time.Millisecond, 3, nil); err == nil {
		t.Fatal("expected error for empty name")
	}
}

func TestDeleteNodeUntilGoneBoundsHungKubectl(t *testing.T) {
	hung := func(ctx context.Context, _ string, _ ...string) (string, error) {
		<-ctx.Done()

		return "", ctx.Err()
	}

	start := time.Now()
	err := deleteNodeUntilGone("n1", hung, 300*time.Millisecond, 100*time.Millisecond, 10*time.Millisecond, 3, nil)
	if err == nil || !strings.Contains(err.Error(), "not removed") {
		t.Fatalf("expected timeout error, got %v", err)
	}
	if elapsed := time.Since(start); elapsed > 3*time.Second {
		t.Fatalf("hung kubectl was not bounded, took %s", elapsed)
	}
}

func TestDeleteNodeUntilGoneGuardChecksEveryDeleteAndStopsWhenDenied(t *testing.T) {
	deletes, checks := 0, 0
	// The node keeps re-registering; ownership is lost before the second delete.
	run := fakeRunner([]string{"node/n1", "node/n1", "node/n1"}, &deletes)
	guard := func() bool {
		checks++

		return checks < 2
	}
	if err := deleteNodeUntilGone("n1", run, time.Second, time.Second, time.Millisecond, 3, guard); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if deletes != 1 || checks != 2 {
		t.Errorf("deletes=%d checks=%d, want 1 delete and a guard check before each of 2 attempts", deletes, checks)
	}
}

func TestDeleteNodeUntilGoneGuardDeniedNeverDeletes(t *testing.T) {
	deletes := 0
	run := fakeRunner(nil, &deletes)
	deny := func() bool { return false }
	if err := deleteNodeUntilGone("n1", run, time.Second, time.Second, time.Millisecond, 3, deny); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if deletes != 0 {
		t.Errorf("deletes=%d, want 0", deletes)
	}
}
