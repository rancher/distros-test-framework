package testcase

import "testing"

func TestSettleTrackerRequiresStableStreak(t *testing.T) {
	tr := &settleTracker{need: 3}
	base := []string{"kube-system/coredns", "kube-system/traefik"}

	if tr.observe(true, base) {
		t.Fatal("first healthy poll must not count as stable")
	}
	if tr.observe(true, base) {
		t.Fatal("second healthy poll must not count as stable")
	}
	// a late addon appears: the pod set changed, the streak restarts even though everything is healthy
	withAddon := append(append([]string{}, base...), "kube-system/metrics-server")
	if tr.observe(true, withAddon) {
		t.Fatal("pod set change must reset the streak")
	}
	if tr.observe(false, withAddon) {
		t.Fatal("an unsettled poll must reset the streak")
	}
	for i := 1; i <= 2; i++ {
		if tr.observe(true, withAddon) {
			t.Fatalf("streak of %d is not enough", i)
		}
	}
	if !tr.observe(true, withAddon) {
		t.Fatal("three consecutive healthy polls with the same pod set must be stable")
	}
}

func TestSettleTrackerOrderInsensitive(t *testing.T) {
	tr := &settleTracker{need: 2}
	tr.observe(true, []string{"a", "b"})
	if !tr.observe(true, []string{"b", "a"}) {
		t.Fatal("pod order must not count as a set change")
	}
}

func TestSettleTrackerFailedPollResets(t *testing.T) {
	tr := &settleTracker{need: 3}
	pods := []string{"kube-system/coredns"}
	tr.observe(true, pods)
	tr.observe(true, pods)
	// healthy, healthy, NotReady (reported as unhealthy with no keys), healthy must not be stable
	tr.observe(false, nil)
	if tr.observe(true, pods) {
		t.Fatal("a failed poll in the middle must reset the streak")
	}
}
