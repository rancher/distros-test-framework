package resources

import "testing"

func TestPodsSettled(t *testing.T) {
	pod := func(ns, name, ready, status string) Pod {
		return Pod{NameSpace: ns, Name: name, Ready: ready, Status: status}
	}
	cases := []struct {
		name string
		pods []Pod
		want bool
	}{
		{"all running and completed", []Pod{
			pod("kube-system", "coredns", "1/1", "Running"),
			pod("kube-system", "helm-install-traefik", "0/1", "Completed"),
			pod("kube-system", "svclb", "2/2", "Running"),
		}, true},
		{"running but not all containers ready", []Pod{pod("kube-system", "metrics-server", "0/1", "Running")}, false},
		{
			"still creating (image pull in flight)",
			[]Pod{pod("kube-system", "metrics-server", "0/1", "ContainerCreating")},
			false,
		},
		{"pending", []Pod{pod("kube-system", "traefik", "0/1", "Pending")}, false},
		{"crashloop", []Pod{pod("kube-system", "coredns", "0/1", "CrashLoopBackOff")}, false},
		{"succeeded phase", []Pod{pod("kube-system", "job", "0/1", "Succeeded")}, true},
		{"no pods is not settled", nil, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, pending := PodsSettled(tc.pods)
			if got != tc.want {
				t.Fatalf("got %v (pending %v), want %v", got, pending, tc.want)
			}
			if !got && len(tc.pods) > 0 && len(pending) == 0 {
				t.Fatal("unsettled result must name the pending pods")
			}
		})
	}
}
