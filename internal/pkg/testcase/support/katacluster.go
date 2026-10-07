package support

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/go-cmp/cmp"

	apps "k8s.io/api/apps/v1"
	core "k8s.io/api/core/v1"
	nodeapi "k8s.io/api/node/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

const (
	kataClass    = "kata-qemu-runtime-rs"
	installLabel = "testing.rancher.io/kata-test"
)

func kataPtr[T any](v T) *T { return &v }

func (s *KataRun) workload(name, node, class, image string) core.Pod {
	labels := map[string]string{"kata-test-run": s.ns, "kata-test-backend": name}
	pod := core.Pod{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Pod",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: s.ns,
			Labels:    labels,
		},
		Spec: core.PodSpec{
			NodeSelector:                  map[string]string{"kubernetes.io/hostname": node},
			AutomountServiceAccountToken:  kataPtr(false),
			TerminationGracePeriodSeconds: kataPtr(int64(5)),
			SecurityContext: &core.PodSecurityContext{
				RunAsNonRoot:   kataPtr(true),
				RunAsUser:      kataPtr(int64(1000)),
				RunAsGroup:     kataPtr(int64(1000)),
				SeccompProfile: &core.SeccompProfile{Type: core.SeccompProfileTypeRuntimeDefault},
			},
			Containers: []core.Container{kataWebContainer(image, s.ns+"-"+name)},
		},
	}
	if class != "" {
		pod.Spec.RuntimeClassName = &class
	} else if s.opts.Security.HostLSM == "apparmor" {
		pod.Annotations = map[string]string{
			core.AppArmorBetaContainerAnnotationKeyPrefix + "web": core.AppArmorBetaProfileRuntimeDefault,
		}
	}

	return pod
}

// idleWorkload avoids per-connection children and recurring probes during process-set sampling.
func (s *KataRun) idleWorkload(name, node, class, image string) core.Pod {
	pod := s.workload(name, node, class, image)
	container := &pod.Spec.Containers[0]
	container.Command = []string{"/bin/sleep"}
	container.Args = []string{"86400"}
	container.Env = nil
	container.Ports = nil
	container.ReadinessProbe = nil

	return pod
}

func kataWebContainer(image, token string) core.Container {
	return core.Container{
		Name:            "web",
		Image:           image,
		ImagePullPolicy: core.PullIfNotPresent,
		Command:         []string{"/bin/sh", "-ec"},
		Args: []string{
			`mkdir -p /tmp/www; printf '%s\n' "$RESPONSE_TOKEN" > /tmp/www/index.html; exec httpd -f -p 8080 -h /tmp/www`,
		},
		Env: []core.EnvVar{{
			Name:  "RESPONSE_TOKEN",
			Value: token,
		}},
		Ports: []core.ContainerPort{{
			Name:          "http",
			ContainerPort: 8080,
		}},
		ReadinessProbe: &core.Probe{
			ProbeHandler: core.ProbeHandler{
				HTTPGet: &core.HTTPGetAction{
					Path: "/",
					Port: intstr.FromInt(8080),
				},
			},
			PeriodSeconds:  2,
			TimeoutSeconds: 1,
		},
		SecurityContext: &core.SecurityContext{
			AllowPrivilegeEscalation: kataPtr(false),
			Capabilities:             &core.Capabilities{Drop: []core.Capability{"ALL"}},
		},
		Resources: core.ResourceRequirements{
			Requests: core.ResourceList{
				core.ResourceCPU:    resource.MustParse("100m"),
				core.ResourceMemory: resource.MustParse("64Mi"),
			},
			Limits: core.ResourceList{core.ResourceMemory: resource.MustParse("256Mi")},
		},
	}
}

func (s *KataRun) service(name string) core.Service {
	return core.Service{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Service",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: s.ns,
		},
		Spec: core.ServiceSpec{
			Selector: map[string]string{"kata-test-run": s.ns, "kata-test-backend": name},
			Ports: []core.ServicePort{{
				Port:       8080,
				TargetPort: intstr.FromInt(8080),
			}},
		},
	}
}

