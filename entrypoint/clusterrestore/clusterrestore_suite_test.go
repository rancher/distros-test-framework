package clusterrestore

import (
	"flag"
	"os"
	"strings"
	"testing"

	"github.com/rancher/distros-test-framework/config"
	"github.com/rancher/distros-test-framework/internal/pkg/aws"
	"github.com/rancher/distros-test-framework/internal/pkg/customflag"
	"github.com/rancher/distros-test-framework/internal/pkg/qase"
	"github.com/rancher/distros-test-framework/internal/provisioning"
	"github.com/rancher/distros-test-framework/internal/provisioning/driver"
	"github.com/rancher/distros-test-framework/internal/provisioning/legacy"
	"github.com/rancher/distros-test-framework/internal/report"
	"github.com/rancher/distros-test-framework/internal/resources"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var (
	qaseReport    = os.Getenv("REPORT_TO_QASE")
	flags         *customflag.FlagConfig
	cfg           *config.Env
	cluster       *driver.Cluster
	infraConfig   *driver.InfraConfig
	reportSummary string
	reportErr     error
	err           error
	awsClient     *aws.Client
)

func TestMain(m *testing.M) {
	flags = &customflag.ServiceFlag
	flag.Var(&flags.Destroy, "destroy", "Destroy cluster after test")
	flag.Var(&flags.Channel, "channel", "channel to use on install")
	flag.StringVar(&flags.S3Flags.Bucket, "s3Bucket", "distrosqa", "s3 bucket to store snapshots")
	flag.StringVar(&flags.S3Flags.Folder, "s3Folder", "snapshots", "s3 folder to store snapshots")
	flag.Parse()

	cfg, err = config.AddEnv()
	if err != nil {
		resources.LogLevel("error", "error adding env vars: %w\n", err)
		os.Exit(1)
	}

	// checkUnsupportedFlags validates the hardening flags are not passed as they are not supported for now.
	checkUnsupportedFlags()

	setupClusterInfra()

	awsClient, err = aws.AddClient(cluster)
	if err != nil {
		resources.LogLevel("error", "error adding aws nodes: %s", err)
		os.Exit(1)
	}

	os.Exit(m.Run())
}

func TestClusterResetRestoreSuite(t *testing.T) {
	RegisterFailHandler(FailWithReport)
	RunSpecs(t, "Cluster Reset Restore Test Suite")
}

func setupClusterInfra() {
	kubeconfig := os.Getenv("KUBE_CONFIG")
	if kubeconfig != "" {
		// gets a cluster from existing kubeconfig.
		cluster = legacy.KubeConfigCluster(kubeconfig)
		resources.LogLevel("info", "Using existing cluster from kubeconfig")

		return
	}

	// initial data load needed for provisioning comming from config env vars.
	infraConfig = &driver.InfraConfig{
		Product:           cfg.Product,
		Module:            cfg.Module,
		ResourceName:      cfg.ResourceName,
		ProvisionerModule: cfg.ProvisionerModule,
		ProvisionerType:   cfg.ProvisionerType,
		InstallVersion:    cfg.InstallVersion,
		QAInfraProvider:   cfg.QAInfraProvider,
		NodeOS:            cfg.NodeOS,
		CNI:               cfg.CNI,
		Cluster: &driver.Cluster{
			Config: driver.Config{
				Arch:        cfg.Arch,
				ServerFlags: cfg.ServerFlags,
				WorkerFlags: cfg.WorkerFlags,
				Channel:     cfg.Channel,
			},
			SSH: driver.SSHConfig{
				User:        cfg.SSHUser,
				PrivKeyPath: cfg.SSHKeyPath,
				KeyName:     cfg.SSHKeyName,
			},
		},
	}

	cluster, err = provisioning.ProvisionInfrastructure(infraConfig)
	if err != nil {
		resources.LogLevel("error", "error provisioning infrastructure: %w\n", err)
		os.Exit(1)
	}

	resources.LogLevel("info", "Cluster provisioned successfully with %+v", cluster)
}

var _ = AfterSuite(func() {
	reportSummary, reportErr = report.SummaryReportData(cluster, flags)
	if reportErr != nil {
		resources.LogLevel("error", "error getting report summary data: %v\n", reportErr)
	}

	if customflag.ServiceFlag.Destroy {
		status, err := provisioning.DestroyInfrastructure(
			infraConfig.ProvisionerModule, infraConfig.Product, infraConfig.Module)
		Expect(err).ToNot(HaveOccurred())
		Expect(status).To(Equal("cluster destroyed"))
	}
})

var _ = ReportAfterSuite("Cluster Reset Restore Test Suite", func(report Report) {
	// AddClient Qase reporting capabilities.
	if strings.ToLower(qaseReport) == "true" {
		qaseClient, err := qase.AddQase()
		Expect(err).ToNot(HaveOccurred(), "error adding qase")

		qaseClient.SpecReportTestResults(qaseClient.Ctx, cluster, &report, reportSummary)
	} else {
		resources.LogLevel("info", "Qase reporting is not enabled")
	}
})

func FailWithReport(message string, callerSkip ...int) {
	Fail(message, callerSkip[0]+1)
}

func checkUnsupportedFlags() {
	serverFlags := os.Getenv("server_flags")

	if strings.Contains(serverFlags, "profile") ||
		strings.Contains(serverFlags, "selinux") ||
		strings.Contains(serverFlags, "protect-kernel-defaults") ||
		strings.Contains(serverFlags, "/etc/rancher/rke2/custom-psa.yaml") {
		resources.LogLevel("error", "hardening flags are not supported for now")
		os.Exit(1)
	}
}
