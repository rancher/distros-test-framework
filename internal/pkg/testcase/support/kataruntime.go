package support

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"
)

type kataIdentity struct {
	UID, Sandbox, Runtime string
	Handler               string
	ContainerIDs          []string `json:"container_ids"`
	Processes             []kataProcess
	VMs                   []kataProcess `json:"vms"`
	HostKernel            string        `json:"host_kernel"`
	HostBootID            string        `json:"host_boot_id"`
	Containers            []kataContainer
	Samples               []kataIdleSample
}

func (s *KataRun) identity(ctx context.Context, pod *kataPod) (json.RawMessage, error) {
	record, err := s.captureIdentity(ctx, pod)
	if err != nil {
		return nil, fmt.Errorf("capture runtime identity for pod %s/%s: %w", pod.Namespace, pod.Name, err)
	}

	data, err := json.Marshal(record)
	if err != nil {
		return nil, fmt.Errorf("encode runtime identity for pod %s: %w", pod.Name, err)
	}

	if evidenceErr := s.evidence(pod.Name+"-identity", record); evidenceErr != nil {
		return nil, evidenceErr
	}

	return data, nil
}

func (s *KataRun) exec(ctx context.Context, pod *kataPod, args ...string) (string, error) {
	return s.kubectl(ctx, append([]string{"exec", "-n", pod.Namespace, pod.Name, "--"}, args...)...)
}

// RuntimeIdentity implements KATA-02 for the explicit handler and default Kata alias.
func (s *KataRun) RuntimeIdentity(ctx context.Context) error {
	for _, fixture := range []struct{ name, class string }{{"kata-local", kataClass}, {"kata-alias", "kata"}} {
		pod, err := s.startPod(ctx, fixture.name, s.eligible, s.eligibleIP, fixture.class, s.opts.Image)
		if err != nil {
			return err
		}

		if kernelErr := s.guestKernel(ctx, pod); kernelErr != nil {
			return kernelErr
		}

		if createErr := s.create(ctx, fixture.name+"-service", s.service(fixture.name)); createErr != nil {
			return createErr
		}
	}
	_, err := s.startPod(ctx, "runc-after", s.eligible, s.eligibleIP, "", s.opts.Image)

	return err
}

func (s *KataRun) guestKernel(ctx context.Context, pod *kataPod) error {
	var host kataIdentity
	if err := json.Unmarshal(pod.Identity, &host); err != nil {
		return fmt.Errorf("decode host identity for guest-kernel check of pod %s: %w", pod.Name, err)
	}

	output, err := s.exec(ctx, pod, "sh", "-ec", "uname -r; cat /proc/sys/kernel/random/boot_id")
	if err != nil {
		return fmt.Errorf("read guest kernel and boot ID in pod %s: %w", pod.Name, err)
	}

	fields := strings.Fields(output)
	if len(fields) != 2 || host.HostKernel == "" || host.HostBootID == "" || fields[1] == host.HostBootID {
		return fmt.Errorf("pod %s: guest kernel/boot identity not established independently of the host", pod.Name)
	}

	return s.evidence(pod.Name+"-guest-kernel", map[string]any{
		"guest_kernel": fields[0], "guest_boot_id": fields[1],
		"host_kernel": host.HostKernel, "host_boot_id": host.HostBootID,
	})
}

func (s *KataRun) deletePod(ctx context.Context, pod *kataPod) error {
	if _, err := s.kubectl(ctx, "delete", "pod", pod.Name, "-n", pod.Namespace, "--wait=false"); err != nil {
		return fmt.Errorf("delete pod %s/%s: %w", pod.Namespace, pod.Name, err)
	}

	err := pollKata(ctx, 3*time.Minute, func(ctx context.Context) error {
		if sandboxErr := s.noSandbox(ctx, pod.IP, pod.UID); sandboxErr != nil {
			return sandboxErr
		}
		return s.oldVMsGone(ctx, pod.IP, pod.Identity)
	})
	if err != nil {
		return fmt.Errorf("wait for pod %s sandbox/VM cleanup: %w", pod.Name, err)
	}
	delete(s.pods, pod.Name)

	return nil
}

type kataProcess struct {
	PID, PPID  int
	StartTicks uint64 `json:"start_ticks"`
	CPUTicks   uint64 `json:"cpu_ticks"`
	Args       []string
}

type kataContainer struct {
	ID, Snapshotter, SnapshotKey string
	Runtime                      struct{ Name string }
}

