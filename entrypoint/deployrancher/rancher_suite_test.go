package deployrancher

import (
	"flag"
	"os"
	"strings"
	"testing"

	"github.com/rancher/distros-test-framework/config"
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

	setupClusterInfra()

	os.Exit(m.Run())
}

func TestRancherSuite(t *testing.T) {
	RegisterFailHandler(FailWithReport)
	RunSpecs(t, "Deploy Rancher Manager Test Suite")
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

func validateRancher() {
	if flags.Charts.Version == "" || flags.Charts.RepoName == "" || flags.Charts.RepoUrl == "" {
		resources.LogLevel("error", "charts version or repo name or url is not set as args\n")
		os.Exit(1)
	}

	if os.Getenv("create_lb") == "" || os.Getenv("create_lb") != "true" {
		resources.LogLevel("error", "create_lb is not set in tfvars\n")
		os.Exit(1)
	}

	if cfg.Product == "rke2" && strings.Contains(os.Getenv("server_flags"), "profile") {
		if os.Getenv("optional_files") == "" {
			resources.LogLevel("error", "optional_files is not set in tfvars\n")
			os.Exit(1)
		}
		if !strings.Contains(os.Getenv("server_flags"), "pod-security-admission-config-file") {
			resources.LogLevel("error", "pod-security-admission-config-file is not set in server_flags\n")
			os.Exit(1)
		}
	}

	// Install helm
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

var _ = ReportAfterSuite("Deploy Rancher Manager Test Suite", func(report Report) {
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
})

func FailWithReport(message string, callerSkip ...int) {
	Fail(message, callerSkip[0]+1)
}
