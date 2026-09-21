package testcase

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/rancher/distros-test-framework/internal/provisioning/driver"
	"github.com/rancher/distros-test-framework/internal/resources"

	. "github.com/onsi/gomega"
)

const (
	domainRuntime   = "container_runtime_t"
	domainContainer = "container_t"
	domainSpc       = "spc_t"
	commContainerd  = "containerd"
	commShim        = "containerd-shim"
)

// TestSelinuxEnforcement fails unless every node is Enforcing with no permissive domain in use, has the container and
// product modules loaded, runs product+containerd as container_runtime_t and every container in its expected domain.
func TestSelinuxEnforcement(cluster *driver.Cluster) {
	agentless := agentDisabled(cluster.Config.ServerFlags)
	err := checkSelinuxEnforcement(resources.RunCommandOnNode, cluster.Config.Product, agentless,
		cluster.ServerIPs, cluster.AgentIPs)
	Expect(err).NotTo(HaveOccurred())
}

// agentDisabled reports whether server_flags really turns the agent off: `disable-agent: true` (YAML, any quoting)
// or a bare `--disable-agent` CLI flag; `disable-agent: false` is not agentless.
func agentDisabled(serverFlags string) bool {
	for _, raw := range strings.Split(strings.ReplaceAll(serverFlags, `\n`, "\n"), "\n") {
		line := strings.TrimSpace(raw)
		if line == "--disable-agent" || line == "disable-agent" {
			return true
		}
		key, val, ok := strings.Cut(line, ":")
		if !ok {
			key, val, ok = strings.Cut(line, "=")
		}
		if !ok || strings.TrimLeft(strings.TrimSpace(key), "-") != "disable-agent" {
			continue
		}
		switch strings.ToLower(strings.Trim(strings.TrimSpace(val), `"'`)) {
		case "true", "yes", "1":
			return true
		}
	}

	return false
}

// checkSelinuxEnforcement runs the per-node checks. agentless (k3s --disable-agent in server_flags) is the only
// way a server may legitimately run no containerd and no containers.
func checkSelinuxEnforcement(run func(cmd, ip string) (string, error), product string, agentless bool,
	serverIPs, agentIPs []string,
) error {
	if len(serverIPs)+len(agentIPs) == 0 {
		return errors.New("no node IPs to check SELinux enforcement on")
	}

	workloadNodes := 0
	for _, ip := range serverIPs {
		n, err := checkNodeSelinux(run, product, ip, agentless)
		if err != nil {
			return err
		}
		if n > 0 {
			workloadNodes++
		}
	}
	for _, ip := range agentIPs {
		n, err := checkNodeSelinux(run, product, ip, false)
		if err != nil {
			return err
		}
		if n > 0 {
			workloadNodes++
		}
	}
	if workloadNodes == 0 {
		return errors.New("no node runs any container; nothing to validate")
	}

	return nil
}

// checkNodeSelinux returns the number of classified containers on the node (0 only for an agentless server).
func checkNodeSelinux(run func(cmd, ip string) (string, error), product, ip string, agentless bool) (int, error) {
	out, err := run("cat /sys/fs/selinux/enforce", ip)
	if err != nil || strings.TrimSpace(out) != "1" {
		return 0, fmt.Errorf("SELinux is not enforcing on %s (enforce=%q, err=%v)", ip, strings.TrimSpace(out), err)
	}

	out, err = run("sudo semodule -l", ip)
	if err != nil {
		return 0, fmt.Errorf("semodule -l failed on %s: %w", ip, err)
	}
	if err := checkModulesLoaded(out, product, ip); err != nil {
		return 0, err
	}

	out, err = run(permissiveTypesCmd, ip)
	if err != nil {
		return 0, fmt.Errorf("permissive type query failed on %s: %w", ip, err)
	}
	permissive, err := parsePermissiveTypes(out, ip)
	if err != nil {
		return 0, err
	}

	return classifyNodeWithRetry(run, snapshotSleep, product, ip, agentless, permissive)
}

const (
	snapshotAttempts = 3
	snapshotBackoff  = 5 * time.Second
)

// snapshotSleep is the backoff between churn retries; tests replace it to avoid real waits.
var snapshotSleep = time.Sleep

// errSnapshotChurn marks mismatches between the ps and crictl snapshots (a pod started or stopped in between);
// those are retried, every other verdict is final.
var errSnapshotChurn = errors.New("snapshot churn")

