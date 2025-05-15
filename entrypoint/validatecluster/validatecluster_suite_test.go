package validatecluster

import (
	"flag"
	"os"
	"strings"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/rancher/distros-test-framework/config"
	"github.com/rancher/distros-test-framework/pkg/customflag"
	"github.com/rancher/distros-test-framework/pkg/qase"
	"github.com/rancher/distros-test-framework/pkg/testcase"
	"github.com/rancher/distros-test-framework/shared"
)

var (
	qaseReport = os.Getenv("REPORT_TO_QASE")
	kubeconfig string
	cluster    *shared.Cluster
	cfg        *config.Env
	err        error
)

func TestMain(m *testing.M) {
	flag.Var(&customflag.ServiceFlag.Destroy, "destroy", "Destroy cluster after test")
	flag.Var(&customflag.ServiceFlag.SelinuxTest, "selinux", "Run selinux test")
	flag.Parse()

	cfg, err = config.AddEnv()
	if err != nil {
		shared.LogLevel("error", "error adding env vars: %w\n", err)
		os.Exit(1)
	}

	kubeconfig = os.Getenv("KUBE_CONFIG")
	if kubeconfig == "" {
		// gets a cluster from terraform.
		cluster = shared.ClusterConfig(cfg)
	} else {
		// gets a cluster from kubeconfig.
		cluster = shared.KubeConfigCluster(kubeconfig)
	}

	os.Exit(m.Run())
}

func TestValidateClusterSuite(t *testing.T) {
	RegisterFailHandler(FailWithReport)
	RunSpecs(t, "Validate Cluster Test Suite")
}

var _ = ReportAfterSuite("Validate Cluster Test Suite", func(report Report) {
	// Add Qase reporting capabilities.
	if strings.ToLower(qaseReport) == "true" {
		qaseClient, err := qase.AddQase()
		Expect(err).ToNot(HaveOccurred(), "error adding qase")

		qaseClient.SpecReportTestResults(qaseClient.Ctx, &report, cfg.InstallVersion)
	} else {
		shared.LogLevel("info", "Qase reporting is not enabled")
	}
})

var _ = AfterSuite(func() {
	if customflag.ServiceFlag.Destroy {
		shared.LogLevel("info", "Running kill all and uninstall tests before destroying the cluster")
		testcase.TestKillAllUninstall(cluster, cfg)

		if customflag.ServiceFlag.SelinuxTest {
			shared.LogLevel("info", "Running selinux test post killall before cluster destroy with uninstall false")
			if strings.Contains(os.Getenv("server_flags"), "selinux: true") {
				testcase.TestUninstallPolicy(cluster, false)
			}
		}

		status, err := shared.DestroyCluster(cfg)
		Expect(err).NotTo(HaveOccurred())
		Expect(status).To(Equal("cluster destroyed"))
	}
})

func FailWithReport(message string, callerSkip ...int) {
	Fail(message, callerSkip[0]+1)
}
