package clusterresetrestore

import (
	"flag"
	"os"
	"testing"

	"github.com/rancher/distros-test-framework/config"
	"github.com/rancher/distros-test-framework/pkg/customflag"
	"github.com/rancher/distros-test-framework/shared"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var (
	cfg        *config.Product
	flags      *customflag.FlagConfig
	kubeconfig string
	cluster    *shared.Cluster
)

func TestMain(m *testing.M) {
	var err error
	flags = &customflag.ServiceFlag
	flag.Var(&flags.Destroy, "destroy", "Destroy cluster after test")
	flag.Parse()

	_, err = config.AddEnv()
	if err != nil {
		shared.LogLevel("error", "error adding env vars: %w\n", err)
		os.Exit(1)
	}

	os.Exit(m.Run())
}

func TestClusterResetRestoreSuite(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Cluster Reset Restore Test Suite")
}

var _ = AfterSuite(func() {
	if customflag.ServiceFlag.Destroy {
		status, err := shared.DestroyCluster()
		Expect(err).NotTo(HaveOccurred())
		Expect(status).To(Equal("cluster destroyed"))
	}
})