func (s *KataRun) namespace(ctx context.Context, name, policy string) error {
	ns := core.Namespace{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Namespace",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
			Labels: map[string]string{
				"pod-security.kubernetes.io/enforce": policy,
				"kata-test-run":                      s.ns,
			},
		},
	}

	if err := s.create(ctx, name, ns); err != nil {
		return err
	}
	if name != "kata-system" {
		s.createdNamespaces = append(s.createdNamespaces, name)
	}

	return nil
}

func kataPodReady(pod *core.Pod) bool {
	if pod.Status.Phase != core.PodRunning || pod.DeletionTimestamp != nil {
		return false
	}

	for _, condition := range pod.Status.Conditions {
		if condition.Type == core.PodReady && condition.Status == core.ConditionTrue {
			return true
		}
	}

	return false
}

func (s *KataRun) ready(ctx context.Context, namespace, name string) (core.Pod, error) {
	var pod core.Pod
	readyErr := pollKata(ctx, 5*time.Minute, func(ctx context.Context) error {
		if getErr := s.get(ctx, &pod, "pod", name, "-n", namespace); getErr != nil {
			return getErr
		}
		if !kataPodReady(&pod) {
			return fmt.Errorf("pod %s is not Ready: %s", name, pod.Status.Phase)
		}

		return nil
	})

	return pod, readyErr
}

func (s *KataRun) startPod(ctx context.Context, name, node, ip, class, image string) (*kataPod, error) {
	pod := s.workload(name, node, class, image)

	return s.startFixture(ctx, &pod, ip)
}

func (s *KataRun) startFixture(ctx context.Context, pod *core.Pod, ip string) (*kataPod, error) {
	if err := s.create(ctx, pod.Name, pod); err != nil {
		return nil, err
	}
	ready, err := s.ready(ctx, pod.Namespace, pod.Name)
	if err != nil {
		return nil, err
	}

	if ready.Spec.NodeSelector["kubernetes.io/hostname"] != pod.Spec.NodeSelector["kubernetes.io/hostname"] ||
		ready.Spec.NodeName != s.nodeNames[ip] {
		return nil, fmt.Errorf("pod %s placement is not the requested node %s", pod.Name, s.nodeNames[ip])
	}

	runtime := "kata"
	if pod.Spec.RuntimeClassName == nil {
		runtime = "runc"
	}
	record := &kataPod{
		Name:      pod.Name,
		Namespace: pod.Namespace,
		Node:      ready.Spec.NodeName,
		IP:        ip,
		UID:       string(ready.UID),
		Runtime:   runtime,
	}
	record.Identity, err = s.identity(ctx, record)
	if err == nil {
		s.pods[pod.Name] = record
	}

	return record, err
}

func (s *KataRun) install(ctx context.Context) error {
	if err := s.discover(ctx); err != nil {
		return err
	}

	var classes nodeapi.RuntimeClassList
	if err := s.get(ctx, &classes, "runtimeclasses"); err != nil {
		return err
	}

	for i := range classes.Items {
		if strings.Contains(classes.Items[i].Handler, "kata") {
			return errors.New("cluster already has Kata; refusing to overwrite an installation")
		}
	}

	if err := s.namespace(ctx, s.ns, "restricted"); err != nil {
		return err
	}

	for _, fixture := range []struct{ name, node, ip string }{
		{"runc-local", s.eligible, s.eligibleIP}, {"runc-remote", s.ordinary, s.ordinaryIP},
	} {
		if _, err := s.startPod(ctx, fixture.name, fixture.node, fixture.ip, "", s.opts.Image); err != nil {
			return err
		}
		if err := s.create(ctx, fixture.name+"-service", s.service(fixture.name)); err != nil {
			return err
		}
	}
	if err := s.traffic(ctx, "runc-local", "runc-remote"); err != nil {
		return err
	}
	if err := s.traffic(ctx, "runc-remote", "runc-local"); err != nil {
		return err
	}
	if baselineErr := s.securityBaselineFixtures(ctx); baselineErr != nil {
		return baselineErr
	}
	if err := s.installChart(ctx); err != nil {
		return err
	}
	if securityErr := pollKata(ctx, 3*time.Minute, func(ctx context.Context) error {
		return s.checkSecurity(ctx, "after-install")
	}); securityErr != nil {
		return securityErr
	}

	return s.checkOrdinary(ctx)
}

