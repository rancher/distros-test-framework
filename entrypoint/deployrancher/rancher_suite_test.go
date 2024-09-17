package deployrancher

import (
	"flag"
	"os"
	"strings"
	"testing"

	"github.com/rancher/distros-test-framework/config"
	"github.com/rancher/distros-test-framework/pkg/customflag"
	"github.com/rancher/distros-test-framework/shared"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var (
	cluster    *shared.Cluster
	flags      *customflag.FlagConfig
	kubeconfig string
	cfg        *config.Product
	err        error
)

func TestMain(m *testing.M) {
	flags = &customflag.ServiceFlag
	flag.Var(&flags.Destroy, "destroy", "Destroy cluster after test")
	flag.StringVar(&flags.RancherConfig.CertManagerVersion, "certManagerVersion", "v1.11.0", "cert-manager version")
	flag.StringVar(&flags.HelmCharts.Version, "chartsVersion", "v2.8.0", "rancher helm chart version")
	flag.StringVar(&flags.HelmCharts.RepoName, "chartsRepoName", "rancher-latest", "rancher helm repo name")
	flag.StringVar(&flags.HelmCharts.RepoUrl, "chartsRepoUrl", "https://releases.rancher.com/server-charts/latest", "rancher helm repo url")
	flag.StringVar(&flags.HelmCharts.Args, "chartsArgs", "", "rancher helm additional args, comma separated")
	flag.StringVar(&flags.RancherConfig.RancherVersion, "rancherVersion", "v2.8.0", "rancher version that will be deployed on the cluster")
	flag.Parse()

	customflag.ValidateVersionFormat()

	cfg, err = config.AddEnv()
	if err != nil {
		shared.LogLevel("error", "error adding env vars: %w\n", err)
		os.Exit(1)
	}

	// Validate rancher deployment vars before running the tests.
	validateRancher()

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

func TestRancherSuite(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Deploy Rancher Manager Test Suite")
}

func validateRancher() {
	if os.Getenv("create_lb") == "" || os.Getenv("create_lb") != "true" {
		shared.LogLevel("error", "create_lb is not set in tfvars\n")
		os.Exit(1)
	}

	if cfg.Product == "rke2" && strings.Contains(os.Getenv("server_flags"), "profile") {
		if os.Getenv("optional_files") == "" {
			shared.LogLevel("error", "optional_files is not set in tfvars\n")
			os.Exit(1)
		}
		if !strings.Contains(os.Getenv("server_flags"), "pod-security-admission-config-file") {
			shared.LogLevel("error", "pod-security-admission-config-file is not set in server_flags\n")
			os.Exit(1)
		}
	}

	// Check helm chart repo
	res, err := shared.CheckHelmRepo(
		flags.HelmCharts.RepoName,
		flags.HelmCharts.RepoUrl,
		flags.HelmCharts.Version)
	if err != nil {
		shared.LogLevel("error", "Error while checking helm repo %v\n", err)
		os.Exit(1)
	}
	if res == "" {
		shared.LogLevel("error", "No version found in helm repo %v\n", flags.HelmCharts.RepoName)
		os.Exit(1)
	}
}

var _ = AfterSuite(func() {
	if customflag.ServiceFlag.Destroy {
		status, err := shared.DestroyCluster()
		Expect(err).NotTo(HaveOccurred())
		Expect(status).To(Equal("cluster destroyed"))
	}
})
