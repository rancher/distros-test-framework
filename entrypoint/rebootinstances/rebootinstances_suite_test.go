package rebootinstances

import (
	"flag"
	"os"
	"strings"
	"sync"
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
	flags         = &customflag.ServiceFlag
	cluster       *driver.Cluster
	infraConfig   *driver.InfraConfig
	cfg           *config.Env
	reportSummary string
	reportErr     error
	err           error
)

func TestMain(m *testing.M) {
	flags = &customflag.ServiceFlag
	flag.Var(&flags.Destroy, "destroy", "Destroy cluster after test")
	flag.Parse()

	cfg, err = config.AddEnv()
	if err != nil {
		resources.LogLevel("error", "error adding env vars: %w\n", err)
		os.Exit(1)
	}

	validateEIP()

	setupClusterInfra()

	os.Exit(m.Run())
}

func TestRebootInstancesSuite(t *testing.T) {
	RegisterFailHandler(FailWithReport)
	RunSpecs(t, "Reboot Instances Test Suite")
}

func setupClusterInfra() {
	kubeconfig := os.Getenv("KUBE_CONFIG")
	if kubeconfig != "" {
		// gets a cluster from existing kubeconfig.
		cluster = legacy.KubeConfigCluster(kubeconfig)
		resources.LogLevel("info", "Using existing cluster from kubeconfig")

		return
	}

	// initial data load needed for provisioning coming from config env vars.
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

	resources.LogLevel("info", "Cluster provisioned successfully with %+v", cluster.Config)
}

var _ = ReportAfterSuite("Reboot Instances Test Suite", func(report Report) {
	// Add Qase reporting capabilities.
	if strings.ToLower(qaseReport) == "true" {
		qaseClient, err := qase.AddQase()
		Expect(err).ToNot(HaveOccurred(), "error adding qase")

		qaseClient.SpecReportTestResults(qaseClient.Ctx, cluster, &report, reportSummary)
	} else {
		resources.LogLevel("info", "Qase reporting is not enabled")
	}
})

var _ = AfterSuite(func() {
	reportSummary, reportErr = report.SummaryReportData(cluster, flags)
	if reportErr != nil {
		resources.LogLevel("error", "error getting report summary data: %v\n", reportErr)
	}

	if customflag.ServiceFlag.Destroy {
		status, err := provisioning.DestroyInfrastructure(
			infraConfig.ProvisionerModule, infraConfig.Product, infraConfig.Module)
		Expect(err).NotTo(HaveOccurred())
		Expect(status).To(Equal("cluster destroyed"))
	}

	cleanEIPs()
})

func validateEIP() {
	if os.Getenv("create_eip") == "" || os.Getenv("create_eip") != "true" {
		resources.LogLevel("error", "create_eip not set")
		os.Exit(1)
	}
}

// cleanEIPs release elastic ips from instances used on test.
func cleanEIPs() {
	release := os.Getenv("RELEASE_EIP")
	if release != "" && release == "false" {
		resources.LogLevel("info", "EIPs not released, being used to run test with kubeconfig")
	} else {
		awsDependencies, err := aws.AddClient(cluster)
		Expect(err).NotTo(HaveOccurred())

		eips := append(cluster.ServerIPs, cluster.AgentIPs...)

		var wg sync.WaitGroup
		for _, ip := range eips {
			ip := ip
			wg.Add(1)
			go func(ip string) {
				defer wg.Done()
				releaseEIPsErr := awsDependencies.ReleaseElasticIps(ip)
				if releaseEIPsErr != nil {
					resources.LogLevel("error", "on %w", releaseEIPsErr)
					return
				}
			}(ip)
		}
		wg.Wait()
	}
}

func FailWithReport(message string, callerSkip ...int) {
	Fail(message, callerSkip[0]+1)
}
