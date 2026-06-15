package deployrancher

import (
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
	cluster       *driver.Cluster
	infraConfig   *driver.InfraConfig
	flags         *customflag.FlagConfig
	cfg           *config.Env
	reportSummary string
	reportErr     error
	err           error
)

func TestMain(m *testing.M) {
	flags = &customflag.ServiceFlag
	flag.Var(&flags.Destroy, "destroy", "Destroy cluster after test")
	flag.StringVar(&flags.CertManager.Version, "certManagerVersion", "v1.16.1", "cert-manager version")
	flag.StringVar(&flags.Charts.Version, "chartsVersion", "", "rancher helm chart version")
	flag.StringVar(&flags.Charts.RepoName, "chartsRepoName", "", "rancher helm chart repo name")
	flag.StringVar(&flags.Charts.RepoUrl, "chartsRepoUrl", "", "rancher helm chart repo url")
	flag.StringVar(&flags.Charts.Args, "chartsArgs", "", "rancher helm additional args, comma separated")
	flag.Parse()

	customflag.ValidateVersionFormat()

	cfg, err = config.AddEnv()
	if err != nil {
		resources.LogLevel("error", "error adding env vars: %w\n", err)
		os.Exit(1)
	}

	// Validate rancher deployment vars before running the tests.
	validateRancher()
	cluster, infraConfig = entrypoint.SetupClusterInfra(cfg)
	os.Exit(m.Run())
}

func TestRancherSuite(t *testing.T) {
	RegisterFailHandler(entrypoint.FailWithReport)
	RunSpecs(t, "Deploy Rancher Manager Test Suite")
}

func validateRancher() {
	if flags.Charts.Version == "" || flags.Charts.RepoName == "" || flags.Charts.RepoUrl == "" {
		resources.LogLevel("error", "charts version or repo name or url is not set as args\n")
		os.Exit(1)
	}

	// no create_lb var to set there — the LB always exists on qa-infra for cps >=2.
	if cfg.ProvisionerModule != "qainfra" && os.Getenv("create_lb") != "true" {
		resources.LogLevel("error", "create_lb is not set in tfvars\n")
		os.Exit(1)
	}

	if cfg.Product == "rke2" && strings.Contains(cfg.ServerFlags, "profile") {
		// CIS rke2 needs the custom PSA file (OPTIONAL_FILES on qainfra, optional_files on legacy) or Rancher's pods are rejected.
		optionalFiles := os.Getenv("OPTIONAL_FILES")
		if optionalFiles == "" {
			optionalFiles = os.Getenv("optional_files")
		}
		if optionalFiles == "" {
			resources.LogLevel("error", "OPTIONAL_FILES is not set (needed to deliver the PSA config file)\n")
			os.Exit(1)
		}
		if !strings.Contains(cfg.ServerFlags, "pod-security-admission-config-file") {
			resources.LogLevel("error", "pod-security-admission-config-file is not set in server_flags\n")
			os.Exit(1)
		}
	}

	// Install helm.
	res, err := resources.InstallHelm()
	if err != nil {
		resources.LogLevel("debug", "helm install response:\n%v", res)
		resources.LogLevel("error", "Error while installing helm %v\n", err)
		os.Exit(1)
	}
	resources.LogLevel("debug", "helm version: %v", res)

	// Check chart repo
	res, err = resources.CheckHelmRepo(
		flags.Charts.RepoName,
		flags.Charts.RepoUrl,
		flags.Charts.Version)
	if err != nil {
		resources.LogLevel("debug", "helm repo check response:\n%v", res)
		resources.LogLevel("error", "Error while checking helm repo %v\n", err)
		os.Exit(1)
	}
	if res == "" {
		resources.LogLevel("error", "No version found in helm repo %v\n", flags.Charts.RepoName)
		os.Exit(1)
	}
}

var _ = ReportAfterSuite("Deploy Rancher Manager Test Suite",
	entrypoint.ReportAfterSuite(&cluster, &reportSummary))

var _ = AfterSuite(entrypoint.AfterSuite(
	&cluster, &infraConfig, &reportSummary, &reportErr))