func (s *KataRun) discover(ctx context.Context) error {
	s.eligibleIP, s.ordinaryIP = s.cluster.AgentIPs[0], s.cluster.AgentIPs[1]
	var nodes core.NodeList
	if nodesErr := s.get(ctx, &nodes, "nodes"); nodesErr != nil {
		return nodesErr
	}
	if evidenceErr := s.recordNodes(&nodes); evidenceErr != nil {
		return evidenceErr
	}

	for _, ip := range append([]string{s.cluster.ServerIPs[0]}, s.cluster.AgentIPs...) {
		label, hostnameErr := s.hostname(ctx, ip, &nodes)
		if hostnameErr != nil {
			return fmt.Errorf("discover provisioned node %s: %w", ip, hostnameErr)
		}
		switch ip {
		case s.eligibleIP:
			s.eligible = label
		case s.ordinaryIP:
			s.ordinary = label
		}
	}

	if s.eligible == s.ordinary {
		return errors.New("eligible and ordinary workers are the same node")
	}

	_, err := s.node(ctx, s.eligibleIP, "sudo", "-n", "sh", "-ec",
		"test -c /dev/kvm && test -r /dev/kvm && test -w /dev/kvm")
	if err != nil {
		return fmt.Errorf("verify readable/writable /dev/kvm on worker %s: %w", s.eligible, err)
	}

	logKata("nodes: Kata worker %s has a readable/writable /dev/kvm; ordinary worker %s stays on runc",
		s.eligible, s.ordinary)

	return nil
}

func (s *KataRun) hostname(ctx context.Context, ip string, nodes *core.NodeList) (string, error) {
	output, err := s.node(ctx, ip, "hostname")
	if err != nil {
		return "", fmt.Errorf("read hostname on provisioned node %s: %w", ip, err)
	}

	var matches []*core.Node
	for i := range nodes.Items {
		n := &nodes.Items[i]
		hostname := n.Labels["kubernetes.io/hostname"]
		if hostname == strings.TrimSpace(output) || n.Name == strings.TrimSpace(output) {
			if n.Status.NodeInfo.Architecture != "amd64" {
				return "", errors.New("kata supports amd64 only, not ARM")
			}
			matches = append(matches, n)
		}
	}
	if len(matches) != 1 || matches[0].Labels["kubernetes.io/hostname"] == "" {
		return "", errors.New("cannot uniquely map provisioned worker to its Kubernetes hostname label")
	}
	s.nodeNames[ip] = matches[0].Name

	return matches[0].Labels["kubernetes.io/hostname"], nil
}

func (s *KataRun) installChart(ctx context.Context) error {
	if err := s.namespace(ctx, "kata-system", "privileged"); err != nil {
		return err
	}

	_, err := s.kubectl(ctx, "label", "node", s.pods["runc-local"].Node, installLabel+"="+s.ns)
	if err != nil {
		return fmt.Errorf("label worker %s for Kata installation: %w", s.eligible, err)
	}

	chart := map[string]any{
		"apiVersion": "helm.cattle.io/v1", "kind": "HelmChart",
		"metadata": map[string]any{"name": "kata-deploy", "namespace": "kube-system"},
		"spec": map[string]any{
			"chart": "oci://ghcr.io/kata-containers/kata-deploy-charts/kata-deploy", "version": s.opts.ChartVersion,
			"targetNamespace": "kata-system", "createNamespace": false,
			"valuesContent": fmt.Sprintf(chartValues, installLabel, s.ns, s.kataSELinux),
		},
	}
	if createErr := s.create(ctx, "kata-chart", chart); createErr != nil {
		return createErr
	}

	if readyErr := pollKata(ctx, 12*time.Minute, s.chartReady); readyErr != nil {
		return fmt.Errorf("wait for Kata installer and RuntimeClasses: %w", readyErr)
	}

	logKata("kata-deploy chart %s installed: installer Ready only on %s; RuntimeClasses kata and %s use handler %s",
		s.opts.ChartVersion, s.eligible, kataClass, kataClass)

	return nil
}

