package support

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"
	"time"

	core "k8s.io/api/core/v1"
)

const (
	ctr              = "/var/lib/rancher/rke2/bin/ctr"
	cri              = "/var/lib/rancher/rke2/bin/crictl"
	containerdSocket = "/run/k3s/containerd/containerd.sock"
)

type kataMeasurement struct {
	Runtime, Cache, Pod, UID string
	CreatedAt, ReadyAt       time.Time
	ReadySeconds             float64
	Identity                 json.RawMessage
	Idle                     json.RawMessage `json:",omitempty"`
	Error                    string          `json:",omitempty"`
}

// Performance implements KATA-15; completeness is required, performance thresholds are not imposed.
func (s *KataRun) Performance(ctx context.Context) error {
	for _, pod := range s.pods {
		if err := s.deletePod(ctx, pod); err != nil {
			return err
		}
	}

	var results []kataMeasurement
	for _, cache := range []string{"cold", "warm"} {
		if cache == "warm" {
			if err := s.prewarm(ctx); err != nil {
				return err
			}
		}
		for round := range 5 {
			runtimes := []string{"runc", "kata"}
			if round%2 == 1 {
				slices.Reverse(runtimes)
			}
			for _, runtime := range runtimes {
				m, sampleErr := s.sample(ctx, runtime, cache, round)
				results = append(results, m)
				saveErr := s.evidence("performance-raw", results)
				if resultErr := errors.Join(sampleErr, saveErr); resultErr != nil {
					return resultErr
				}
			}
		}
	}

	summary, err := summarizeKata(results)
	if err != nil {
		return err
	}

	return s.evidence("performance-summary", summary)
}

func (s *KataRun) containerd(ctx context.Context, command string, args ...string) (string, error) {
	prefix := []string{"sudo", "-n", ctr, "--address", containerdSocket, "--namespace", "k8s.io", command}

	return s.node(ctx, s.eligibleIP, append(prefix, args...)...)
}

func kataCacheMatches(blobs, snapshots string, opts *KataConfig, warm bool) bool {
	content := strings.Fields(blobs)
	for _, blob := range opts.PerfBlobs {
		if slices.Contains(content, blob) != warm {
			return false
		}
	}

	return slices.Contains(strings.Fields(snapshots), opts.PerfChain) == warm
}

func (s *KataRun) cache(ctx context.Context, name string, warm bool) error {
	return pollKata(ctx, 2*time.Minute, func(ctx context.Context) error {
		content, err := s.containerd(ctx, "content", "ls", "-q")
		if err != nil {
			return fmt.Errorf("list containerd content for performance cache check: %w", err)
		}

		snapshots, listErr := s.containerd(ctx, "snapshots", "--snapshotter", "overlayfs", "ls")
		if listErr != nil {
			return fmt.Errorf("list overlayfs snapshots for performance cache check: %w", listErr)
		}
		keys, parseErr := kataSnapshotKeys(snapshots)
		if parseErr != nil {
			return parseErr
		}
		if !kataCacheMatches(content, strings.Join(keys, "\n"), s.opts, warm) {
			return fmt.Errorf("fixture cache does not match warm=%t; no cold/warm claim", warm)
		}

		return s.evidence(name+"-cache", map[string]any{"warm": warm, "content": content, "snapshots": snapshots})
	})
}

func kataSnapshotKeys(output string) ([]string, error) {
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if !slices.Equal(strings.Fields(lines[0]), []string{"KEY", "PARENT", "KIND"}) {
		return nil, errors.New("overlayfs snapshot listing has no recognized KEY/PARENT/KIND header")
	}

	var keys []string
	for _, line := range lines[1:] {
		columns := strings.Fields(line)
		if len(columns) < 2 || len(columns) > 3 {
			return nil, errors.New("malformed overlayfs snapshot row; cannot establish cache state")
		}
		keys = append(keys, columns[0])
	}

	return keys, nil
}

func (s *KataRun) removePerfImage(ctx context.Context) error {
	output, err := s.node(ctx, s.eligibleIP, "sudo", "-n", cri,
		"--runtime-endpoint", "unix://"+containerdSocket, "images", "-o", "json")
	if err != nil {
		return fmt.Errorf("list CRI images before cold performance sample: %w", err)
	}

	var listing struct {
		Images []struct {
			ID                    string
			RepoDigests, RepoTags []string
		}
	}
	if decodeErr := json.Unmarshal([]byte(output), &listing); decodeErr != nil {
		return fmt.Errorf("decode CRI images before cold performance sample: %w", decodeErr)
	}

	for i := range listing.Images {
		image := &listing.Images[i]
		if !slices.Contains(image.RepoDigests, s.opts.PerfImage) {
			continue
		}
		if len(image.RepoDigests) != 1 || len(image.RepoTags) != 0 {
			return errors.New("performance image has aliases; refusing shared image removal")
		}
		if usersErr := s.noImageUsers(ctx, image.ID); usersErr != nil {
			return usersErr
		}
		_, removeErr := s.node(ctx, s.eligibleIP, "sudo", "-n", cri,
			"--runtime-endpoint", "unix://"+containerdSocket, "rmi", s.opts.PerfImage)
		if removeErr != nil {
			return fmt.Errorf("remove dedicated performance image for cold sample: %w", removeErr)
		}

		return nil
	}

	return nil
}

