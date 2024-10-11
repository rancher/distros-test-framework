package validatecluster

import (
	"context"
	"flag"
	"fmt"
	"os"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/rancher/distros-test-framework/config"
	"github.com/rancher/distros-test-framework/pkg/customflag"
	"github.com/rancher/distros-test-framework/pkg/k8s"
	"github.com/rancher/distros-test-framework/shared"
)

var (
	kubeconfig string
	cluster    *shared.Cluster
)

func TestMain(m *testing.M) {
	flag.Var(&customflag.ServiceFlag.Destroy, "destroy", "Destroy cluster after test")
	flag.Parse()

	_, err := config.AddEnv()
	if err != nil {
		shared.LogLevel("error", "error adding env vars: %w\n", err)
		os.Exit(1)
	}

	kubeconfig = os.Getenv("KUBE_CONFIG")
	if kubeconfig == "" {
		// gets a cluster from terraform.
		cluster = shared.ClusterConfig()
	} else {
		// gets a cluster from kubeconfig.
		cluster = shared.KubeConfigCluster(kubeconfig)
	}

	c, err := k8s.Add()
	if err != nil {
		shared.LogLevel("error", "error adding k8s: %w\n", err)
		os.Exit(1)
	}

	// pods, err := c.ListResources("pods", "kube-system", "app=nginx")
	// if err != nil {
	// 	shared.LogLevel("error", "error listing pods: %w\n", err)
	// 	os.Exit(1)
	// }
	//
	// if pods.([]v1.Pod) == nil {
	// 	shared.LogLevel("error", "error listing pods: %w\n", err)
	// 	os.Exit(1)
	// }
	//
	// if pp, ok := pods.([]v1.Pod); ok {
	//
	// 	for _, p := range pp {
	// 		fmt.Printf("Pod: %v\n", p.Name)
	// 	}
	// }

	ctx := context.Background()
	nodes, err := c.WatchResources(ctx, "kube-system", "node", "")
	if err != nil {
		shared.LogLevel("error", "error watching nodes: %w\n", err)
		os.Exit(1)
	}

	if nodes {
		fmt.Printf("Nodes: %v\n", nodes)
	}

	os.Exit(m.Run())
}

func TestValidateClusterSuite(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Validate Cluster Test Suite")
}

var _ = AfterSuite(func() {
	if customflag.ServiceFlag.Destroy {
		status, err := shared.DestroyCluster()
		Expect(err).NotTo(HaveOccurred())
		Expect(status).To(Equal("cluster destroyed"))
	}
})
