package airgap

import (
	"errors"
	"flag"
	"os"
	"strings"
	"testing"

	"github.com/rancher/distros-test-framework/config"
	"github.com/rancher/distros-test-framework/entrypoint"
	"github.com/rancher/distros-test-framework/internal/pkg/customflag"
	"github.com/rancher/distros-test-framework/internal/provisioning/driver"
	"github.com/rancher/distros-test-framework/internal/resources"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var (
	flags         *customflag.FlagConfig
	cluster       *driver.Cluster
	infraConfig   *driver.InfraConfig
	cfg           *config.Env
	reportSummary string
	reportErr     error
	err           error
)

const (
	registryUsernameEnv = "REGISTRY_USERNAME"
	registryPasswordEnv = "REGISTRY_PASSWORD" //nolint:gosec // env var name, not a credential
)

func TestMain(m *testing.M) {
	flags = &customflag.ServiceFlag
	flag.Var(&flags.Destroy, "destroy", "Destroy cluster after test")
	flag.StringVar(&flags.AirgapFlag.ImageRegistryUrl, "imageRegistryUrl", "", "image registry url to get the images from")
	flag.StringVar(&flags.AirgapFlag.RegistryUsername, "registryUsername", customflag.DefaultRegistryUsername, "private registry username")
	flag.StringVar(&flags.AirgapFlag.RegistryPassword, "registryPassword", customflag.DefaultRegistryPassword, "private registry password")
	flag.StringVar(&flags.AirgapFlag.TarballType, "tarballType", "", "artifact tarball type")
	flag.Parse()

	cfg, err = config.AddEnv()
	if err != nil {
		resources.LogLevel("error", "error adding env vars: %w\n", err)
		os.Exit(1)
	}

	// Every guard runs BEFORE any cloud resource exists.
	if err := loadRegistryCredentials(cfg.ProvisionerModule, flagsExplicitlySet()); err != nil {
		resources.LogLevel("error", "%v\n", err)
		os.Exit(1)
	}
	validateAirgap()

	cluster, infraConfig = entrypoint.SetupClusterInfra(cfg)

	os.Exit(m.Run())
}

// flagsExplicitlySet reports which CLI flags the caller actually passed.
func flagsExplicitlySet() map[string]bool {
	set := map[string]bool{}
	flag.Visit(func(f *flag.Flag) { set[f.Name] = true })

	return set
}

// loadRegistryCredentials takes REGISTRY_USERNAME/PASSWORD from the env when set
// (optional Jenkins credential); otherwise qainfra generates a per-run password.
func loadRegistryCredentials(provisioner string, explicit map[string]bool) error {
	if provisioner == "qainfra" && (explicit["registryUsername"] || explicit["registryPassword"]) {
		return errors.New("qainfra: -registryUsername/-registryPassword are not accepted; omit them " +
			"(built-in test pair) or set " + registryUsernameEnv + "/" + registryPasswordEnv + " from Jenkins credentials")
	}
	user, pass := os.Getenv(registryUsernameEnv), os.Getenv(registryPasswordEnv)
	if user != "" && pass != "" {
		flags.AirgapFlag.RegistryUsername = user
		flags.AirgapFlag.RegistryPassword = pass

		return nil
	}
	if provisioner == "qainfra" {
		// Blank the legacy defaults so the provisioner mints a per-run password.
		flags.AirgapFlag.RegistryUsername, flags.AirgapFlag.RegistryPassword = "", ""
		resources.LogLevel("info", "%s/%s not set; a per-run registry password will be generated\n",
			registryUsernameEnv, registryPasswordEnv)
	}

	return nil
}

// validateAirgap rejects unsupported scenarios before provisioning.
func validateAirgap() {
	serverFlags := cfg.ServerFlags
	if serverFlags == "" {
		serverFlags = os.Getenv("server_flags")
	}

	// This is required in .env file as param ENV_MODULE=airgap.
	if cfg.Module == "" || cfg.Module != "airgap" {
		resources.LogLevel("info", "ENV_MODULE is not set with value airgap. Setting the value...\n")
		cfg.Module = "airgap"
	}

	// qainfra: method (from the build tag), tarball type, version and Prime URL are
	// checked here, before SetupClusterInfra creates anything.
	if cfg.ProvisionerModule == "qainfra" {
		if err := customflag.ValidateAirgapInputs(cfg.Product, cfg.InstallVersion, &flags.AirgapFlag); err != nil {
			resources.LogLevel("error", "%v\n", err)
			os.Exit(1)
		}
	}

	// Legacy tfvars contract; qainfra provisions the bastion itself.
	if cfg.ProvisionerModule != "qainfra" && os.Getenv("no_of_bastion_nodes") == "0" {
		resources.LogLevel("error", "no_of_bastion_nodes is not set, should be 1\n")
		os.Exit(1)
	}

	installMode := os.Getenv("INSTALL_MODE")
	if installMode == "" {
		installMode = os.Getenv("install_mode")
	}
	if strings.Contains(installMode, "COMMIT") {
		resources.LogLevel("error", "airgap with commit installs is not supported\n")
		os.Exit(1)
	}

	if cfg.Product == "k3s" && strings.Contains(serverFlags, "protect") {
		resources.LogLevel("error", "airgap with hardened k3s setup is not supported\n")
		os.Exit(1)
	}

	if cfg.Product == "rke2" {
		validateRKE2Airgap(serverFlags)
	}
}

func validateRKE2Airgap(serverFlags string) {
	if strings.Contains(serverFlags, "profile") {
		resources.LogLevel("error", "airgap with hardened rke2 setup is not supported\n")
		os.Exit(1)
	}

	// Windows agents are a legacy-only topology; an unset count means none.
	winAgents := os.Getenv("no_of_windows_worker_nodes")
	if winAgents == "" || winAgents == "0" {
		return
	}
	cni := cfg.CNI
	if cni == "" {
		cni = serverFlags
	}
	cniSlice := []string{"calico", "flannel", "multus,calico", "multus,flannel"}
	if !resources.SliceContainsString(cniSlice, cni) {
		resources.LogLevel("error", "only calico or flannel cni or "+
			"multus,calico or multus,flannel is supported for Windows agent\n")
		resources.LogLevel("error", "found cni -> %v\n", cni)
		os.Exit(1)
	}
}

func TestAirgapClusterSuite(t *testing.T) {
	RegisterFailHandler(entrypoint.FailWithReport)
	RunSpecs(t, "Create Airgap Cluster Test Suite")
}

var _ = ReportAfterSuite("Create Airgap Cluster Test Suite",
	entrypoint.ReportAfterSuite(&cluster, &reportSummary))

var _ = AfterSuite(entrypoint.AfterSuite(
	&cluster, &infraConfig, &reportSummary, &reportErr))