func (s *KataRun) noImageUsers(ctx context.Context, id string) error {
	return pollKata(ctx, 3*time.Minute, func(ctx context.Context) error {
		output, err := s.node(ctx, s.eligibleIP, "sudo", "-n", cri,
			"--runtime-endpoint", "unix://"+containerdSocket, "ps", "-a", "-o", "json")
		if err != nil {
			return fmt.Errorf("list CRI containers using the performance image: %w", err)
		}

		var result struct {
			Containers []struct {
				ImageRef string
				Image    struct{ Image string }
			}
		}
		if decodeErr := json.Unmarshal([]byte(output), &result); decodeErr != nil {
			return fmt.Errorf("decode CRI containers using the performance image: %w", decodeErr)
		}

		for i := range result.Containers {
			c := &result.Containers[i]
			if c.ImageRef == id || c.ImageRef == s.opts.PerfImage || c.Image.Image == s.opts.PerfImage {
				return errors.New("performance image still has a CRI container; waiting for normal garbage collection")
			}
		}

		return nil
	})
}

func (s *KataRun) prewarm(ctx context.Context) error {
	for _, class := range []string{"", kataClass} {
		name := "prewarm-runc"
		if class != "" {
			name = "prewarm-kata"
		}

		fixture := s.idleWorkload(name, s.eligible, class, s.opts.PerfImage)
		pod, err := s.startFixture(ctx, &fixture, s.eligibleIP)
		if err != nil {
			return err
		}

		if cleanupErr := s.deletePod(ctx, pod); cleanupErr != nil {
			return cleanupErr
		}
	}

	return nil
}

func (s *KataRun) sample(ctx context.Context, runtime, cache string, round int) (m kataMeasurement, sampleErr error) {
	name := fmt.Sprintf("perf-%s-%s-%d", runtime, cache, round+1)
	m = kataMeasurement{
		Runtime: runtime,
		Cache:   cache,
		Pod:     name,
	}
	defer func() {
		if sampleErr != nil {
			sampleErr = fmt.Errorf("measure %s: %w", name, sampleErr)
			m.Error = sampleErr.Error()
		}
	}()
	if cache == "cold" {
		if removeErr := s.removePerfImage(ctx); removeErr != nil {
			return m, removeErr
		}
	}

	if cacheErr := s.cache(ctx, name, cache == "warm"); cacheErr != nil {
		return m, cacheErr
	}

	class := ""
	if runtime == "kata" {
		class = kataClass
	}

	fixture := s.idleWorkload(name, s.eligible, class, s.opts.PerfImage)
	if createErr := s.create(ctx, name, fixture); createErr != nil {
		return m, createErr
	}

	sampleErr = s.observePerformance(ctx, &m)

	return m, sampleErr
}

func (s *KataRun) observePerformance(ctx context.Context, m *kataMeasurement) error {
	pod, err := s.ready(ctx, s.ns, m.Pod)
	if err != nil {
		return err
	}

	if timingErr := recordKataStartup(&pod, m); timingErr != nil {
		return timingErr
	}

	record := &kataPod{
		Name:      m.Pod,
		Namespace: s.ns,
		Node:      pod.Spec.NodeName,
		IP:        s.eligibleIP,
		Runtime:   m.Runtime,
		UID:       m.UID,
	}
	record.Identity, err = s.identity(ctx, record)
	if err != nil {
		return err
	}

	m.Identity = record.Identity
	if snapshotterErr := kataOverlayfs(record.Identity); snapshotterErr != nil {
		return snapshotterErr
	}

	if m.Cache == "warm" {
		m.Idle, err = s.idle(ctx, record)
		if err != nil {
			return err
		}
	}

	return s.deletePod(ctx, record)
}

