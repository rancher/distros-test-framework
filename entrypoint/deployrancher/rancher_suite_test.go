package deployrancher

import (
	"flag"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/rancher/distros-test-framework/config"
	"github.com/rancher/distros-test-framework/factory"
	"github.com/rancher/distros-test-framework/pkg/productflag"
	"github.com/rancher/distros-test-framework/shared"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var cfg *config.Product

func TestMain(m *testing.M) {
	flag.Var(&productflag.ServiceFlag.Destroy, "destroy", "Destroy cluster after test")
	flag.StringVar(&productflag.ServiceFlag.RancherConfig.CertManagerVersion, "certManagerVersion", "v1.13.1", "cert-manager version that will be deployed on the cluster")
	flag.StringVar(&productflag.ServiceFlag.RancherConfig.RancherHelmVersion, "rancherHelmVersion", "v2.8.0", "rancher helm chart version to use to deploy rancher manager")
	flag.StringVar(&productflag.ServiceFlag.RancherConfig.RancherImageVersion, "rancherImageVersion", "v2.8.3", "rancher version that will be deployed on the cluster")
	flag.Parse()
	productflag.ValidateVersionFormat()

	var err error
	cfg, err = shared.EnvConfig()
	if err != nil {
		return
	}

	os.Exit(m.Run())
}

func TestRancherSuite(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Deploy Rancher Manager Test Suite")
}

var _ = BeforeSuite(func() {
	if err := config.SetEnv(shared.BasePath() + fmt.Sprintf("/config/%s.tfvars", cfg.Product)); err != nil {
		Expect(err).To(BeNil(), fmt.Sprintf("error loading tf vars: %v\n", err))
	}

	Expect(os.Getenv("create_lb")).To(Equal("true"), "Wrong value passed in tfvars for 'create_lb'")

	if cfg.Product == "rke2" &&
		strings.Contains(os.Getenv("server_flags"), "profile") {
		Expect(os.Getenv("optional_files")).NotTo(BeEmpty(), "Need to pass a value in tfvars for 'optional_files'")
		Expect(os.Getenv("server_flags")).To(ContainSubstring("pod-security-admission-config-file:"),
			"Wrong value passed in tfvars for 'server_flags'")
	}
})

var _ = AfterSuite(func() {
	g := GinkgoT()
	if productflag.ServiceFlag.Destroy {
		status, err := factory.DestroyCluster(g)
		Expect(err).NotTo(HaveOccurred())
		Expect(status).To(Equal("cluster destroyed"))
	}
})