type kataSandbox struct {
	ID, State string
	Metadata  struct{ UID string }
}

func (s *KataRun) criJSON(ctx context.Context, ip string, target any, args ...string) error {
	prefix := []string{"sudo", "-n", cri, "--runtime-endpoint", "unix://" + containerdSocket}
	output, err := s.node(ctx, ip, append(prefix, args...)...)
	if err != nil {
		return fmt.Errorf("CRI %s on node %s: %w", strings.Join(args, " "), ip, err)
	}

	if decodeErr := json.Unmarshal([]byte(output), target); decodeErr != nil {
		return fmt.Errorf("decode CRI %s on node %s: %w", strings.Join(args, " "), ip, decodeErr)
	}

	return nil
}

func (s *KataRun) sandboxes(ctx context.Context, ip string) ([]kataSandbox, error) {
	var result struct{ Items []kataSandbox }
	err := s.criJSON(ctx, ip, &result, "pods", "-o", "json")

	return result.Items, err
}

func (s *KataRun) sandboxIDs(ctx context.Context, pod *kataPod) (sandbox string, containers []string, err error) {
	boxes, err := s.sandboxes(ctx, pod.IP)
	if err != nil {
		return "", nil, err
	}

	var ready []string
	for _, box := range boxes {
		if box.Metadata.UID == pod.UID && box.State == "SANDBOX_READY" {
			ready = append(ready, box.ID)
		}
	}
	if len(ready) != 1 || ready[0] == "" {
		return "", nil, fmt.Errorf("pod %s (UID %s): expected one Ready sandbox, found %d", pod.Name, pod.UID, len(ready))
	}

	var listing struct{ Containers []struct{ ID, State string } }
	if listErr := s.criJSON(ctx, pod.IP, &listing, "ps", "--pod", ready[0], "-o", "json"); listErr != nil {
		return "", nil, listErr
	}

	var ids []string
	for _, container := range listing.Containers {
		if container.State == "CONTAINER_RUNNING" && container.ID != "" {
			ids = append(ids, container.ID)
		}
	}
	if len(ids) == 0 {
		return "", nil, fmt.Errorf("sandbox %s for pod %s has no running workload container", ready[0], pod.Name)
	}
	slices.Sort(ids)

	return ready[0], ids, nil
}

func (s *KataRun) captureIdentity(ctx context.Context, pod *kataPod) (*kataIdentity, error) {
	sid, ids, err := s.sandboxIDs(ctx, pod)
	if err != nil {
		return nil, err
	}

	result := &kataIdentity{
		UID:          pod.UID,
		Sandbox:      sid,
		ContainerIDs: ids,
	}
	var inspection struct {
		Status struct{ RuntimeHandler string }
	}

	if inspectErr := s.criJSON(ctx, pod.IP, &inspection, "inspectp", sid); inspectErr != nil {
		return nil, inspectErr
	}
	result.Handler = inspection.Status.RuntimeHandler
	allIDs := append([]string{sid}, ids...)
	for _, id := range allIDs {
		container, containerErr := s.containerInfo(ctx, pod.IP, id)
		if containerErr != nil {
			return nil, containerErr
		}
		result.Containers = append(result.Containers, container)
	}

	if runtimeErr := validateKataRuntime(result, pod.Runtime); runtimeErr != nil {
		return nil, runtimeErr
	}

	table, err := s.processes(ctx, pod.IP)
	if err != nil {
		return nil, err
	}

	result.Processes, err = selectKataProcesses(table, allIDs)
	if err != nil {
		return nil, fmt.Errorf("correlate sandbox %s with host processes: %w", sid, err)
	}

	if vmErr := s.verifyVM(ctx, pod.IP, pod.Runtime, result); vmErr != nil {
		return nil, fmt.Errorf("verify sandbox %s VM: %w", sid, vmErr)
	}
	if kernelErr := s.hostKernelIdentity(ctx, pod.IP, result); kernelErr != nil {
		return nil, kernelErr
	}

	return result, nil
}

func (s *KataRun) hostKernelIdentity(ctx context.Context, ip string, record *kataIdentity) error {
	output, err := s.node(ctx, ip, "sh", "-ec", "uname -r; cat /proc/sys/kernel/random/boot_id")
	if err != nil {
		return fmt.Errorf("read kernel and boot ID on node %s: %w", ip, err)
	}

	fields := strings.Fields(output)
	if len(fields) != 2 {
		return fmt.Errorf("node %s: missing host kernel/boot identity", ip)
	}
	record.HostKernel, record.HostBootID = fields[0], fields[1]

	return nil
}