func recordKataStartup(pod *core.Pod, m *kataMeasurement) error {
	m.UID = string(pod.UID)
	m.CreatedAt = pod.CreationTimestamp.Time
	for _, condition := range pod.Status.Conditions {
		if condition.Type == core.PodReady && condition.Status == core.ConditionTrue {
			m.ReadyAt = condition.LastTransitionTime.Time
			break
		}
	}

	if m.UID == "" || m.CreatedAt.IsZero() || m.ReadyAt.IsZero() {
		return fmt.Errorf("pod %s: missing UID, creationTimestamp or Ready=True lastTransitionTime", pod.Name)
	}
	m.ReadySeconds = m.ReadyAt.Sub(m.CreatedAt).Seconds()
	if m.ReadySeconds < 0 {
		return fmt.Errorf("pod %s: Ready timestamp precedes creation by %s; check server/worker clock synchronization",
			pod.Name, m.CreatedAt.Sub(m.ReadyAt))
	}

	return nil
}

func kataOverlayfs(data json.RawMessage) error {
	var record kataIdentity
	if err := json.Unmarshal(data, &record); err != nil {
		return fmt.Errorf("decode runtime identity for snapshotter validation: %w", err)
	}

	if len(record.Containers) == 0 {
		return errors.New("no snapshotter evidence")
	}
	for _, c := range record.Containers {
		if c.Snapshotter != "overlayfs" {
			return fmt.Errorf("container %s: snapshotter %q; cold-cache procedure requires overlayfs", c.ID, c.Snapshotter)
		}
	}

	return nil
}

type kataIdleSample struct {
	IntervalSeconds float64 `json:"interval_seconds"`
	PSS             float64 `json:"pss_mib"`
	CPU             float64 `json:"cpu_seconds_per_second"`
	Ticks           uint64  `json:"cpu_ticks"`
	Identities      map[int]uint64
	observed        time.Time
}