const chartValues = `k8sDistribution: rke2
deploymentMode: daemonset
nodeSelector:
  %s: %s
runtimeClasses:
  createDefault: true
shims:
  disableAll: true
  qemu-runtime-rs:
    enabled: true
    supportedArches: [amd64]
node-feature-discovery:
  enabled: false
selinux:
  enabled: %t
`

func (s *KataRun) chartReady(ctx context.Context) error {
	var sets apps.DaemonSetList
	if err := s.get(ctx, &sets, "daemonsets", "-n", "kata-system"); err != nil {
		return err
	}
	if len(sets.Items) != 1 || sets.Items[0].Status.DesiredNumberScheduled != 1 || sets.Items[0].Status.NumberReady != 1 {
		return errors.New("Kata installer must be Ready on exactly one selected worker")
	}

	for _, name := range []string{kataClass, "kata"} {
		var class nodeapi.RuntimeClass
		if err := s.get(ctx, &class, "runtimeclass", name); err != nil {
			return err
		}
		if class.Handler != kataClass || class.Scheduling == nil || len(class.Scheduling.NodeSelector) == 0 {
			return fmt.Errorf("RuntimeClass %s has wrong handler or no scheduling boundary", name)
		}
		if err := s.evidence("runtimeclass-"+name, class); err != nil {
			return err
		}
	}

	return nil
}

func (s *KataRun) checkOrdinary(ctx context.Context) error {
	output, err := s.node(ctx, s.ordinaryIP, "sudo", "-n", "cat", "/var/lib/rancher/rke2/agent/etc/containerd/config.toml")
	if err != nil {
		return fmt.Errorf("read containerd configuration on excluded worker %s: %w", s.ordinary, err)
	}

	if strings.Contains(output, "kata") {
		return errors.New("excluded worker acquired Kata configuration")
	}
	for _, name := range []string{"runc-local", "runc-remote"} {
		if _, identityErr := s.identity(ctx, s.pods[name]); identityErr != nil {
			return identityErr
		}
	}

	if trafficErr := s.traffic(ctx, "runc-local", "runc-remote"); trafficErr != nil {
		return trafficErr
	}
	logKata("ordinary worker %s: no Kata in its containerd config; runc pods kept their runtime", s.ordinary)

	return nil
}

type kataSecurityState struct {
	Requested               kataSecurityConfig `json:"requested"`
	Role                    string             `json:"role"`
	OS                      string             `json:"os"`
	HostSELinux             string             `json:"host_selinux"`
	AppArmorEnabled         bool               `json:"apparmor_enabled"`
	AppArmorProfiles        map[string]string  `json:"apparmor_expected_profiles,omitempty"`
	AppArmorWorkloadProfile string             `json:"apparmor_workload_profile,omitempty"`
	CRISELinux              bool               `json:"cri_selinux"`
	CRIDisableAppArmor      bool               `json:"cri_disable_apparmor"`
	ProtectKernelDefaults   bool               `json:"protect_kernel_defaults"`
	SystemImages            map[string]string  `json:"system_images"`
}

