package support

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/rancher/distros-test-framework/config"
	"github.com/rancher/distros-test-framework/internal/provisioning/driver"
	"github.com/rancher/distros-test-framework/internal/resources"
	"gopkg.in/yaml.v3"
)

var kataGAVersion = regexp.MustCompile(`^v\d+\.\d+\.\d+\+rke2r\d+$`)

// Fixed linux/amd64 BusyBox 1.37.0 fixtures: glibc for functional checks, musl for performance only.
// The performance layer is distinct; cold-cache checks refuse shared or retained content.
const (
	kataTestImage = "docker.io/library/busybox@sha256:66a6306db78bf2dbf3487f293aa8d6990d8e506fdffab9cc43fe422becf886e4"
	kataPerfImage = "docker.io/library/busybox@sha256:dc88b80842580654294472735332ec7c7789a4010ec9a68b69f9b88c19abede6"
)

type KataConfig struct {
	run                            *KataRun
	Image, PerfImage, ChartVersion string
	PerfBlobs                      []string
	PerfChain                      string
	Security                       kataSecurityPolicy
}

type kataSecurityPolicy struct {
	BuildType, Version, Channel, Registry, HostLSM string
}

type kataSecurityConfig struct {
	Prime       *bool  `yaml:"prime" json:"prime,omitempty"`
	SELinux     bool   `yaml:"selinux" json:"selinux_config"`
	Profile     string `yaml:"profile" json:"profile"`
	Registry    string `yaml:"system-default-registry" json:"system_default_registry"`
	Snapshotter string `yaml:"snapshotter" json:"snapshotter,omitempty"`
}

func LoadKataConfig(cfg *config.Env) (KataConfig, error) {
	if configErr := validateKataClusterConfig(cfg, os.Getenv); configErr != nil {
		return KataConfig{}, configErr
	}

	opts := KataConfig{
		Image:        kataTestImage,
		PerfImage:    kataPerfImage,
		ChartVersion: "4.2.0",
		PerfBlobs: []string{
			"sha256:dc88b80842580654294472735332ec7c7789a4010ec9a68b69f9b88c19abede6",
			"sha256:4b3c3c5fa628f793b84f999b8be30200339beb0a017e8546983af9766480029a",
			"sha256:f4255029a25bc97fe31c78724f2972e1ad162354163517d2bf202005a1c3c906",
		},
		PerfChain: "sha256:cdb90b3acfad4e427a5bb3dc46cef97d9b63942f7c572d385ac61eb545a7cef2",
	}
	security, securityErr := loadKataSecurity(cfg)
	if securityErr != nil {
		return opts, securityErr
	}
	opts.Security = security

	// Nested-capable workers are intrinsic to this suite, not a per-build choice.
	if nestedErr := os.Setenv("QA_INFRA_WORKER_NESTED", "true"); nestedErr != nil {
		return opts, fmt.Errorf("enable nested virtualization for Kata workers: %w", nestedErr)
	}

	return opts, nil
}

func validateKataClusterConfig(cfg *config.Env, env func(string) string) error {
	if cfg.Product != "rke2" || cfg.ProvisionerModule != "qainfra" || cfg.QAInfraProvider != "aws" {
		return errors.New("kata P0 requires RKE2 with PROVISIONER_MODULE=qainfra and QA_INFRA_PROVIDER=aws")
	}
	if cfg.Arch != "amd64" || env("KUBE_CONFIG") != "" {
		return errors.New("kata P0 requires amd64 and a new cluster (unset KUBE_CONFIG)")
	}

	for name, count := range map[string]string{"NO_OF_SERVER_NODES": "1", "NO_OF_WORKER_NODES": "2"} {
		if env(name) != count || (env(strings.ToLower(name)) != "" && env(strings.ToLower(name)) != count) {
			return fmt.Errorf("kata P0 requires %s=%s and no conflicting lowercase value", name, count)
		}
	}

	if env("split_roles") == "true" || env("SPLIT_ROLES") == "true" {
		return errors.New("kata P0 requires the simple one-server/two-worker topology")
	}

	return nil
}

