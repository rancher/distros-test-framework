package deployrancher

import (
	"flag"
	"os"
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
	customflag.ValidateRancherChartsFlags()

	cfg, err = config.AddEnv()
	if err != nil {
		resources.LogLevel("error", "error adding env vars: %w\n", err)
		os.Exit(1)
	}

	// Validate rancher deployment vars before running the tests.
	entrypoint.ValidateRancherDeploy(cfg, flags)
	cluster, infraConfig = entrypoint.SetupClusterInfra(cfg)
	os.Exit(m.Run())
}

func TestRancherSuite(t *testing.T) {
	RegisterFailHandler(entrypoint.FailWithReport)
	RunSpecs(t, "Deploy Rancher Manager Test Suite")
}

var _ = ReportAfterSuite("Deploy Rancher Manager Test Suite",
	entrypoint.ReportAfterSuite(&cluster, &reportSummary))

var _ = AfterSuite(entrypoint.AfterSuite(
	&cluster, &infraConfig, &reportSummary, &reportErr))
