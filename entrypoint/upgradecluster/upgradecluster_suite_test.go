package upgradecluster

import (
	"flag"
	"os"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/rancher/distros-test-framework/config"
	"github.com/rancher/distros-test-framework/pkg/customflag"
	"github.com/rancher/distros-test-framework/pkg/qase"
	"github.com/rancher/distros-test-framework/shared"
)

var (
	qaseReport = os.Getenv("REPORT_TO_QASE")
	kubeconfig string
	flags      *customflag.FlagConfig
	cluster    *shared.Cluster
)

func TestMain(m *testing.M) {
	flags = &customflag.ServiceFlag
	flag.Var(&flags.InstallMode, "installVersionOrCommit", "Upgrade with version or commit")
	flag.Var(&flags.Channel, "channel", "channel to use on upgrade")
	flag.Var(&flags.Destroy, "destroy", "Destroy cluster after test")
	flag.Var(&flags.SUCUpgradeVersion, "sucUpgradeVersion", "Version for upgrading using SUC")
	flag.Parse()

	_, err := config.AddEnv()
	if err != nil {
		shared.LogLevel("error", "error adding env vars: %w\n", err)
		os.Exit(1)
	}

	kubeconfig = os.Getenv("KUBE_CONFIG")
	if kubeconfig == "" {
		// gets a cluster from terraform.
		cluster = shared.ClusterConfig()
	} else {
		// gets a cluster from kubeconfig.
		cluster = shared.KubeConfigCluster(kubeconfig)
	}

	os.Exit(m.Run())
}

func TestClusterUpgradeSuite(t *testing.T) {
	RegisterFailHandler(FailWithReport)
	RunSpecs(t, "Upgrade Cluster Test Suite")
}

var _ = ReportAfterSuite("Upgrade Cluster Test Suite", func(report Report) {
	// Add Qase reporting capabilities.
	if qaseReport == "true" {
		qaseClient, err := qase.AddQase()
		Expect(err).ToNot(HaveOccurred(), "error adding qase")

		qaseClient.ReportTestResults(qaseClient.Ctx, &report, flags.InstallMode.String())
	} else {
		shared.LogLevel("info", "Qase reporting is not enabled")
	}
})

var _ = AfterSuite(func() {
	if customflag.ServiceFlag.Destroy {
		status, err := shared.DestroyCluster()
		Expect(err).NotTo(HaveOccurred())
		Expect(status).To(Equal("cluster destroyed"))
	}
})

func FailWithReport(message string, callerSkip ...int) {
	Fail(message, callerSkip[0]+1)
}