func (s *KataRun) securityBaselineFixtures(ctx context.Context) error {
	if s.opts.Security.HostLSM == "apparmor" {
		ip := s.cluster.ServerIPs[0]
		pod := s.idleWorkload("runc-security", "", "", s.opts.Image)
		pod.Spec.NodeSelector = nil
		// This security witness is bound to the server; it does not test scheduler placement.
		pod.Spec.NodeName = s.nodeNames[ip]
		if _, fixtureErr := s.startFixture(ctx, &pod, ip); fixtureErr != nil {
			return fmt.Errorf("start AppArmor security witness on server: %w", fixtureErr)
		}
	}

	// Runtime-default profiles may be loaded lazily when the first runc workload starts.
	return s.checkSecurity(ctx, "baseline")
}

func (s *KataRun) checkSecurity(ctx context.Context, phase string) error {
	for _, ip := range append([]string{s.cluster.ServerIPs[0]}, s.cluster.AgentIPs...) {
		state, probeErr := s.probeSecurity(ctx, ip)
		evidenceErr := s.evidence("security-"+phase+"-"+strings.ReplaceAll(ip, ".", "-"), state)
		if observationErr := errors.Join(probeErr, evidenceErr); observationErr != nil {
			return fmt.Errorf("security %s on node %s: %w", phase, ip, observationErr)
		}

		if phase == "baseline" {
			s.securityBaseline[ip] = state
			if ip == s.eligibleIP {
				s.kataSELinux = state.HostSELinux == "Enforcing"
			}
			logKata("security baseline %s (%s): host SELinux %s, CRI SELinux %t, AppArmor %t, "+
				"protectKernelDefaults %t, profile %s, system images %v", ip, state.Role, state.HostSELinux,
				state.CRISELinux, state.AppArmorEnabled, state.ProtectKernelDefaults, state.Requested.Profile,
				state.SystemImages)

			continue
		}

		baseline, exists := s.securityBaseline[ip]
		if !exists {
			return fmt.Errorf("security %s on node %s: initial baseline is missing", phase, ip)
		}
		if diff := cmp.Diff(baseline, state); diff != "" {
			return fmt.Errorf("security %s on node %s differs from baseline (-baseline +current):\n%s", phase, ip, diff)
		}
	}

	if phase != "baseline" {
		logKata("security %s: all nodes match the baseline", phase)
	}

	return nil
}

func (s *KataRun) probeSecurity(ctx context.Context, ip string) (kataSecurityState, error) {
	state := kataSecurityState{Role: "agent"}
	server := ip == s.cluster.ServerIPs[0]
	if server {
		state.Role = "server"
	}

	output, err := s.node(ctx, ip, "sudo", "-n", "cat", "/etc/rancher/rke2/config.yaml")
	if err != nil {
		return state, fmt.Errorf("read RKE2 bootstrap configuration: %w", err)
	}
	state.Requested, err = validateKataSecurityFlags(output, server, &s.opts.Security)
	if err != nil {
		return state, fmt.Errorf("validate %s RKE2 configuration: %w", state.Role, err)
	}

	state.OS, err = s.node(ctx, ip, "cat", "/etc/os-release")
	if err != nil {
		return state, fmt.Errorf("read host OS release: %w", err)
	}

	if hostErr := s.checkHostLSM(ctx, ip, &state); hostErr != nil {
		return state, hostErr
	}

	if runtimeErr := s.checkRuntimeSecurity(ctx, ip, &state); runtimeErr != nil {
		return state, runtimeErr
	}

	state.SystemImages, err = s.systemImages(ctx, s.nodeNames[ip], server)

	return state, err
}

func (s *KataRun) checkHostLSM(ctx context.Context, ip string, state *kataSecurityState) error {
	mode, modeErr := s.node(ctx, ip, "sudo", "-n", "sh", "-ec",
		"if command -v getenforce >/dev/null; then getenforce; else echo Unavailable; fi")
	if modeErr != nil {
		return fmt.Errorf("read effective SELinux mode: %w", modeErr)
	}
	state.HostSELinux = strings.TrimSpace(mode)

	if s.opts.Security.HostLSM == "selinux" {
		if state.HostSELinux != "Enforcing" {
			return fmt.Errorf("approved SELinux baseline requires Enforcing, got %s", state.HostSELinux)
		}

		return nil
	}
	if state.HostSELinux != "Disabled" && state.HostSELinux != "Unavailable" {
		return fmt.Errorf("approved AppArmor-only baseline does not match SELinux mode %s", state.HostSELinux)
	}

	return s.checkAppArmor(ctx, ip, state)
}

