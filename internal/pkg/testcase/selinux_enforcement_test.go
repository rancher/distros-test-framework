package testcase

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

const (
	rt  = "system_u:system_r:container_runtime_t:s0"
	ct  = "system_u:system_r:container_t:s0:c1,c2"
	spc = "system_u:system_r:spc_t:s0"
	svc = "system_u:system_r:rke2_service_t:s0"
	db  = "system_u:system_r:rke2_service_db_t:s0"

	modulesK3s   = "container\nk3s\npasst\n"
	modulesRKE2  = "container\nrke2\n"
	noPermissive = "permissive: systemd_generic_generator_t\npermissive-query: ok\n"
)

// criJSON builds one crictl inspect/inspectp document.
func criJSON(name, ns, pod string, pid int, privileged bool, seType, label string) string {
	return fmt.Sprintf(`{"status":{"metadata":{"name":%q,"namespace":%q},"labels":{"io.kubernetes.pod.name":%q,`+
		`"io.kubernetes.pod.namespace":%q}},"info":{"pid":%d,"config":{"linux":{"security_context":{"privileged":%v,`+
		`"selinux_options":{"type":%q}}}},"runtimeSpec":{"process":{"selinuxLabel":%q}}}}`,
		name, ns, pod, ns, pid, privileged, seType, label)
}

type fakeNode struct {
	enforce, modules, permissive, ps, containers, pods string
	err                                                error
}

func fakeRunner(nodes map[string]fakeNode) func(cmd, ip string) (string, error) {
	return func(cmd, ip string) (string, error) {
		n, ok := nodes[ip]
		if !ok {
			return "", errors.New("unknown node " + ip)
		}
		if n.err != nil {
			return "", n.err
		}
		switch {
		case strings.Contains(cmd, "/sys/fs/selinux/enforce"):
			return n.enforce, nil
		case strings.Contains(cmd, "semodule -l"):
			return n.modules, nil
		case strings.Contains(cmd, "typepermissive"):
			return n.permissive, nil
		case strings.Contains(cmd, "ps -eo"):
			return n.ps, nil
		case strings.Contains(cmd, "inspectp"):
			return n.pods, nil
		case strings.Contains(cmd, "inspect"):
			return n.containers, nil
		}

		return "", errors.New("unexpected command " + cmd)
	}
}

// k3sServer is a healthy k3s server: coredns pod (sandbox 121 + container 122) under shim 120.
func k3sServer() fakeNode {
	return fakeNode{
		enforce: "1\n", modules: modulesK3s, permissive: noPermissive,
		ps: rt + " 100 1 k3s-server\n" + rt + " 110 100 containerd\n" + rt + " 120 1 containerd-shim\n" +
			ct + " 121 120 pause\n" + ct + " 122 120 coredns\n" + "system_u:system_r:kernel_t:s0 2 0 kthreadd\n",
		containers: criJSON("coredns", "kube-system", "coredns-x", 122, false, "", ct),
		pods:       criJSON("coredns-x", "kube-system", "", 121, false, "", ct),
	}
}

func k3sAgent() fakeNode {
	return fakeNode{
		enforce: "1\n", modules: modulesK3s, permissive: noPermissive,
		ps: rt + " 200 1 k3s-agent\n" + rt + " 210 200 containerd\n" + rt + " 220 1 containerd-shim\n" +
			ct + " 221 220 pause\n" + ct + " 222 220 svclb\n",
		containers: criJSON("lb", "kube-system", "svclb-x", 222, false, "", ct),
		pods:       criJSON("svclb-x", "kube-system", "", 221, false, "", ct),
	}
}

func TestEnforcementHealthyK3s(t *testing.T) {
	nodes := map[string]fakeNode{"s1": k3sServer(), "a1": k3sAgent()}
	if err := checkSelinuxEnforcement(fakeRunner(nodes), "k3s", false, []string{"s1"}, []string{"a1"}); err != nil {
		t.Fatalf("healthy cluster must pass: %v", err)
	}
	if err := checkSelinuxEnforcement(fakeRunner(nodes), "k3s", false, nil, nil); err == nil {
		t.Fatal("empty IP list must fail")
	}
}

