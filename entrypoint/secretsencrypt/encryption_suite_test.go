package secretsencrypt

import (
	"flag"
	"fmt"
	"os"
	"strings"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/rancher/distros-test-framework/config"
	"github.com/rancher/distros-test-framework/pkg/customflag"
	"github.com/rancher/distros-test-framework/pkg/k8s"
	"github.com/rancher/distros-test-framework/pkg/qase"
	"github.com/rancher/distros-test-framework/shared"
)

var (
	qaseReport = os.Getenv("REPORT_TO_QASE")
	kubeconfig string
	cluster    *shared.Cluster
	k8sClient  *k8s.Client
	cfg        *config.Product
	err        error
)

func TestMain(m *testing.M) {
	flag.Var(&customflag.ServiceFlag.Destroy, "destroy", "Destroy cluster after test")
	flag.Parse()

	cfg, err = config.AddEnv()
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

	k8sClient, err = k8s.AddClient()
	if err != nil {
		shared.LogLevel("error", "error adding k8s: %w\n", err)
		os.Exit(1)
	}

	os.Exit(m.Run())
}

func TestSecretsEncryptionSuite(t *testing.T) {
	RegisterFailHandler(FailWithReport)
	RunSpecs(t, "Secrets Encryption Test Suite")
}

var _ = BeforeSuite(func() {
	if cluster.Config.Product == "k3s" {
		Expect(os.Getenv("server_flags")).To(ContainSubstring("secrets-encryption:"),
			"ERROR: Add secrets-encryption:true to server_flags for this test")
	}

	version := os.Getenv(fmt.Sprintf("%s_version", cluster.Config.Product))

	var envErr error
	if strings.Contains(version, "1.27") || strings.Contains(version, "1.26") {
		envErr = os.Setenv("TEST_TYPE", "classic")
		Expect(envErr).To(BeNil(), fmt.Sprintf("error setting env var: %v\n", envErr))
	} else {
		envErr = os.Setenv("TEST_TYPE", "both")
		Expect(envErr).To(BeNil(), fmt.Sprintf("error setting env var: %v\n", envErr))
	}
})

var _ = ReportAfterSuite("Secrets Encryption Test Suite", func(report Report) {
	// Add Qase reporting capabilities.
	if strings.ToLower(qaseReport) == "true" {
		qaseClient, err := qase.AddQase()
		Expect(err).ToNot(HaveOccurred(), "error adding qase")

		qaseClient.ReportTestResults(qaseClient.Ctx, &report, cfg.InstallVersion)
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