const kataAppArmorProfile = "cri-containerd.apparmor.d"

func (s *KataRun) checkAppArmor(ctx context.Context, ip string, state *kataSecurityState) error {
	toolPath, toolErr := s.node(ctx, ip, "sudo", "-n", "sh", "-ec", "command -v aa-status || true")
	if toolErr != nil {
		return fmt.Errorf("check aa-status prerequisite in sudo PATH: %w", toolErr)
	}
	if strings.TrimSpace(toolPath) == "" {
		return errors.New("AppArmor prerequisite missing: aa-status not found in sudo PATH; install approved AppArmor tools")
	}

	output, apparmorErr := s.node(ctx, ip, "sudo", "-n", "sh", "-ec",
		"aa-status --enabled >/dev/null; cat /sys/module/apparmor/parameters/enabled")
	if apparmorErr != nil {
		return fmt.Errorf("verify AppArmor is enabled with aa-status and the kernel module: %w", apparmorErr)
	}
	state.AppArmorEnabled = strings.TrimSpace(output) == "Y"
	if !state.AppArmorEnabled {
		return errors.New("approved AppArmor baseline requires an enabled AppArmor kernel module")
	}

	output, statusErr := s.node(ctx, ip, "sudo", "-n", "aa-status", "--json")
	if statusErr != nil {
		return fmt.Errorf("read loaded AppArmor profiles and modes: %w", statusErr)
	}
	var status struct {
		Profiles map[string]string `json:"profiles"`
	}
	if decodeErr := json.Unmarshal([]byte(output), &status); decodeErr != nil {
		return fmt.Errorf("decode AppArmor profile inventory: %w", decodeErr)
	}
	state.AppArmorProfiles = map[string]string{kataAppArmorProfile: status.Profiles[kataAppArmorProfile]}
	if mode := state.AppArmorProfiles[kataAppArmorProfile]; mode != "enforce" {
		return fmt.Errorf("AppArmor profile %s must be loaded in enforce mode; observed %q", kataAppArmorProfile, mode)
	}

	return s.checkAppArmorWorkload(ctx, ip, state)
}

func (s *KataRun) checkAppArmorWorkload(ctx context.Context, ip string, state *kataSecurityState) error {
	name := "runc-security"
	switch ip {
	case s.eligibleIP:
		name = "runc-local"
	case s.ordinaryIP:
		name = "runc-remote"
	}
	probe := s.pods[name]
	if probe == nil {
		return fmt.Errorf("missing AppArmor security witness %s on node %s", name, ip)
	}

	var pod core.Pod
	if podErr := s.get(ctx, &pod, "pod", name, "-n", probe.Namespace); podErr != nil {
		return podErr
	}
	if !kataPodReady(&pod) || string(pod.UID) != probe.UID || pod.Spec.NodeName != s.nodeNames[ip] {
		return fmt.Errorf("AppArmor witness %s is not the expected Ready pod UID %s on node %s", name, probe.UID, ip)
	}
	if _, identityErr := s.identity(ctx, probe); identityErr != nil {
		return identityErr
	}

	output, profileErr := s.exec(ctx, probe, "cat", "/proc/1/attr/current")
	if profileErr != nil {
		return fmt.Errorf("read AppArmor confinement of pod %s PID 1: %w", name, profileErr)
	}
	state.AppArmorWorkloadProfile = strings.TrimSpace(output)
	expected := kataAppArmorProfile + " (enforce)"
	if state.AppArmorWorkloadProfile != expected {
		return fmt.Errorf("pod %s PID 1: AppArmor confinement %q; expected %q",
			name, state.AppArmorWorkloadProfile, expected)
	}

	return nil
}

