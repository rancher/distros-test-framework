package sonobuoyconformance

import (
	"flag"
	"os"
	"strconv"
	"testing"

	"github.com/rancher/distros-test-framework/config"
	"github.com/rancher/distros-test-framework/pkg/customflag"
	"github.com/rancher/distros-test-framework/shared"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var (
	kubeconfig string
	cluster    *shared.Cluster
)

func TestMain(m *testing.M) {
	flag.StringVar(&customflag.ServiceFlag.External.SonobuoyVersion, "sonobuoyVersion", "0.57.2", "Sonobuoy binary version")
	flag.Var(&customflag.ServiceFlag.Destroy, "destroy", "Destroy cluster after test")
	flag.Parse()

	_, err := config.AddEnv()
	if err != nil {
		shared.LogLevel("error", "error adding env vars: %w\n", err)
		os.Exit(1)
	}
	verifyClusterNodes(cluster)

	kubeconfig = os.Getenv("KUBE_CONFIG")
	if kubeconfig == "" {
		cluster = shared.ClusterConfig()
	}
	os.Exit(m.Run())
	shared.LogLevel("error", "cluster must at least consist of 1 server and 1 agent")
	os.Exit(1)
}

func TestConformance(t *testing.T) {
	RegisterFailHandler(Fail)

	RunSpecs(t, "Run Conformance Suite")
}

var _ = AfterSuite(func() {
	if customflag.ServiceFlag.Destroy {
		status, err := shared.DestroyCluster()
		Expect(err).NotTo(HaveOccurred())
		Expect(status).To(Equal("cluster destroyed"))
	}
})

func verifyClusterNodes() bool {
	shared.LogLevel("info", "verying cluster configuration matches minimum requirements for conformance tests")
	serverNum, err := strconv.Atoi(os.Getenv("no_of_server_nodes"))
	if err != nil {
		shared.LogLevel("error", "error converting no_of_server_nodes to int: %w", err)
		os.Exit(1)
	}

	agentNum, _ := strconv.Atoi(os.Getenv("no_of_agent_nodes"))
	if err != nil {
		shared.LogLevel("error", "error converting no_of_agent_nodes to int: %w", err)
		os.Exit(1)
	}

	if serverNum < 1 && agentNum < 1 {
		shared.LogLevel("error", "%s", "cluster must at least consist of 1 server and 1 agent")
		os.Exit(1)
	}

	return true
}
