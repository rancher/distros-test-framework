package testcase

import (
	"errors"
	"reflect"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

type removalRecorder struct {
	calls []string
	r     nodeRemoval
}

func newRecorder(name, instanceID string) *removalRecorder {
	rec := &removalRecorder{}
	rec.r = nodeRemoval{
		checkAPIReachable: func(string) error { rec.calls = append(rec.calls, "check"); return nil },
		resolveName: func(string) (string, error) {
			if name == "" {
				return "", errNodeNotFound
			}

			return name, nil
		},
		resolveInstanceID: func(string) (string, error) { return instanceID, nil },
		stopServices:      func(string) error { rec.calls = append(rec.calls, "stop"); return nil },
		deleteNode:        func(string) error { rec.calls = append(rec.calls, "delete"); return nil },
		terminate:         func(string) error { rec.calls = append(rec.calls, "terminate"); return nil },
	}

	return rec
}

func TestRemoveNodeOrdersStopDeleteTerminateThenVerify(t *testing.T) {
	rec := newRecorder("n1", "i-1")
	if err := rec.r.run("1.2.3.4"); err != nil {
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
	if err := rec.r.run("1.2.3.4"); err != nil {
		t.Fatal(err)
	}
	want := []string{"check", "delete", "delete"}
	if !reflect.DeepEqual(rec.calls, want) {
		t.Fatalf("calls = %v, want %v", rec.calls, want)
	}
}

// Rerun after the Node is gone but the instance is still running.
func TestRemoveNodeRetryAfterNodeDeletedStillTerminates(t *testing.T) {
	rec := newRecorder("", "i-1")
	if err := rec.r.run("1.2.3.4"); err != nil {
		t.Fatal(err)
	}
	want := []string{"check", "stop", "terminate"}
	if !reflect.DeepEqual(rec.calls, want) {
		t.Fatalf("calls = %v, want %v", rec.calls, want)
	}
}

func TestRemoveNodeNodeDeleteFailureKeepsInstance(t *testing.T) {
	rec := newRecorder("n1", "i-1")
	rec.r.deleteNode = func(string) error { return errors.New("api down") }
	if err := rec.r.run("1.2.3.4"); err == nil {
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
	if err := rec.r.run("1.2.3.4"); err != nil {
		t.Fatalf("stop failure should not abort: %v", err)
	}
}

func TestRemoveNodeResolveFailuresAbort(t *testing.T) {
	rec := newRecorder("n1", "i-1")
	rec.r.resolveName = func(string) (string, error) { return "", errors.New("api down") }
	if err := rec.r.run("1.2.3.4"); err == nil {
		t.Error("name resolve error must abort")
	}

	rec = newRecorder("n1", "i-1")
	rec.r.resolveInstanceID = func(string) (string, error) { return "", errors.New("ambiguous") }
	if err := rec.r.run("1.2.3.4"); err == nil {
		t.Error("ambiguous instance must abort")
	}

	rec = newRecorder("n1", "i-1")
	rec.r.checkAPIReachable = func(string) error { return errors.New("targets node") }
	if err := rec.r.run("1.2.3.4"); err == nil || len(rec.calls) != 0 {
		t.Errorf("api guard must abort before any action, calls=%v err=%v", rec.calls, err)
	}
}

func TestKubeconfigTargetsNode(t *testing.T) {
	tests := []struct {
		server, ip string
		want       bool
	}{
		{"https://1.2.3.4:6443", "1.2.3.4", true},
		{"https://1.2.3.4:6443", "1.2.3.40", false},
		{"https://cluster.example.com:6443", "1.2.3.4", false},
		{"https://[2600::1]:6443", "2600::1", true},
		{"", "1.2.3.4", false},
	}
	for _, tc := range tests {
		if got := kubeconfigTargetsNode(tc.server, tc.ip); got != tc.want {
			t.Errorf("kubeconfigTargetsNode(%q,%q)=%v want %v", tc.server, tc.ip, got, tc.want)
		}
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