// classifyNodeWithRetry takes the ps and crictl snapshots together and retries only when they disagree.
func classifyNodeWithRetry(run func(cmd, ip string) (string, error), sleep func(time.Duration), product, ip string,
	agentless bool, permissive map[string]bool,
) (int, error) {
	var (
		n   int
		err error
	)
	for attempt := 1; attempt <= snapshotAttempts; attempt++ {
		n, err = classifyNode(run, product, ip, agentless, permissive)
		if !errors.Is(err, errSnapshotChurn) {
			return n, err
		}
		resources.LogLevel("warn", "attempt %d/%d on %s: %v", attempt, snapshotAttempts, ip, err)
		if attempt < snapshotAttempts {
			sleep(snapshotBackoff)
		}
	}

	return n, err
}

func classifyNode(run func(cmd, ip string) (string, error), product, ip string, agentless bool,
	permissive map[string]bool,
) (int, error) {
	psOut, err := run("ps -eo label,pid,ppid,comm --no-headers", ip)
	if err != nil {
		return 0, fmt.Errorf("ps failed on %s: %w", ip, err)
	}
	procs := parseProcs(psOut)

	if err := checkRuntimeDomains(procs, product, ip, permissive); err != nil {
		return 0, err
	}
	if agentless && !hasAgentWorkloadSigns(procs, product) {
		return 0, nil
	}

	cOut, err := run(crictlInspectAllCmd(product), ip)
	if err != nil {
		return 0, fmt.Errorf("crictl inspect failed on %s: %w", ip, err)
	}
	pOut, err := run(crictlInspectPodsCmd(product), ip)
	if err != nil {
		return 0, fmt.Errorf("crictl inspectp failed on %s: %w", ip, err)
	}
	containers, err := parseCriDocs(cOut, pOut)
	if err != nil {
		return 0, fmt.Errorf("parsing crictl output from %s: %w", ip, err)
	}

	return checkContainerDomains(procs, containers, ip, permissive)
}

// checkModulesLoaded requires the container and product policy modules.
func checkModulesLoaded(semoduleOut, product, ip string) error {
	loaded := map[string]bool{}
	for _, m := range strings.Fields(semoduleOut) {
		loaded[m] = true
	}
	for _, m := range []string{"container", product} {
		if !loaded[m] {
			return fmt.Errorf("SELinux policy module %q is not loaded on %s", m, ip)
		}
	}

	return nil
}

// permissiveTypesCmd lists the effective permissive types and ends with "permissive-query: ok" only on success:
// semanage where present, otherwise the highest-priority enabled module of each name in the SELinux store.
const permissiveTypesCmd = `if command -v semanage >/dev/null 2>&1; then ` +
	`out=$(sudo semanage permissive -l 2>&1) || { echo "PERMISSIVE_QUERY_FAILED: semanage rc=$? $out"; exit 0; }; ` +
	`echo "$out" | grep -E '^[A-Za-z0-9_]+_t$' | sed 's/^/permissive: /'; echo "permissive-query: ok"; ` +
	`elif command -v python3 >/dev/null 2>&1; then sudo python3 -c "` + permissiveScanPy + `" || ` +
	`echo "PERMISSIVE_QUERY_FAILED: python scan rc=$?"; ` +
	`else echo "PERMISSIVE_QUERY_FAILED: neither semanage nor python3 available"; fi`

// permissiveScanPy mirrors libsemanage: per module name only the highest priority copy counts, names listed under
// active/modules/disabled/ are skipped, CIL is plain or bz2. Double quotes only (it is embedded in a shell "...").
const permissiveScanPy = `import bz2,glob,os,re,sys
store=os.environ.get(\"DTF_SELINUX_STORE\",\"/var/lib/selinux\")
roots=glob.glob(store+\"/*/active/modules\")
if not roots: sys.exit(\"no SELinux module store under \"+store)
best={}
for root in roots:
    disabled=set(os.listdir(root+\"/disabled\")) if os.path.isdir(root+\"/disabled\") else set()
    for pdir in glob.glob(root+\"/[0-9]*\"):
        prio=int(os.path.basename(pdir))
        for name in os.listdir(pdir):
            if name in disabled: continue
            f=pdir+\"/\"+name+\"/cil\"
            if os.path.isfile(f) and (name not in best or prio>best[name][0]): best[name]=(prio,f)
types=set()
for prio,f in best.values():
    raw=open(f,\"rb\").read()
    txt=(bz2.decompress(raw) if raw[:3]==b\"BZh\" else raw).decode(\"utf-8\",\"replace\")
    types.update(re.findall(r\"\(typepermissive\s+([A-Za-z0-9_]+)\)\",txt))
print(\"\n\".join(\"permissive: \"+x for x in sorted(types)))
print(\"permissive-query: ok\")`