func loadKataSecurity(cfg *config.Env) (kataSecurityPolicy, error) {
	policy := kataSecurityPolicy{
		BuildType: "ga",
		Version:   cfg.InstallVersion,
		Channel:   cfg.Channel,
		Registry:  "registry.rancher.com",
	}
	if policy.Version == "" || policy.Channel == "" {
		return policy, errors.New("Kata requires explicit INSTALL_VERSION and INSTALL_CHANNEL (or CHANNEL)")
	}
	if policy.Channel != "testing" && policy.Channel != "stable" && policy.Channel != "latest" {
		return policy, errors.New("Kata requires the testing, stable or latest install channel")
	}

	var serverConfig kataSecurityConfig
	if decodeErr := yaml.Unmarshal([]byte(resources.NormalizeString(cfg.ServerFlags)), &serverConfig); decodeErr != nil {
		return policy, fmt.Errorf("decode server security configuration: %w", decodeErr)
	}
	if policy.Channel == "testing" || !kataGAVersion.MatchString(policy.Version) ||
		strings.Contains(os.Getenv("INSTALL_MODE")+os.Getenv("install_mode"), "COMMIT") ||
		serverConfig.Registry == "stgregistry.suse.com" {
		policy.BuildType, policy.Registry = "staging", "stgregistry.suse.com"
	}

	// Approved homogeneous OS rows, never a fallback when SELinux enforcement is missing.
	switch cfg.NodeOS {
	case "rhel10", "sles16":
		policy.HostLSM = "selinux"
	case "ubuntu", "sles15":
		policy.HostLSM = "apparmor"
	default:
		return policy, fmt.Errorf("Kata has no approved host security baseline for NODE_OS=%q", cfg.NodeOS)
	}

	for name, flags := range map[string]string{"SERVER_FLAGS": cfg.ServerFlags, "WORKER_FLAGS": cfg.WorkerFlags} {
		if _, configErr := validateKataSecurityFlags(flags, name == "SERVER_FLAGS", &policy); configErr != nil {
			return policy, fmt.Errorf("%s: %w", name, configErr)
		}
	}

	return policy, nil
}

func validateKataSecurityFlags(flags string, server bool, policy *kataSecurityPolicy) (kataSecurityConfig, error) {
	var values kataSecurityConfig
	if decodeErr := yaml.Unmarshal([]byte(resources.NormalizeString(flags)), &values); decodeErr != nil {
		return values, fmt.Errorf("invalid RKE2 configuration: %w", decodeErr)
	}
	if !values.SELinux || values.Profile != "cis" {
		return values, errors.New("require selinux: true and profile: cis from first boot")
	}
	if server && (values.Prime == nil || !*values.Prime) {
		return values, errors.New("require prime: true on the server from first boot")
	}
	if !server && values.Prime != nil {
		return values, errors.New("prime is server-only; remove it from worker configuration")
	}
	if values.Registry != "" && values.Registry != policy.Registry {
		return values, fmt.Errorf("registry must match the %s baseline: %s", policy.BuildType, policy.Registry)
	}
	if server && policy.BuildType == "staging" && values.Registry != policy.Registry {
		return values, errors.New("RC/staging requires the approved system-default-registry explicitly on the server")
	}
	if values.Snapshotter != "" && values.Snapshotter != "overlayfs" {
		return values, errors.New("p0 cold-cache characterization currently requires overlayfs")
	}

	return values, nil
}

type kataCommand func(context.Context, string, ...string) (string, error)

func StartKata(ctx context.Context, cluster *driver.Cluster, cfg *KataConfig) error {
	var err error
	cfg.run, err = newKataRun(cluster, cfg, resources.KubeConfigFile)
	if err != nil {
		return err
	}

	resources.LogLevel("info", "Private Kata evidence: %s", cfg.run.dir)
	if installErr := cfg.run.install(ctx); installErr != nil {
		return fmt.Errorf("install Kata and validate the runc baseline: %w", installErr)
	}
	cfg.run.installed = true

	return nil
}

func RequireKata(cfg *KataConfig, pods ...string) (*KataRun, error) {
	if cfg.run == nil || !cfg.run.installed {
		return nil, errors.New("KATA-01 must initialize and install the fresh cluster fixtures first")
	}
	for _, name := range pods {
		if cfg.run.pods[name] == nil {
			return nil, fmt.Errorf("missing prerequisite fixture %s", name)
		}
	}

	return cfg.run, nil
}

func CleanupKata(cfg *KataConfig) error {
	if cfg.run == nil {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	return cfg.run.cleanup(ctx)
}

func kataShellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}

type KataRun struct {
	installed              bool
	opts                   *KataConfig
	cluster                *driver.Cluster
	kubeconfig, dir, ns    string
	eligible, ordinary     string
	eligibleIP, ordinaryIP string
	kataSELinux            bool
	run                    kataCommand
	pods                   map[string]*kataPod
	nodeNames              map[string]string
	securityBaseline       map[string]kataSecurityState
	createdNamespaces      []string
}

type kataPod struct {
	Name, Namespace, Node, IP, Runtime, UID string
	Identity                                json.RawMessage
}

