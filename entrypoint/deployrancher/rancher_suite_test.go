package deployrancher

import (
	"flag"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/rancher/distros-test-framework/config"
	"github.com/rancher/distros-test-framework/factory"
	"github.com/rancher/distros-test-framework/pkg/customflag"
	"github.com/rancher/distros-test-framework/shared"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var (
	cluster *factory.Cluster
	flags   *customflag.FlagConfig
)

func TestMain(m *testing.M) {
	flags = &customflag.ServiceFlag
	flag.Var(&flags.ClusterConfig.Destroy, "destroy", "Destroy cluster after test")
	flag.StringVar(&flags.ExternalFlag.CertManagerVersion, "certManagerVersion", "v1.11.0", "cert-manager version")
	flag.StringVar(&flags.ExternalFlag.HelmChartsFlag.Version, "chartsVersion", "v2.8.0", "rancher helm chart version")
	flag.StringVar(&flags.ExternalFlag.HelmChartsFlag.RepoName, "chartsRepoName", "rancher-latest", "rancher helm repo name")
	flag.StringVar(&flags.ExternalFlag.HelmChartsFlag.RepoUrl, "chartsRepoUrl", "https://releases.rancher.com/server-charts/latest", "rancher helm repo url")
	flag.StringVar(&flags.ExternalFlag.HelmChartsFlag.Args, "chartsArgs", "", "rancher helm additional args, comma separated")
	flag.StringVar(&flags.ExternalFlag.RancherVersion, "rancherVersion", "v2.8.0", "rancher version that will be deployed on the cluster")
	flag.Parse()

	cluster = factory.ClusterConfig()

	os.Exit(m.Run())
}

func TestRancherSuite(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Deploy Rancher Manager Test Suite")
}

var _ = BeforeSuite(func() {
	if err := config.SetEnv(shared.BasePath() + fmt.Sprintf("/config/%s.tfvars", cluster.Config.Product)); err != nil {
		Expect(err).To(BeNil(), fmt.Sprintf("error loading tf vars: %v\n", err))
	}

	Expect(os.Getenv("create_lb")).To(Equal("true"), "Wrong value passed in tfvars for 'create_lb'")

	if cluster.Config.Product == "rke2" &&
		strings.Contains(os.Getenv("server_flags"), "profile") {
		Expect(os.Getenv("optional_files")).NotTo(BeEmpty(), "Need to pass a value in tfvars for 'optional_files'")
		Expect(os.Getenv("server_flags")).To(ContainSubstring("pod-security-admission-config-file:"),
			"Wrong value passed in tfvars for 'server_flags'")
	}

	// Check helm chart repo
	res, err := shared.CheckHelmRepo(
		flags.ExternalFlag.HelmChartsFlag.RepoName,
		flags.ExternalFlag.HelmChartsFlag.RepoUrl,
		flags.ExternalFlag.HelmChartsFlag.Version)
	Expect(err).To(BeNil(), "Error while checking helm repo ", err)
	Expect(res).ToNot(BeEmpty(), "No version found in helm repo ", flags.ExternalFlag.HelmChartsFlag.RepoName)
})

var _ = AfterSuite(func() {
	if customflag.ServiceFlag.ClusterConfig.Destroy {
		status, err := factory.DestroyCluster()
		Expect(err).NotTo(HaveOccurred())
		Expect(status).To(Equal("cluster destroyed"))
	}
})