func (s *KataRun) checkRuntimeSecurity(ctx context.Context, ip string, state *kataSecurityState) error {
	var info struct {
		Config struct {
			EnableSELinux   *bool `json:"enableSelinux"`
			DisableAppArmor *bool `json:"disableApparmor"`
		}
	}
	if infoErr := s.criJSON(ctx, ip, &info, "info"); infoErr != nil {
		return fmt.Errorf("read active CRI security configuration: %w", infoErr)
	}
	if info.Config.EnableSELinux == nil || info.Config.DisableAppArmor == nil {
		return errors.New("active CRI security configuration is missing SELinux/AppArmor fields")
	}
	state.CRISELinux, state.CRIDisableAppArmor = *info.Config.EnableSELinux, *info.Config.DisableAppArmor
	if s.opts.Security.HostLSM == "selinux" && !state.CRISELinux {
		return errors.New("SELinux baseline requires enableSelinux=true in active CRI configuration")
	}
	if s.opts.Security.HostLSM == "apparmor" && state.CRIDisableAppArmor {
		return errors.New("AppArmor baseline requires disableApparmor=false in active CRI configuration")
	}

	output, configErr := s.kubectl(ctx, "get", "--raw", "/api/v1/nodes/"+s.nodeNames[ip]+"/proxy/configz")
	if configErr != nil {
		return fmt.Errorf("read effective kubelet CIS configuration: %w", configErr)
	}
	var config struct {
		Kubelet struct {
			ProtectKernelDefaults *bool `json:"protectKernelDefaults"`
		} `json:"kubeletconfig"`
	}
	if decodeErr := json.Unmarshal([]byte(output), &config); decodeErr != nil {
		return fmt.Errorf("decode effective kubelet CIS configuration: %w", decodeErr)
	}
	state.ProtectKernelDefaults = config.Kubelet.ProtectKernelDefaults != nil && *config.Kubelet.ProtectKernelDefaults
	if !state.ProtectKernelDefaults {
		return errors.New("CIS requires protectKernelDefaults=true in effective kubelet configuration")
	}

	return nil
}

func (s *KataRun) systemImages(ctx context.Context, node string, server bool) (map[string]string, error) {
	var pods core.PodList
	if podsErr := s.get(ctx, &pods, "pods", "-n", "kube-system", "--field-selector", "spec.nodeName="+node); podsErr != nil {
		return nil, podsErr
	}
	components := []string{"kube-proxy"}
	if server {
		components = append(components, "kube-apiserver", "etcd")
	}

	images := make(map[string]string)
	for _, component := range components {
		for i := range pods.Items {
			pod := &pods.Items[i]
			if !strings.HasPrefix(pod.Name, component+"-") || !kataPodReady(pod) {
				continue
			}
			for j := range pod.Status.ContainerStatuses {
				container := &pod.Status.ContainerStatuses[j]
				if container.Name != component || container.State.Running == nil || !container.Ready {
					continue
				}
				if images[component] != "" || container.ImageID == "" ||
					!strings.HasPrefix(container.Image, s.opts.Security.Registry+"/") {
					return nil, fmt.Errorf("node %s: ambiguous %s image or incorrect registry", node, component)
				}
				images[component] = container.Image
			}
		}
		if images[component] == "" {
			return nil, fmt.Errorf("node %s: no running %s image to verify registry", node, component)
		}
	}

	return images, nil
}

func (s *KataRun) recordNodes(nodes *core.NodeList) error {
	var records []map[string]any
	for i := range nodes.Items {
		node := &nodes.Items[i]
		records = append(records, map[string]any{
			"name": node.Name, "hostname": node.Labels["kubernetes.io/hostname"],
			"system": node.Status.NodeInfo, "addresses": node.Status.Addresses,
		})
	}

	return s.evidence("nodes", records)
}