func TestEnforcementRKE2DomainsAndPrivileged(t *testing.T) {
	// static pods carry rke2_service_t / rke2_service_db_t via seLinuxOptions, canal is privileged -> spc_t
	ps := rt + " 100 1 rke2\n" + rt + " 110 100 containerd\n" + rt + " 120 1 containerd-shim\n" +
		svc + " 121 120 pause\n" + svc + " 122 120 kube-apiserver\n" +
		rt + " 130 1 containerd-shim\n" + db + " 131 130 pause\n" + db + " 132 130 etcd\n" +
		rt + " 140 1 containerd-shim\n" + spc + " 141 140 pause\n" + spc + " 142 140 calico-node\n"
	containers := criJSON("kube-apiserver", "kube-system", "kube-apiserver-n", 122, false, "rke2_service_t", svc) +
		criJSON("etcd", "kube-system", "etcd-n", 132, false, "rke2_service_db_t", db) +
		criJSON("calico-node", "kube-system", "rke2-canal-x", 142, true, "", spc)
	pods := criJSON("kube-apiserver-n", "kube-system", "", 121, false, "rke2_service_t", svc) +
		criJSON("etcd-n", "kube-system", "", 131, false, "rke2_service_db_t", db) +
		criJSON("rke2-canal-x", "kube-system", "", 141, true, "", spc)
	node := fakeNode{
		enforce: "1", modules: modulesRKE2, permissive: noPermissive, ps: ps, containers: containers, pods: pods,
	}
	if err := checkSelinuxEnforcement(fakeRunner(map[string]fakeNode{"s1": node}), "rke2", false,
		[]string{"s1"}, nil); err != nil {
		t.Fatalf("official rke2 domains and privileged canal must pass: %v", err)
	}

	// the same canal container running spc_t WITHOUT being privileged is unconfined and must fail
	bad := node
	bad.containers = strings.Replace(containers, `"privileged":true`, `"privileged":false`, 1)
	err := checkSelinuxEnforcement(fakeRunner(map[string]fakeNode{"s1": bad}), "rke2", false, []string{"s1"}, nil)
	if err == nil || !strings.Contains(err.Error(), "runs as spc_t") {
		t.Fatalf("non-privileged container in spc_t must fail, got %v", err)
	}
}

func TestEnforcementPrivilegedWinsOverSelinuxType(t *testing.T) {
	// RKE2 kube-proxy is privileged and its pod asks for rke2_service_t; containerd drops the type and runs spc_t
	ps := rt + " 100 1 rke2\n" + rt + " 110 100 containerd\n" + rt + " 120 1 containerd-shim\n" +
		spc + " 121 120 pause\n" + spc + " 122 120 kube-proxy\n"
	node := fakeNode{
		enforce: "1", modules: modulesRKE2, permissive: noPermissive, ps: ps,
		containers: criJSON("kube-proxy", "kube-system", "kube-proxy-n", 122, true, "rke2_service_t", spc),
		pods:       criJSON("kube-proxy-n", "kube-system", "", 121, true, "rke2_service_t", spc),
	}
	err := checkSelinuxEnforcement(fakeRunner(map[string]fakeNode{"s1": node}), "rke2", false, []string{"s1"}, nil)
	if err != nil {
		t.Fatalf("privileged kube-proxy in spc_t must pass: %v", err)
	}
	if got := expectedDomain(&criEntry{privileged: true, selinuxType: "rke2_service_t"}); got != domainSpc {
		t.Fatalf("privileged must win over seLinuxOptions, got %s", got)
	}
}

func TestAgentDisabled(t *testing.T) {
	cases := map[string]bool{
		"":                        false,
		"disable-agent: true":     true,
		"disable-agent: \"true\"": true,
		"protect-kernel-defaults: true\ndisable-agent: true": true,
		`protect-kernel-defaults: true\ndisable-agent: yes`:  true,
		"disable-agent: false":                               false,
		"disable-agent: \"false\"":                           false,
		"--disable-agent":                                    true,
		"--disable-agent=false":                              false,
		"# disable-agent mentioned in a comment":             false,
	}
	for flags, want := range cases {
		if got := agentDisabled(flags); got != want {
			t.Fatalf("agentDisabled(%q) = %v, want %v", flags, got, want)
		}
	}
}

