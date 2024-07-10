package validatecluster

import (
	"encoding/base64"
	"flag"
	"fmt"
	"os"
	"testing"

	"github.com/rancher/distros-test-framework/factory"
	"github.com/rancher/distros-test-framework/pkg/customflag"
	"github.com/rancher/distros-test-framework/shared"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var cluster *factory.Cluster
var k = flag.String("kubeconfig", "", "kubeconfig file")

func TestMain(m *testing.M) {
	flag.Var(&customflag.ServiceFlag.Destroy, "destroy", "Destroy cluster after test")
	flag.Parse()

	if *k == "" {
		cluster = factory.ClusterConfig()
	} else {
		dec, err := base64.StdEncoding.DecodeString(*k)
		if err != nil {
			fmt.Println("error decoding kubeconfig")
		}

		localPath := fmt.Sprintf("/tmp/%s_kubeconfig", "franmorallocalrke2")
		err = os.WriteFile(localPath, dec, 0644)

		factory.KubeConfigFile = localPath

		ips := shared.FetchNodeExternalIPs()
		cluster, err = factory.AddClusterFromKubeConfig(cluster, ips, factory.KubeConfigFile)
		if err != nil {
			fmt.Println("error adding cluster from kubeconfig")
		}
	}

	os.Exit(m.Run())
}

func TestValidateClusterSuite(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Create Cluster Test Suite")
}

var _ = AfterSuite(func() {
	if customflag.ServiceFlag.Destroy {
		status, err := factory.DestroyCluster()
		Expect(err).NotTo(HaveOccurred())
		Expect(status).To(Equal("cluster destroyed"))
	}
})