// parsePermissiveTypes accepts the query only when it ends with the success marker; a failed or truncated query
// is an error, never "no permissive types".
func parsePermissiveTypes(out, ip string) (map[string]bool, error) {
	types := map[string]bool{}
	ok := false
	for _, raw := range strings.Split(out, "\n") {
		line := strings.TrimSpace(raw)
		switch {
		case strings.HasPrefix(line, "PERMISSIVE_QUERY_FAILED"):
			return nil, fmt.Errorf("cannot list permissive SELinux types on %s: %s", ip, line)
		case line == "permissive-query: ok":
			ok = true
		default:
			if t, found := strings.CutPrefix(line, "permissive: "); found {
				types[strings.TrimSpace(t)] = true
			}
		}
	}
	if !ok {
		return nil, fmt.Errorf("permissive type query on %s did not complete: %q", ip, strings.TrimSpace(out))
	}

	return types, nil
}

type proc struct {
	label, pid, ppid, comm string
}

func parseProcs(psOut string) []proc {
	var procs []proc
	for _, line := range strings.Split(psOut, "\n") {
		f := strings.Fields(line)
		if len(f) < 4 {
			continue
		}
		procs = append(procs, proc{label: f[0], pid: f[1], ppid: f[2], comm: f[3]})
	}

	return procs
}

// labelType returns the type field of an SELinux label (user:role:type:level).
func labelType(label string) string {
	parts := strings.Split(label, ":")
	if len(parts) < 3 {
		return ""
	}

	return parts[2]
}

func productComms(product string) map[string]bool {
	if product == "rke2" {
		return map[string]bool{"rke2": true}
	}

	return map[string]bool{"k3s-server": true, "k3s-agent": true}
}

// checkRuntimeDomains verifies product, containerd and shims run as container_runtime_t and that domain is enforced.
func checkRuntimeDomains(procs []proc, product, ip string, permissive map[string]bool) error {
	if permissive[domainRuntime] {
		return fmt.Errorf("%s is a permissive type on %s", domainRuntime, ip)
	}
	products := productComms(product)
	seen := false
	for _, p := range procs {
		if !products[p.comm] && p.comm != commContainerd && p.comm != commShim {
			continue
		}
		if labelType(p.label) != domainRuntime {
			return fmt.Errorf("%s (pid %s) runs as %q on %s, expected %s", p.comm, p.pid, p.label, ip, domainRuntime)
		}
		if products[p.comm] {
			seen = true
		}
	}
	if !seen {
		return fmt.Errorf("no %s process found on %s", product, ip)
	}

	return nil
}

// hasAgentWorkloadSigns reports whether a node shows an agent, containerd or shims, which contradicts --disable-agent.
func hasAgentWorkloadSigns(procs []proc, product string) bool {
	for _, p := range procs {
		if p.comm == commContainerd || p.comm == commShim || (product == "k3s" && p.comm == "k3s-agent") {
			return true
		}
	}

	return false
}

func crictlBin(product string) string {
	if product == "rke2" {
		return "/var/lib/rancher/rke2/bin/crictl --config /var/lib/rancher/rke2/agent/etc/crictl.yaml"
	}

	return "k3s crictl"
}

// crictlInspectAllCmd dumps the inspect JSON of every running container, one document after another.
func crictlInspectAllCmd(product string) string {
	c := crictlBin(product)

	return fmt.Sprintf(`sudo sh -c 'PATH=$PATH:/usr/local/bin:/opt/bin; `+
		`for id in $(%s ps -q); do %s inspect $id; done'`, c, c)
}

// crictlInspectPodsCmd dumps the inspectp JSON of every Ready sandbox (NotReady ones have no process).
func crictlInspectPodsCmd(product string) string {
	c := crictlBin(product)

	return fmt.Sprintf(`sudo sh -c 'PATH=$PATH:/usr/local/bin:/opt/bin; `+
		`for id in $(%s pods -q --state Ready); do %s inspectp $id; done'`, c, c)
}

// criEntry is one container or sandbox with the fields that decide its expected SELinux domain.
type criEntry struct {
	name, pod, ns string
	pid           string
	privileged    bool
	selinuxType   string
	runtimeLabel  string
	sandbox       bool
}

