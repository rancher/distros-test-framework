package testcase

import (
	"errors"
	"reflect"
	"slices"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

type removalRecorder struct {
	calls []string
	r     nodeRemoval
	// recheckID/recheckErr are what the ip lookup returns after stopping, before termination.
	recheckID  string
	recheckErr error
	// liveAfter/liveAfterErr are what the ip lookup returns after termination.
	liveAfter    string
	liveAfterErr error
}

// currentID is the instance now live on the ip before removal ("" when none).
func newRecorder(name, currentID string) *removalRecorder {
	rec := &removalRecorder{recheckID: currentID}
	lookups := 0
	rec.r = nodeRemoval{
		checkAPIReachable: func(string) error { rec.calls = append(rec.calls, "check"); return nil },
		resolveName: func(string) (string, error) {
			if name == "" {
				return "", errNodeNotFound
			}

			return name, nil
		},
		resolveInstanceID: func(string) (string, error) {
			lookups++
			if lookups == 1 {
				return currentID, nil
			}
			if !slices.Contains(rec.calls, "terminate") {
				return rec.recheckID, rec.recheckErr
			}

			return rec.liveAfter, rec.liveAfterErr
		},
		stopServices: func(string) error { rec.calls = append(rec.calls, "stop"); return nil },
		deleteNode: func(_ string, mayDelete func() bool) error {
			if mayDelete == nil || mayDelete() {
				rec.calls = append(rec.calls, "delete")
			}

			return nil
		},
		terminate: func(string) error { rec.calls = append(rec.calls, "terminate"); return nil },
	}

	return rec
}

func TestRemoveNodeOrdersStopDeleteTerminateThenVerify(t *testing.T) {
	rec := newRecorder("n1", "i-1")
	if err := rec.r.run("1.2.3.4", "i-1"); err != nil {
		t.Fatal(err)
	}
	want := []string{"check", "stop", "delete", "terminate", "delete"}
	if !reflect.DeepEqual(rec.calls, want) {
		t.Fatalf("calls = %v, want %v", rec.calls, want)
	}
}

// Rerun after the instance was terminated but the Node delete failed.
func TestRemoveNodeRetryAfterTerminationStillDeletesNode(t *testing.T) {
	rec := newRecorder("n1", "")
	if err := rec.r.run("1.2.3.4", "i-1"); err != nil {
		t.Fatal(err)
	}
	want := []string{"check", "terminate", "delete"}
	if !reflect.DeepEqual(rec.calls, want) {
		t.Fatalf("calls = %v, want %v", rec.calls, want)
	}
}

// Rerun after the Node is gone but the instance is still running.
func TestRemoveNodeRetryAfterNodeDeletedStillTerminates(t *testing.T) {
	rec := newRecorder("", "i-1")
	if err := rec.r.run("1.2.3.4", "i-1"); err != nil {
		t.Fatal(err)
	}
	want := []string{"check", "stop", "terminate"}
	if !reflect.DeepEqual(rec.calls, want) {
		t.Fatalf("calls = %v, want %v", rec.calls, want)
	}
}

// AWS reassigned the ip to an unrelated live instance: never stop or terminate it.
func TestRemoveNodeRecycledIPLeavesOtherInstanceAlone(t *testing.T) {
	rec := newRecorder("n1", "i-OTHER")
	var terminated string
	rec.r.terminate = func(id string) error { terminated = id; return nil }
	if err := rec.r.run("1.2.3.4", "i-1"); err != nil {
		t.Fatal(err)
	}
	for _, c := range rec.calls {
		if c == "stop" || c == "delete" {
			t.Fatalf("must not %s on a recycled ip", c)
		}
	}
	if terminated != "i-1" {
		t.Fatalf("terminated %q, want only the recorded i-1", terminated)
	}
}

// A replacement can take over the ip (and Node name) once the old instance is terminated.
func TestRemoveNodeSkipsFinalDeleteWhenReplacementHoldsIP(t *testing.T) {
	rec := newRecorder("n1", "i-1")
	rec.liveAfter = "i-NEW"
	if err := rec.r.run("1.2.3.4", "i-1"); err != nil {
		t.Fatal(err)
	}
	want := []string{"check", "stop", "delete", "terminate"}
	if !reflect.DeepEqual(rec.calls, want) {
		t.Fatalf("calls = %v, want %v", rec.calls, want)
	}
}

func TestRemoveNodeSkipsFinalDeleteWhenOwnershipIndeterminate(t *testing.T) {
	rec := newRecorder("n1", "i-1")
	rec.liveAfterErr = errors.New("describe throttled")
	if err := rec.r.run("1.2.3.4", "i-1"); err != nil {
		t.Fatal(err)
	}
	if got := rec.calls[len(rec.calls)-1]; got != "terminate" {
		t.Fatalf("final call = %s, want terminate (no final delete)", got)
	}
}

// The ip can be recycled during the (slow) service stop, so ownership is re-verified before the Node delete.
func TestRemoveNodeSkipsNodeDeleteWhenOwnerChangesDuringStop(t *testing.T) {
	for name, tc := range map[string]struct {
		id        string
		err       error
		finalWant string
	}{
		"other instance": {id: "i-NEW"},
		"no owner":       {id: "", finalWant: "delete"},
		"lookup error":   {err: errors.New("describe throttled"), finalWant: "delete"},
	} {
		t.Run(name, func(t *testing.T) {
			rec := newRecorder("n1", "i-1")
			rec.recheckID, rec.recheckErr = tc.id, tc.err
			rec.liveAfter = tc.id // a replacement that took the ip is still live afterwards
			if err := rec.r.run("1.2.3.4", "i-1"); err != nil {
				t.Fatal(err)
			}
			want := []string{"check", "stop", "terminate"}
			if tc.finalWant != "" {
				want = append(want, tc.finalWant)
			}
			if !reflect.DeepEqual(rec.calls, want) {
				t.Fatalf("calls = %v, want %v", rec.calls, want)
			}
		})
	}
}

func TestRemoveNodeRefusesWithoutRecordedID(t *testing.T) {
	rec := newRecorder("n1", "i-1")
	if err := rec.r.run("1.2.3.4", ""); err == nil || len(rec.calls) != 0 {
		t.Fatalf("expected refusal before any action, calls=%v err=%v", rec.calls, err)
	}
}

func TestRemoveNodeNodeDeleteFailureKeepsInstance(t *testing.T) {
	rec := newRecorder("n1", "i-1")
	rec.r.deleteNode = func(string, func() bool) error { return errors.New("api down") }
	if err := rec.r.run("1.2.3.4", "i-1"); err == nil {
		t.Fatal("expected error")
	}
	for _, c := range rec.calls {
		if c == "terminate" {
			t.Fatal("must not terminate before the Node is deleted")
		}
	}
}

func TestRemoveNodeStopFailureIsNonFatal(t *testing.T) {
	rec := newRecorder("n1", "i-1")
	rec.r.stopServices = func(string) error { return errors.New("ssh refused") }
	if err := rec.r.run("1.2.3.4", "i-1"); err != nil {
		t.Fatalf("stop failure should not abort: %v", err)
	}
}

func TestRemoveNodeResolveFailuresAbort(t *testing.T) {
	rec := newRecorder("n1", "i-1")
	rec.r.resolveName = func(string) (string, error) { return "", errors.New("api down") }
	if err := rec.r.run("1.2.3.4", "i-1"); err == nil {
		t.Error("name resolve error must abort")
	}

	rec = newRecorder("n1", "i-1")
	rec.r.resolveInstanceID = func(string) (string, error) { return "", errors.New("ambiguous") }
	if err := rec.r.run("1.2.3.4", "i-1"); err == nil {
		t.Error("ambiguous instance must abort")
	}

	rec = newRecorder("n1", "i-1")
	rec.r.checkAPIReachable = func(string) error { return errors.New("targets node") }
	if err := rec.r.run("1.2.3.4", "i-1"); err == nil || len(rec.calls) != 0 {
		t.Errorf("api guard must abort before any action, calls=%v err=%v", rec.calls, err)
	}
}

func TestOldServerToRemoveNeverPicksLeaderAndCoversAll(t *testing.T) {
	for n := 1; n <= 5; n++ {
		old := make([]string, n)
		for i := range old {
			old[i] = string(rune('a' + i))
		}
		var got []string
		for i := 0; ; i++ {
			ip, ok := oldServerToRemove(old, i)
			if !ok {
				break
			}
			got = append(got, ip)
		}
		if !reflect.DeepEqual(got, old[1:]) && !(n == 1 && len(got) == 0) {
			t.Errorf("n=%d got %v, want %v", n, got, old[1:])
		}
	}
	// Three servers must keep the originally tested order: second then third.
	if ip, _ := oldServerToRemove([]string{"s0", "s1", "s2"}, 0); ip != "s1" {
		t.Errorf("first removal = %s, want s1", ip)
	}
	if ip, _ := oldServerToRemove([]string{"s0", "s1", "s2"}, 1); ip != "s2" {
		t.Errorf("second removal = %s, want s2", ip)
	}
}

func TestKubeconfigTargetsNode(t *testing.T) {
	tests := []struct {
		name, server, ip string
		want             bool
		wantErr          bool
	}{
		{"same ipv4", "https://1.2.3.4:6443", "1.2.3.4", true, false},
		{"prefix ipv4 is different", "https://1.2.3.4:6443", "1.2.3.40", false, false},
		{"fqdn is not the node", "https://cluster.example.com:6443", "1.2.3.4", false, false},
		{"same ipv6", "https://[2600::1]:6443", "2600::1", true, false},
		{"expanded vs compressed ipv6", "https://[2600:0:0:0:0:0:0:1]:6443", "2600::1", true, false},
		{"different ipv6", "https://[2600::2]:6443", "2600::1", false, false},
		{"unparsable endpoint fails closed", "https://[::1", "1.2.3.4", false, true},
		{"empty endpoint fails closed", "", "1.2.3.4", false, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := kubeconfigTargetsNode(tc.server, tc.ip)
			if (err != nil) != tc.wantErr || got != tc.want {
				t.Errorf("got (%v, %v), want (%v, err=%v)", got, err, tc.want, tc.wantErr)
			}
		})
	}
}

func TestFindNodeNameByIPNotFoundIsSentinel(t *testing.T) {
	cs := fake.NewSimpleClientset(&corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "n1"},
		Status: corev1.NodeStatus{Addresses: []corev1.NodeAddress{
			{Type: corev1.NodeInternalIP, Address: "10.0.0.1"},
		}},
	})
	if name, err := findNodeNameByIP(cs, "10.0.0.1"); err != nil || name != "n1" {
		t.Fatalf("got %q, %v", name, err)
	}
	if _, err := findNodeNameByIP(cs, "9.9.9.9"); !errors.Is(err, errNodeNotFound) {
		t.Fatalf("want errNodeNotFound, got %v", err)
	}
}