// newKataRun creates private evidence for this suite's freshly provisioned cluster.
func newKataRun(cluster *driver.Cluster, opts *KataConfig, kubeconfig string) (*KataRun, error) {
	if cluster == nil || len(cluster.ServerIPs) != 1 || len(cluster.AgentIPs) != 2 || kubeconfig == "" {
		return nil, errors.New("kata requires the provisioned one-server/two-worker cluster and its kubeconfig")
	}

	root := os.Getenv("KATA_EVIDENCE_ROOT")
	if root != "" {
		// #nosec G703 -- Operator-selected artifact root; each run creates a fresh private subdirectory.
		if err := os.MkdirAll(root, 0o700); err != nil {
			return nil, fmt.Errorf("create Kata evidence root: %w", err)
		}
	}

	dir, err := os.MkdirTemp(root, "dtf-kata-")
	if err != nil {
		return nil, fmt.Errorf("create private Kata evidence directory: %w", err)
	}

	session := &KataRun{
		opts:             opts,
		cluster:          cluster,
		kubeconfig:       kubeconfig,
		dir:              dir,
		ns:               strings.ToLower(filepath.Base(dir)),
		pods:             make(map[string]*kataPod),
		nodeNames:        make(map[string]string),
		securityBaseline: make(map[string]kataSecurityState),
		run:              resources.RunHostArgsQuietContext,
	}
	if evidenceErr := session.evidence("pinned-inputs", opts); evidenceErr != nil {
		return nil, evidenceErr
	}

	return session, nil
}

func (s *KataRun) kubectl(ctx context.Context, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()

	return s.run(ctx, "kubectl", append([]string{"--kubeconfig", s.kubeconfig, "--request-timeout=30s"}, args...)...)
}

func (s *KataRun) node(ctx context.Context, ip string, args ...string) (string, error) {
	quoted := make([]string, len(args))
	for i, arg := range args {
		quoted[i] = kataShellQuote(arg)
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	ssh := s.cluster.SSH

	return s.run(ctx, "ssh", "-i", ssh.PrivKeyPath, "-o", "BatchMode=yes", "-o", "ConnectTimeout=10",
		"-o", "ServerAliveInterval=10", "-o", "ServerAliveCountMax=3", "-o", "StrictHostKeyChecking=accept-new",
		"-o", "UserKnownHostsFile="+filepath.Join(s.dir, "known_hosts"), "--", ssh.User+"@"+ip,
		strings.Join(quoted, " "))
}

func (s *KataRun) evidence(name string, value any) error {
	data, encodeErr := json.MarshalIndent(value, "", "  ")
	if encodeErr != nil {
		return fmt.Errorf("encode Kata evidence %s: %w", name, encodeErr)
	}

	if writeErr := os.WriteFile(filepath.Join(s.dir, name+".json"), data, 0o600); writeErr != nil {
		return fmt.Errorf("write Kata evidence %s: %w", name, writeErr)
	}

	return nil
}

func (s *KataRun) create(ctx context.Context, name string, object any) error {
	if err := s.evidence(name, object); err != nil {
		return err
	}
	_, err := s.kubectl(ctx, "create", "-f", filepath.Join(s.dir, name+".json"))
	if err != nil {
		return fmt.Errorf("create Kata fixture %s: %w", name, err)
	}

	return nil
}

func pollKata(ctx context.Context, limit time.Duration, check func(context.Context) error) error {
	ctx, cancel := context.WithTimeout(ctx, limit)
	defer cancel()
	var last error
	for {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("waiting: %w", errors.Join(err, last))
		}
		if last = check(ctx); last == nil {
			return nil
		}

		select {
		case <-ctx.Done():
		case <-time.After(time.Second):
		}
	}
}

func (s *KataRun) get(ctx context.Context, target any, args ...string) error {
	output, err := s.kubectl(ctx, append([]string{"get"}, append(args, "-o", "json")...)...)
	if err != nil {
		return fmt.Errorf("kubectl get %s: %w", strings.Join(args, " "), err)
	}

	if decodeErr := json.Unmarshal([]byte(output), target); decodeErr != nil {
		return fmt.Errorf("decode kubectl get %s: %w", strings.Join(args, " "), decodeErr)
	}

	return nil
}

// cleanup removes only fixtures owned by this session; shared AfterSuite destroys the infrastructure.
func (s *KataRun) cleanup(ctx context.Context) error {
	var errs []error
	for _, ns := range s.createdNamespaces {
		_, cleanupErr := s.kubectl(ctx, "delete", "namespace", ns, "--ignore-not-found", "--wait=false")
		if cleanupErr != nil {
			errs = append(errs, fmt.Errorf("delete Kata fixture namespace %s: %w", ns, cleanupErr))
		}
	}

	// Keep the HelmChart on preserved clusters; uninstall is KATA-08, outside P0.
	return errors.Join(errs...)
}