// criDoc is the subset of crictl inspect / inspectp JSON the classifier needs.
type criDoc struct {
	Status struct {
		Metadata struct {
			Name      string `json:"name"`
			Namespace string `json:"namespace"`
		} `json:"metadata"`
		Labels map[string]string `json:"labels"`
	} `json:"status"`
	Info struct {
		Pid    json.Number `json:"pid"`
		Config struct {
			Linux struct {
				SecurityContext struct {
					Privileged     bool `json:"privileged"`
					SelinuxOptions struct {
						Type string `json:"type"`
					} `json:"selinux_options"`
				} `json:"security_context"`
			} `json:"linux"`
		} `json:"config"`
		RuntimeSpec struct {
			Process struct {
				SelinuxLabel string `json:"selinuxLabel"`
			} `json:"process"`
		} `json:"runtimeSpec"`
	} `json:"info"`
}

// parseCriDocs decodes concatenated crictl JSON documents for containers and pod sandboxes.
func parseCriDocs(containersOut, podsOut string) ([]criEntry, error) {
	var entries []criEntry
	for _, in := range []struct {
		out     string
		sandbox bool
	}{{containersOut, false}, {podsOut, true}} {
		dec := json.NewDecoder(strings.NewReader(in.out))
		for {
			var d criDoc
			if err := dec.Decode(&d); errors.Is(err, io.EOF) {
				break
			} else if err != nil {
				return nil, err
			}
			e := criEntry{
				name:         d.Status.Metadata.Name,
				pod:          d.Status.Labels["io.kubernetes.pod.name"],
				ns:           d.Status.Labels["io.kubernetes.pod.namespace"],
				pid:          d.Info.Pid.String(),
				privileged:   d.Info.Config.Linux.SecurityContext.Privileged,
				selinuxType:  d.Info.Config.Linux.SecurityContext.SelinuxOptions.Type,
				runtimeLabel: d.Info.RuntimeSpec.Process.SelinuxLabel,
				sandbox:      in.sandbox,
			}
			if in.sandbox {
				e.pod, e.ns = d.Status.Metadata.Name, d.Status.Metadata.Namespace
			}
			entries = append(entries, e)
		}
	}

	return entries, nil
}

// expectedDomain follows containerd's CRI: a privileged container always gets spc_t (the requested seLinuxOptions
// are dropped, e.g. RKE2 kube-proxy), else the explicit type (rke2_service_t, rke2_service_db_t), else container_t.
func expectedDomain(e *criEntry) string {
	switch {
	case e.privileged:
		return domainSpc
	case e.selinuxType != "":
		return e.selinuxType
	default:
		return domainContainer
	}
}

// checkContainerDomains classifies every shim child through crictl and requires each to run in its expected,
// non-permissive domain; an unclassified process under a shim is an error, not a skip.
func checkContainerDomains(procs []proc, containers []criEntry, ip string, permissive map[string]bool) (int, error) {
	byPid := map[string]proc{}
	shims := map[string]bool{}
	for _, p := range procs {
		byPid[p.pid] = p
		if p.comm == commShim {
			shims[p.pid] = true
		}
	}
	if len(containers) == 0 {
		return 0, fmt.Errorf("no containers reported by crictl on %s although the node runs an agent", ip)
	}

	classified := map[string]bool{}
	for i := range containers {
		c := &containers[i]
		// crictl pods also lists sandboxes of finished pods (completed jobs); they have no process to classify.
		if c.pid == "" || c.pid == "0" {
			continue
		}
		p, ok := byPid[c.pid]
		if !ok {
			return 0, fmt.Errorf("%w: %s %s/%s (pid %s) reported by crictl is not in the process list on %s",
				errSnapshotChurn, kind(c), c.ns, c.pod, c.pid, ip)
		}
		want := expectedDomain(&containers[i])
		if got := labelType(p.label); got != want {
			return 0, fmt.Errorf("%s %s/%s (%s, pid %s) runs as %s on %s, expected %s", kind(c), c.ns, c.pod,
				c.name, c.pid, got, ip, want)
		}
		if permissive[want] {
			return 0, fmt.Errorf("%s %s/%s runs in %s which is a permissive type on %s", kind(c), c.ns, c.pod, want, ip)
		}
		classified[c.pid] = true
	}

	for _, p := range procs {
		if shims[p.ppid] && !classified[p.pid] {
			return 0, fmt.Errorf("%w: process %q (pid %s, label %s) under a containerd-shim on %s is not a known container",
				errSnapshotChurn, p.comm, p.pid, p.label, ip)
		}
	}

	return len(classified), nil
}

func kind(c *criEntry) string {
	if c.sandbox {
		return "sandbox"
	}

	return "container"
}
