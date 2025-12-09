package sanity

import (
	"flag"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/rancher/distros-test-framework/config"
	"github.com/rancher/distros-test-framework/internal/provisioning"

	"github.com/rancher/distros-test-framework/internal/pkg/customflag"
	"github.com/rancher/distros-test-framework/internal/pkg/qase"
	"github.com/rancher/distros-test-framework/internal/provisioning/driver"
	"github.com/rancher/distros-test-framework/internal/provisioning/legacy"
	"github.com/rancher/distros-test-framework/internal/report"
	"github.com/rancher/distros-test-framework/internal/resources"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var (
	qaseReport       = os.Getenv("REPORT_TO_QASE")
	flags            *customflag.FlagConfig
	cluster          *driver.Cluster
	infraConfig      *driver.InfraConfig
	host             *driver.HostCluster
	SSH              *driver.SSHConfig
	cfg              *config.Env
	reportSummary    string
	reportErr        error
	err              error
	K3kNamespace     string
	StorageClassType string
	PersistenceType  string
	ServiceCIDR      string
	K3SVersion       string
	K3kTestCases     []driver.K3kClusterOptions
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

	setupClusterInfra()
	setupHostCluster()

	resources.LogLevel("info", "Host cluster setup successfully with %+v", host)
	os.Exit(m.Run())
}

func setupHostCluster() {
	if cfg.ServerIP == "" {
		if len(cluster.ServerIPs) == 0 {
			serverIP := os.Getenv("SERVER_IP")
			if serverIP == "" {
				resources.LogLevel("error", "SERVER_IP is not set")
				os.Exit(1)
			}
			cfg.ServerIP = serverIP
		} else {
			cfg.ServerIP = cluster.ServerIPs[0]
		}
	}
	if cfg.HostClusterType == "" {
		cfg.HostClusterType = cfg.Product
	}
	if cfg.K3kcliVersion == "" {
		k3kcliversion := os.Getenv("K3KCLI_VERSION")
		if k3kcliversion == "" {
			cfg.K3kcliVersion = getK3kVersion()
		} else {
			cfg.K3kcliVersion = k3kcliversion
		}
		resources.LogLevel("info", "K3kcli version set to: %s", cfg.K3kcliVersion)
	}
	// initial data load needed for provisioning with data from config env vars.
	host = &driver.HostCluster{
		ServerIP:        cfg.ServerIP,
		HostClusterType: cfg.HostClusterType,
		NodeOS:          cfg.NodeOS,
		// FQDN:			cfg.FQDN,
		KubeconfigPath: fmt.Sprintf("/etc/rancher/%s/%s.yaml", cfg.HostClusterType, cfg.HostClusterType),
		SSH: driver.SSHConfig{
			User:        cfg.SSHUser,
			PrivKeyPath: cfg.SSHKeyPath,
			KeyName:     cfg.SSHKeyName,
		},
		K3kcliVersion: cfg.K3kcliVersion,
	}
	K3kNamespace = os.Getenv("K3K_NAMESPACE")
	if K3kNamespace == "" {
		K3kNamespace = "k3k-system"
	}
	resources.LogLevel("info", "K3k Namespace set to: %s", K3kNamespace)

	StorageClassType = os.Getenv("STORAGECLASS_TYPE")
	if StorageClassType == "" {
		StorageClassType = "local-path"
	}
	resources.LogLevel("debug", "StorageClass Type set to: %s", StorageClassType)

	ServiceCIDR = os.Getenv("SERVICE_CIDR")
	if ServiceCIDR == "" {
		ServiceCIDR = "10.43.0.0/16"
	}
	resources.LogLevel("debug", "Service CIDR set to: %s", ServiceCIDR)

	K3SVersion = os.Getenv("K3K_K3S_VERSION")
	if K3SVersion == "" {
		K3SVersion = "v1.33.5+k3s1"
	}
	resources.LogLevel("debug", "K3S Version set to: %s", K3SVersion)

	resources.LogLevel("info", "Cluster provisioned successfully with %+v", cluster)
}

func getK3kVersion() string {
	// get the latest k3kcli version from github releases.
	k3kVersion, err := resources.GetLatestReleaseTag("rancher", "k3k")
	if err != nil {
		resources.LogLevel("error", "error getting latest k3kcli version: %v\n", err)
		os.Exit(1)
	}
	return k3kVersion
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

func TestK3kClusterSanitySuite(t *testing.T) {
	RegisterFailHandler(FailWithReport)
	RunSpecs(t, "K3k Cluster Sanity Test Suite")
}

var _ = ReportAfterSuite("Validate Cluster Test Suite", func(report Report) {
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
		status, err := provisioning.DestroyInfrastructure(infraConfig.ProvisionerModule, infraConfig.Product, infraConfig.Module)
		Expect(err).ToNot(HaveOccurred())
		Expect(status).To(Equal("cluster destroyed"))
	}
})

func FailWithReport(message string, callerSkip ...int) {
	Fail(message, callerSkip[0]+1)
}