func TestEnforcementAgentlessNeedsExplicitFlag(t *testing.T) {
	agentless := fakeNode{enforce: "1", modules: modulesK3s, permissive: noPermissive, ps: rt + " 100 1 k3s-server\n"}
	nodes := map[string]fakeNode{"s1": agentless, "a1": k3sAgent()}
	if err := checkSelinuxEnforcement(fakeRunner(nodes), "k3s", true, []string{"s1"}, []string{"a1"}); err != nil {
		t.Fatalf("server with --disable-agent must pass: %v", err)
	}
	err := checkSelinuxEnforcement(fakeRunner(nodes), "k3s", false, []string{"s1"}, []string{"a1"})
	if err == nil || !strings.Contains(err.Error(), "no containers reported by crictl") {
		t.Fatalf("server without workloads and without disable-agent must fail, got %v", err)
	}
	// a server that has containerd but no workloads is not agentless either
	idle := k3sServer()
	idle.ps = rt + " 100 1 k3s-server\n" + rt + " 110 100 containerd\n"
	idle.containers, idle.pods = "", ""
	err = checkSelinuxEnforcement(fakeRunner(map[string]fakeNode{"s1": idle, "a1": k3sAgent()}), "k3s", true,
		[]string{"s1"}, []string{"a1"})
	if err == nil || !strings.Contains(err.Error(), "no containers reported by crictl") {
		t.Fatalf("containerd present contradicts disable-agent, got %v", err)
	}
}

func failingNodes() map[string]fakeNode {
	permissiveCT := noPermissive + "permissive: container_t\n"
	permissiveRT := noPermissive + "permissive: container_runtime_t\n"
	semanageFailed := "PERMISSIVE_QUERY_FAILED: semanage rc=42\n"
	unconfinedServer := "system_u:system_r:unconfined_service_t:s0 100 1 k3s-server"
	mutate := func(f func(*fakeNode)) fakeNode { n := k3sServer(); f(&n); return n }
	swapPS := func(from, to string) fakeNode {
		return mutate(func(n *fakeNode) { n.ps = strings.Replace(n.ps, from, to, 1) })
	}

	return map[string]fakeNode{
		"permissive mode":              mutate(func(n *fakeNode) { n.enforce = "0\n" }),
		"selinux disabled":             {err: errors.New("No such file")},
		"product module missing":       mutate(func(n *fakeNode) { n.modules = "container\n" }),
		"container module missing":     mutate(func(n *fakeNode) { n.modules = "k3s\n" }),
		"container_t permissive":       mutate(func(n *fakeNode) { n.permissive = permissiveCT }),
		"runtime permissive":           mutate(func(n *fakeNode) { n.permissive = permissiveRT }),
		"permissive query unavailable": mutate(func(n *fakeNode) { n.permissive = "PERMISSIVE_QUERY_FAILED: neither\n" }),
		"semanage failed":              mutate(func(n *fakeNode) { n.permissive = semanageFailed }),
		"query without marker":         mutate(func(n *fakeNode) { n.permissive = "" }),
		"query error text only":        mutate(func(n *fakeNode) { n.permissive = "ValueError: boom\n" }),
		"product not runtime domain":   swapPS(rt+" 100 1 k3s-server", unconfinedServer),
		"no product process":           swapPS(rt+" 100 1 k3s-server\n", ""),
		"container in spc_t":           swapPS(ct+" 122 120 coredns", spc+" 122 120 coredns"),
		"workload in runtime domain":   swapPS(ct+" 122 120 coredns", rt+" 122 120 coredns"),
		"unclassified shim child":      mutate(func(n *fakeNode) { n.ps += ct + " 123 120 mystery\n" }),
		"crictl pid not in ps":         swapPS(ct+" 122 120 coredns\n", ""),
	}
}

