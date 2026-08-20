package testcase

import (
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func node(name string) *corev1.Node {
	return &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: name}}
}

func pod(namespace, name, nodeName string) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: name},
		Spec:       corev1.PodSpec{NodeName: nodeName},
		Status:     corev1.PodStatus{Phase: corev1.PodRunning},
	}
}

func endpointSlice(namespace, name, nodeName string) *discoveryv1.EndpointSlice {
	return &discoveryv1.EndpointSlice{
		ObjectMeta:  metav1.ObjectMeta{Namespace: namespace, Name: name},
		AddressType: discoveryv1.AddressTypeIPv4,
		Endpoints:   []discoveryv1.Endpoint{{Addresses: []string{"10.42.0.10"}, NodeName: &nodeName}},
	}
}

func TestWorkloadsOnLiveNodesRemovedNodeStillInAPI(t *testing.T) {
	// The deleted Node object lingering in the API must not count as live,
	// or the wait would pass on its first poll with the ghost pod intact.
	cs := fake.NewSimpleClientset(
		node("new-server"),
		node("old-leader"),
		pod("test-clusterip", "test-clusterip-8pgmr", "old-leader"),
	)

	converged, detail, err := workloadsOnLiveNodes(cs, "old-leader")
	if err != nil {
		t.Fatal(err)
	}
	if converged {
		t.Fatal("expected not converged while the removed node object is still in the API")
	}
	if !strings.Contains(detail, "old-leader") {
		t.Fatalf("detail should name the removed node, got: %s", detail)
	}
}

func TestWorkloadsOnLiveNodesPodOnRemovedNode(t *testing.T) {
	cs := fake.NewSimpleClientset(
		node("new-server"),
		pod("test-clusterip", "test-clusterip-8pgmr", "old-leader"),
	)

	converged, detail, err := workloadsOnLiveNodes(cs, "old-leader")
	if err != nil {
		t.Fatal(err)
	}
	if converged {
		t.Fatal("expected not converged while a pod references the removed node")
	}
	if !strings.Contains(detail, "old-leader") {
		t.Fatalf("detail should name the removed node, got: %s", detail)
	}
}

func TestWorkloadsOnLiveNodesEndpointOnRemovedNode(t *testing.T) {
	cs := fake.NewSimpleClientset(
		node("new-server"),
		pod("test-clusterip", "test-clusterip-57srj", "new-server"),
		endpointSlice("test-clusterip", "test-clusterip-abc", "old-leader"),
	)

	converged, detail, err := workloadsOnLiveNodes(cs, "old-leader")
	if err != nil {
		t.Fatal(err)
	}
	if converged {
		t.Fatal("expected not converged while an endpoint targets the removed node")
	}
	if !strings.Contains(detail, "old-leader") {
		t.Fatalf("detail should name the removed node, got: %s", detail)
	}
}

func TestWorkloadsOnLiveNodesGhostNodeWithoutName(t *testing.T) {
	// Even without the removed-node name, workloads on nonexistent nodes block.
	cs := fake.NewSimpleClientset(
		node("new-server"),
		pod("test-clusterip", "test-clusterip-8pgmr", "gone-node"),
	)

	converged, detail, err := workloadsOnLiveNodes(cs, "")
	if err != nil {
		t.Fatal(err)
	}
	if converged {
		t.Fatalf("expected not converged for pod on nonexistent node, detail: %s", detail)
	}
}

func TestWaitWorkloadConvergenceRejectsEmptyNodeName(t *testing.T) {
	err := waitWorkloadConvergence("", time.Second)
	if err == nil {
		t.Fatal("expected error for empty removed node name")
	}
}

func TestFindNodeNameByIP(t *testing.T) {
	leader := node("old-leader")
	leader.Status.Addresses = []corev1.NodeAddress{
		{Type: corev1.NodeInternalIP, Address: "172.31.18.139"},
		{Type: corev1.NodeExternalIP, Address: "3.21.28.248"},
	}
	cs := fake.NewSimpleClientset(leader, node("new-server"))

	name, err := findNodeNameByIP(cs, "3.21.28.248")
	if err != nil || name != "old-leader" {
		t.Fatalf("expected old-leader, got %q err %v", name, err)
	}

	if _, err = findNodeNameByIP(cs, "1.2.3.4"); err == nil {
		t.Fatal("expected error for unknown IP")
	}
}

func TestWorkloadsOnLiveNodesConverged(t *testing.T) {
	cs := fake.NewSimpleClientset(
		node("new-server"),
		pod("test-clusterip", "test-clusterip-57srj", "new-server"),
		pod("kube-system", "pending-pod", ""),
		endpointSlice("test-clusterip", "test-clusterip-abc", "new-server"),
	)

	converged, detail, err := workloadsOnLiveNodes(cs, "old-leader")
	if err != nil {
		t.Fatal(err)
	}
	if !converged {
		t.Fatalf("expected converged, got detail: %s", detail)
	}
}
