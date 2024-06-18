package restartservice

import (
	"flag"
	"os"
	"testing"

	"github.com/rancher/distros-test-framework/factory"
	"github.com/rancher/distros-test-framework/pkg/customflag"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var cluster *factory.Cluster

func TestMain(m *testing.M) {
	flag.Var(&customflag.ServiceFlag.ClusterConfig.Destroy, "destroy", "Destroy cluster after test")
	flag.Parse()

	cluster = factory.ClusterConfig()

	os.Exit(m.Run())
}

func TestRestartServiceSuite(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Restart Service Test Suite")
}

var _ = AfterSuite(func() {
	if customflag.ServiceFlag.ClusterConfig.Destroy {
		status, err := factory.DestroyCluster()
		Expect(err).NotTo(HaveOccurred())
		Expect(status).To(Equal("cluster destroyed"))
	}
})