func waitKata(ctx context.Context, duration time.Duration) error {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func (s *KataRun) idleObservation(ctx context.Context, ip string, ids []string) (kataIdleSample, error) {
	sample := kataIdleSample{Identities: make(map[int]uint64)}
	table, err := s.processes(ctx, ip)
	if err != nil {
		return sample, err
	}

	selected, err := selectKataProcesses(table, ids)
	if err != nil {
		return sample, fmt.Errorf("select idle sample processes for sandbox/containers %v: %w", ids, err)
	}

	sample.observed = time.Now()
	for i := range selected {
		process := &selected[i]
		output, readErr := s.node(ctx, ip, "sudo", "-n", "cat", fmt.Sprintf("/proc/%d/smaps_rollup", process.PID))
		if readErr != nil {
			return sample, fmt.Errorf("read idle memory for PID %d: %w", process.PID, readErr)
		}

		pss, parseErr := parseKataPSS(output)
		if parseErr != nil {
			return sample, fmt.Errorf("parse idle memory for PID %d: %w", process.PID, parseErr)
		}

		sample.PSS += pss / 1024
		sample.Ticks += process.CPUTicks
		sample.Identities[process.PID] = process.StartTicks
	}

	return sample, nil
}

func parseKataPSS(output string) (float64, error) {
	for _, line := range strings.Split(output, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 3 && fields[0] == "Pss:" && fields[2] == "kB" {
			n, err := strconv.ParseUint(fields[1], 10, 64)
			return float64(n), err
		}
	}

	return 0, errors.New("missing process PSS evidence")
}

func kataCPU(previous, current *kataIdleSample, hz float64) (float64, error) {
	if !maps.Equal(previous.Identities, current.Identities) || current.Ticks < previous.Ticks {
		return 0, errors.New("process set changed during idle sampling")
	}

	seconds := current.observed.Sub(previous.observed).Seconds()
	if seconds <= 0 || hz <= 0 {
		return 0, errors.New("invalid sampling interval or clock rate")
	}

	return float64(current.Ticks-previous.Ticks) / hz / seconds, nil
}

func (s *KataRun) idle(ctx context.Context, pod *kataPod) (data json.RawMessage, sampleErr error) {
	record, err := s.captureIdentity(ctx, pod)
	if err != nil {
		return nil, err
	}

	defer func() {
		var marshalErr error
		data, marshalErr = json.Marshal(record)
		sampleErr = errors.Join(sampleErr, marshalErr, s.evidence(pod.Name+"-idle", record))
	}()
	hz, err := s.hostClockRate(ctx, pod.IP)
	if err != nil {
		return nil, err
	}

	if settleErr := waitKata(ctx, 30*time.Second); settleErr != nil {
		return nil, fmt.Errorf("wait for idle settling of pod %s: %w", pod.Name, settleErr)
	}

	ids := append([]string{record.Sandbox}, record.ContainerIDs...)
	previous, err := s.idleObservation(ctx, pod.IP, ids)
	if err != nil {
		return nil, err
	}

	for observation := range 12 {
		if waitErr := waitKata(ctx, 5*time.Second); waitErr != nil {
			return nil, fmt.Errorf("wait for idle observation %d: %w", observation+1, waitErr)
		}
		current, observationErr := s.idleObservation(ctx, pod.IP, ids)
		if observationErr != nil {
			return nil, fmt.Errorf("collect idle observation %d: %w", observation+1, observationErr)
		}
		current.IntervalSeconds = current.observed.Sub(previous.observed).Seconds()
		cpu, cpuErr := kataCPU(&previous, &current, float64(hz))
		if cpuErr != nil {
			return nil, fmt.Errorf("calculate idle CPU at observation %d: %w", observation+1, cpuErr)
		}
		current.CPU = cpu
		record.Samples = append(record.Samples, current)
		previous = current
	}

	after, err := s.captureIdentity(ctx, pod)
	if err != nil {
		return nil, err
	}
	if after.Sandbox != record.Sandbox || !slices.Equal(after.ContainerIDs, record.ContainerIDs) {
		return nil, errors.New("sandbox/container changed during idle sampling")
	}

	return nil, nil
}

func (s *KataRun) hostClockRate(ctx context.Context, ip string) (uint64, error) {
	output, err := s.node(ctx, ip, "getconf", "CLK_TCK")
	if err != nil {
		return 0, fmt.Errorf("read host clock rate for idle CPU measurement: %w", err)
	}
	hz, err := strconv.ParseUint(strings.TrimSpace(output), 10, 64)
	if err != nil || hz == 0 {
		return 0, errors.New("invalid host clock rate")
	}

	return hz, nil
}

type kataDistribution struct {
	N             int
	Median, Worst float64
}

func describeKataSamples(values []float64) kataDistribution {
	sorted := slices.Clone(values)
	slices.Sort(sorted)
	n := len(sorted)
	median := sorted[n/2]
	if n%2 == 0 {
		median = (sorted[n/2-1] + median) / 2
	}

	return kataDistribution{
		N:      n,
		Median: median,
		Worst:  sorted[n-1],
	}
}

func summarizeKata(results []kataMeasurement) (map[string]any, error) {
	series := make(map[string][]float64)
	for i := range results {
		m := &results[i]
		if m.Error != "" || m.ReadySeconds < 0 || m.UID == "" || m.CreatedAt.IsZero() || m.ReadyAt.IsZero() {
			return nil, errors.New("incomplete performance series; failed attempts must not be discarded")
		}
		key := m.Runtime + "-" + m.Cache + "-ready_seconds"
		series[key] = append(series[key], m.ReadySeconds)
		if m.Cache == "warm" {
			if err := kataIdleSeries(series, m); err != nil {
				return nil, err
			}
		}
	}

	return summarizeKataSeries(series)
}

func kataIdleSeries(series map[string][]float64, m *kataMeasurement) error {
	var record kataIdentity
	if err := json.Unmarshal(m.Idle, &record); err != nil {
		return fmt.Errorf("decode idle observations for sample %s: %w", m.Pod, err)
	}
	if len(record.Samples) != 12 {
		return errors.New("idle sample must contain twelve five-second observations")
	}

	var pss, cpu []float64
	for _, sample := range record.Samples {
		pss = append(pss, sample.PSS)
		cpu = append(cpu, sample.CPU)
	}
	series[m.Runtime+"-idle_pss_mib"] = append(series[m.Runtime+"-idle_pss_mib"], describeKataSamples(pss).Median)
	series[m.Runtime+"-idle_cpu_cores"] = append(series[m.Runtime+"-idle_cpu_cores"], describeKataSamples(cpu).Median)

	return nil
}

func summarizeKataSeries(series map[string][]float64) (map[string]any, error) {
	out := map[string]any{
		"status":         "CHARACTERIZED",
		"thresholds":     "none; characterization, not an acceptance performance target",
		"startup_timing": "creationTimestamp to Ready=True lastTransitionTime; second resolution; requires synced clocks",
		"workload":       "sleep process; no HTTP service or probes; Ready denotes container startup, not HTTP readiness",
	}

	for _, suffix := range []string{"cold-ready_seconds", "warm-ready_seconds", "idle_pss_mib", "idle_cpu_cores"} {
		for _, runtime := range []string{"runc", "kata"} {
			key := runtime + "-" + suffix
			if len(series[key]) != 5 {
				return nil, errors.New("expected five observations for " + key)
			}
			out[key] = describeKataSamples(series[key])
		}
		kataMedian := describeKataSamples(series["kata-"+suffix]).Median
		out["kata-minus-runc-"+suffix] = kataMedian - describeKataSamples(series["runc-"+suffix]).Median
	}

	return out, nil
}