func (s *KataRun) containerInfo(ctx context.Context, ip, id string) (kataContainer, error) {
	var container kataContainer
	output, err := s.node(ctx, ip, "sudo", "-n", ctr, "--address", containerdSocket,
		"--namespace", "k8s.io", "containers", "info", id)
	if err != nil {
		return container, fmt.Errorf("inspect containerd container %s: %w", id, err)
	}
	if decodeErr := json.Unmarshal([]byte(output), &container); decodeErr != nil {
		return container, fmt.Errorf("decode containerd container %s inspection: %w", id, decodeErr)
	}
	if container.ID != id {
		return container, fmt.Errorf("containerd returned container %q; expected %q", container.ID, id)
	}

	return container, nil
}

func validateKataRuntime(record *kataIdentity, runtime string) error {
	expected := "io.containerd.runc.v2"
	if runtime == "kata" {
		expected = "io.containerd.kata-qemu-runtime-rs.v2"
	}
	if runtime != "kata" && runtime != "runc" {
		return errors.New("unknown expected runtime")
	}
	if len(record.Containers) == 0 {
		return errors.New("no containerd runtime evidence")
	}

	for i := range record.Containers {
		if record.Containers[i].Runtime.Name != expected {
			return fmt.Errorf("container %s: runtime %q; expected %q",
				record.Containers[i].ID, record.Containers[i].Runtime.Name, expected)
		}
	}
	record.Runtime = expected

	return nil
}

func (s *KataRun) verifyVM(ctx context.Context, ip, runtime string, record *kataIdentity) error {
	for i := range record.Processes {
		process := &record.Processes[i]
		if strings.Contains(filepath.Base(process.Args[0]), "qemu-system") {
			record.VMs = append(record.VMs, *process)
		}
	}

	if (runtime == "kata" && len(record.VMs) != 1) || (runtime == "runc" && len(record.VMs) != 0) {
		return fmt.Errorf("ambiguous sandbox-to-VM mapping: runtime %s has %d VMs", runtime, len(record.VMs))
	}
	if runtime == "runc" {
		return nil
	}

	output, err := s.node(ctx, ip, "sudo", "-n", "sh", "-ec",
		`for fd in /proc/"$1"/fd/*; do readlink "$fd"; done`, "probe", strconv.Itoa(record.VMs[0].PID))
	if err != nil {
		return fmt.Errorf("inspect QEMU PID %d KVM descriptors: %w", record.VMs[0].PID, err)
	}

	if !strings.Contains(output, "kvm-vm") {
		return errors.New("QEMU is not proven to use KVM acceleration")
	}

	return nil
}

const kataProcessRead = `for p in /proc/[0-9]*; do
  test -d "$p" || continue
  stat=$(base64 < "$p/stat" 2>/dev/null) || { test ! -d "$p" && continue; exit 1; }
  args=$(base64 < "$p/cmdline" 2>/dev/null) || { test ! -d "$p" && continue; exit 1; }
  test -d "$p" || continue
  printf '%s\t%s\t%s\n' "${p##*/}" "$(printf '%s' "$stat" | tr -d '\n')" "$(printf '%s' "$args" | tr -d '\n')"
done`

func (s *KataRun) processes(ctx context.Context, ip string) ([]kataProcess, error) {
	output, err := s.node(ctx, ip, "sudo", "-n", "sh", "-ec", kataProcessRead)
	if err != nil {
		return nil, fmt.Errorf("read host process table on node %s: %w", ip, err)
	}

	var result []kataProcess
	for _, line := range strings.Split(strings.TrimSuffix(output, "\n"), "\n") {
		process, parseErr := parseKataProcess(line)
		if parseErr != nil {
			return nil, fmt.Errorf("parse host process table on node %s: %w", ip, parseErr)
		}
		result = append(result, process)
	}

	return result, nil
}