func TestEnforcementFailures(t *testing.T) {
	snapshotSleep = func(time.Duration) {}
	t.Cleanup(func() { snapshotSleep = time.Sleep })
	want := map[string]string{
		"permissive mode":              "not enforcing",
		"selinux disabled":             "not enforcing",
		"product module missing":       `module "k3s" is not loaded`,
		"container module missing":     `module "container" is not loaded`,
		"container_t permissive":       "container_t which is a permissive type",
		"runtime permissive":           "container_runtime_t is a permissive type",
		"permissive query unavailable": "cannot list permissive SELinux types",
		"semanage failed":              "semanage rc=42",
		"query without marker":         "did not complete",
		"query error text only":        "did not complete",
		"product not runtime domain":   "expected container_runtime_t",
		"no product process":           "no k3s process found",
		"container in spc_t":           "runs as spc_t",
		"workload in runtime domain":   "runs as container_runtime_t",
		"unclassified shim child":      "is not a known container",
		"crictl pid not in ps":         "is not in the process list",
	}
	for name, node := range failingNodes() {
		t.Run(name, func(t *testing.T) {
			err := checkSelinuxEnforcement(fakeRunner(map[string]fakeNode{"s1": node}), "k3s", false, []string{"s1"}, nil)
			if err == nil || !strings.Contains(err.Error(), want[name]) {
				t.Fatalf("expected error containing %q, got %v", want[name], err)
			}
		})
	}
}

func TestEnforcementIgnoresFinishedSandboxes(t *testing.T) {
	// a completed helm-install job leaves a NotReady sandbox with pid 0 in crictl pods; it must not count as missing
	node := k3sServer()
	node.pods += criJSON("helm-install-traefik-x", "kube-system", "", 0, false, "", "")
	err := checkSelinuxEnforcement(fakeRunner(map[string]fakeNode{"s1": node}), "k3s", false, []string{"s1"}, nil)
	if err != nil {
		t.Fatalf("finished sandbox without a process must be skipped: %v", err)
	}
}

func TestEnforcementPodsCmdOnlyReadySandboxes(t *testing.T) {
	for _, product := range []string{"k3s", "rke2"} {
		if !strings.Contains(crictlInspectPodsCmd(product), "pods -q --state Ready") {
			t.Fatalf("%s: sandbox listing must be limited to Ready pods, got %q", product, crictlInspectPodsCmd(product))
		}
	}
}

func TestEnforcementRetriesSnapshotChurn(t *testing.T) {
	// first two ps snapshots miss the container pid (pod just started), the third is consistent
	calls := 0
	base := k3sServer()
	run := func(cmd, ip string) (string, error) {
		if strings.Contains(cmd, "ps -eo") {
			calls++
			if calls < 3 {
				return strings.Replace(base.ps, ct+" 122 120 coredns\n", "", 1), nil
			}
		}

		return fakeRunner(map[string]fakeNode{"s1": base})(cmd, ip)
	}
	sleeps := 0
	n, err := classifyNodeWithRetry(run, func(time.Duration) { sleeps++ }, "k3s", "s1", false, map[string]bool{})
	if err != nil || n != 2 || calls != 3 || sleeps != 2 {
		t.Fatalf("churn must be retried until consistent: n=%d calls=%d sleeps=%d err=%v", n, calls, sleeps, err)
	}

	// a real domain violation is final: no retry
	calls, sleeps = 0, 0
	bad := k3sServer()
	bad.ps = strings.Replace(bad.ps, ct+" 122 120 coredns", rt+" 122 120 coredns", 1)
	_, err = classifyNodeWithRetry(fakeRunner(map[string]fakeNode{"s1": bad}), func(time.Duration) { sleeps++ },
		"k3s", "s1", false, map[string]bool{})
	if err == nil || sleeps != 0 {
		t.Fatalf("domain violation must fail without retry, sleeps=%d err=%v", sleeps, err)
	}
}

func TestEnforcementWorkerCaughtWhenServerFine(t *testing.T) {
	worker := k3sAgent()
	worker.ps = strings.Replace(worker.ps, ct+" 222 220 svclb", rt+" 222 220 svclb", 1)
	err := checkSelinuxEnforcement(fakeRunner(map[string]fakeNode{"s1": k3sServer(), "a1": worker}), "k3s", false,
		[]string{"s1"}, []string{"a1"})
	if err == nil || !strings.Contains(err.Error(), "on a1") {
		t.Fatalf("unconfined worker container must fail naming the worker, got %v", err)
	}
}

func TestParsePermissiveTypesSemanageFormat(t *testing.T) {
	out := "permissive: container_t\npermissive: ktlshd_t\npermissive-query: ok\n"
	types, err := parsePermissiveTypes(out, "1.2.3.4")
	if err != nil || !types["container_t"] || !types["ktlshd_t"] || len(types) != 2 {
		t.Fatalf("unexpected parse result %v %v", types, err)
	}
}
