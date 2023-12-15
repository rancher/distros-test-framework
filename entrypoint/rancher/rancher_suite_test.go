package rancher

import (
	"flag"
	"os"
	"strings"
	"testing"

	"github.com/rancher/distros-test-framework/factory"
	"github.com/rancher/distros-test-framework/pkg/customflag"
	"github.com/rancher/distros-test-framework/pkg/testcase"
	"github.com/rancher/distros-test-framework/shared"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestMain(m *testing.M) {
	var err error
	flag.Var(&customflag.ServiceFlag.ClusterConfig.Destroy, "destroy", "Destroy cluster after test")
	flag.StringVar(&customflag.ServiceFlag.ExternalFlag.CertManagerVersion, "certManagerVersion", "v1.11.0", "cert-manager version that will be deployed on the cluster")
	flag.StringVar(&customflag.ServiceFlag.ExternalFlag.RancherHelmVersion, "rancherHelmVersion", "v2.8.0", "rancher helm chart version to use to deploy rancher manager")
	flag.StringVar(&customflag.ServiceFlag.ExternalFlag.RancherImageVersion, "rancherImageVersion", "v2.8.0", "rancher version that will be deployed on the cluster")
	flag.Parse()

	_, err = shared.EnvConfig()
	if err != nil {
		return
	}

	os.Exit(m.Run())
}

func TestRancherSuite(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Create Cluster Test Suite")
}

var _ = BeforeSuite(func() {
	testcase.TestConfigVarVal(GinkgoT(), "create_lb", "true")
	product, err := shared.Product()
	Expect(err).NotTo(HaveOccurred(), "error getting product: %v", err)
	if product == "rke2" && strings.Contains(factory.GetConfigVarValue(GinkgoT(), "server_flags"), "profile") {
		testcase.TestConfigVarSet(GinkgoT(), "optional_files")
		testcase.TestConfigVarVal(GinkgoT(), "server_flags", "pod-security-admission-config-file:")
	}

})

var _ = AfterSuite(func() {
	g := GinkgoT()
	if customflag.ServiceFlag.ClusterConfig.Destroy {
		status, err := factory.DestroyCluster(g)
		Expect(err).NotTo(HaveOccurred())
		Expect(status).To(Equal("cluster destroyed"))
	}
})