func parseKataProcess(line string) (kataProcess, error) {
	var result kataProcess
	parts := strings.Split(line, "\t")
	if len(parts) != 3 {
		return result, errors.New("invalid process observation")
	}

	pid, err := strconv.Atoi(parts[0])
	if err != nil || pid < 1 {
		return result, errors.New("invalid process PID")
	}

	result, statErr := parseKataStat(pid, parts[1])
	if statErr != nil {
		return result, fmt.Errorf("parse process %d stat: %w", pid, statErr)
	}

	args, decodeErr := base64.StdEncoding.DecodeString(parts[2])
	if decodeErr != nil {
		return result, fmt.Errorf("decode process %d cmdline: %w", pid, decodeErr)
	}
	result.Args = strings.Split(strings.TrimSuffix(string(args), "\x00"), "\x00")

	return result, nil
}

func parseKataStat(pid int, encoded string) (kataProcess, error) {
	result := kataProcess{PID: pid}
	data, decodeErr := base64.StdEncoding.DecodeString(encoded)
	if decodeErr != nil {
		return result, fmt.Errorf("decode stat: %w", decodeErr)
	}

	stat := string(data)
	end := strings.LastIndex(stat, ") ")
	if end < 0 || !strings.HasPrefix(stat, strconv.Itoa(pid)+" (") {
		return result, errors.New("invalid stat or mismatched PID")
	}

	fields := strings.Fields(stat[end+2:])
	if len(fields) < 20 {
		return result, errors.New("short process stat")
	}

	parent, err := strconv.Atoi(fields[1])
	if err != nil || parent < 0 {
		return result, errors.New("invalid parent PID")
	}

	values := make([]uint64, 3)
	for i, index := range []int{11, 12, 19} {
		values[i], err = strconv.ParseUint(fields[index], 10, 64)
		if err != nil {
			return result, fmt.Errorf("parse stat field %d: %w", index+3, err)
		}
	}

	result = kataProcess{
		PID:        pid,
		PPID:       parent,
		CPUTicks:   values[0] + values[1],
		StartTicks: values[2],
	}

	return result, nil
}

func selectKataProcesses(table []kataProcess, ids []string) ([]kataProcess, error) {
	selected := make(map[int]bool)
	for i := range table {
		p := &table[i]
		if len(p.Args) == 0 || !strings.Contains(filepath.Base(p.Args[0]), "containerd-shim") {
			continue
		}
		for _, id := range ids {
			if id != "" && slices.Contains(p.Args, id) {
				selected[p.PID] = true
			}
		}
	}

	if len(selected) == 0 {
		return nil, errors.New("no shim with an exact sandbox/container ID argument")
	}

	for i := range table {
		p := &table[i]
		if len(p.Args) > 0 && (strings.Contains(filepath.Base(p.Args[0]), "qemu-system") ||
			strings.Contains(filepath.Base(p.Args[0]), "virtiofs")) && kataArgsContainID(p.Args, ids) {
			selected[p.PID] = true
		}
	}

	for changed := true; changed; {
		changed = false
		for i := range table {
			p := &table[i]
			if selected[p.PPID] && !selected[p.PID] {
				selected[p.PID], changed = true, true
			}
		}
	}

	var result []kataProcess
	for i := range table {
		if selected[table[i].PID] {
			result = append(result, table[i])
		}
	}
	slices.SortFunc(result, func(a, b kataProcess) int { return a.PID - b.PID })

	return result, nil
}

func kataArgsContainID(args, ids []string) bool {
	for _, id := range ids {
		for _, arg := range args {
			if id != "" && strings.Contains(arg, id) {
				return true
			}
		}
	}

	return false
}

func (s *KataRun) noSandbox(ctx context.Context, ip, uid string) error {
	boxes, err := s.sandboxes(ctx, ip)
	if err != nil {
		return err
	}

	for _, box := range boxes {
		if box.Metadata.UID == uid {
			return fmt.Errorf("pod UID %s still has CRI sandbox %s on node %s", uid, box.ID, ip)
		}
	}

	return nil
}

func (s *KataRun) oldVMsGone(ctx context.Context, ip string, previous json.RawMessage) error {
	var record kataIdentity
	if err := json.Unmarshal(previous, &record); err != nil {
		return fmt.Errorf("decode previous runtime identity for VM cleanup: %w", err)
	}
	if len(record.VMs) == 0 {
		return nil
	}

	table, err := s.processes(ctx, ip)
	if err != nil {
		return err
	}

	for i := range record.VMs {
		for j := range table {
			if record.VMs[i].PID == table[j].PID && record.VMs[i].StartTicks == table[j].StartTicks {
				return fmt.Errorf("previous Kata VM PID %d for sandbox %s is still alive", table[j].PID, record.Sandbox)
			}
		}
	}

	return nil
}
