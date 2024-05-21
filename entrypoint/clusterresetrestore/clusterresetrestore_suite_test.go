package clusterreset

import (
	"flag"
	"os"
	"testing"

	"github.com/rancher/distros-test-framework/config"
	"github.com/rancher/distros-test-framework/factory"
	"github.com/rancher/distros-test-framework/pkg/customflag"
	"github.com/rancher/distros-test-framework/shared"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var (
	cfg   *config.Product
	flags *customflag.FlagConfig
)

func TestMain(m *testing.M) {
	var err error
	flags = &customflag.ServiceFlag
	flag.Var(&flags.ClusterConfig.Destroy, "destroy", "Destroy cluster after test")
	flag.StringVar(&flags.ExternalFlag.S3Bucket, "s3Bucket", "", "s3 Bucket name")
	flag.StringVar(&flags.ExternalFlag.S3Folder, "s3Folder", "", "s3 Folder name")
	flag.StringVar(&flags.ExternalFlag.S3Region, "s3Region", "", "s3 Region")
	flag.Parse()

	cfg, err = shared.EnvConfig()
	if err != nil {
		return
	}

	os.Exit(m.Run())
}

func TestClusterResetRestoreSuite(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Cluster Reset Restore Test Suite")
}

var _ = AfterSuite(func() {
	g := GinkgoT()
	if customflag.ServiceFlag.ClusterConfig.Destroy {
		status, err := factory.DestroyCluster(g)
		Expect(err).NotTo(HaveOccurred())
		Expect(status).To(Equal("cluster destroyed"))
	}
})
