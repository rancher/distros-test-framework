package upgradecluster

import (
	"flag"
	"os"
	"testing"

	"github.com/rancher/distros-test-framework/config"
	"github.com/rancher/distros-test-framework/entrypoint"
	"github.com/rancher/distros-test-framework/internal/pkg/customflag"
	"github.com/rancher/distros-test-framework/internal/pkg/k8s"
	"github.com/rancher/distros-test-framework/internal/provisioning/driver"
	"github.com/rancher/distros-test-framework/internal/resources"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var (
	flags         *customflag.FlagConfig
	cluster       *driver.Cluster
	k8sClient     *k8s.Client
	cfg           *config.Env
	infraConfig   *driver.InfraConfig
	reportSummary string
	reportErr     error
	err           error
)

func TestMain(m *testing.M) {
	flags = &customflag.ServiceFlag
	flag.Var(&flags.InstallMode, "installVersionOrCommit", "Upgrade with version or commit")
	flag.Var(&flags.Channel, "channel", "channel to use on upgrade")
	flag.Var(&flags.Destroy, "destroy", "Destroy cluster after test")
	flag.Var(&flags.SUCUpgradeVersion, "sucUpgradeVersion", "Version for upgrading using SUC")
	if v := os.Getenv("UPGRADE_CHANNEL"); v != "" {
		_ = flags.Channel.Set(v)
	}
	flag.Parse()

	cfg, err = config.AddEnv()
	if err != nil {
		resources.LogLevel("error", "error adding env vars: %w\n", err)
		os.Exit(1)
	}
	cluster, infraConfig = entrypoint.SetupClusterInfra(cfg)
	k8sClient, err = k8s.AddClient()
	if err != nil {
		resources.LogLevel("error", "error adding k8s client: %w\n", err)
		os.Exit(1)
	}

	os.Exit(m.Run())
}

func TestClusterUpgradeSuite(t *testing.T) {
	RegisterFailHandler(entrypoint.FailWithReport)
	RunSpecs(t, "Upgrade Cluster Test Suite")
}

var _ = ReportAfterSuite("Upgrade Cluster Test Suite",
	entrypoint.ReportAfterSuite(&cluster, &reportSummary))

var _ = AfterSuite(entrypoint.AfterSuite(
	&cluster, &infraConfig, &reportSummary, &reportErr))
