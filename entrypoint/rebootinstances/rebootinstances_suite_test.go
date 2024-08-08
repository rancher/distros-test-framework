package rebootinstances

import (
	"flag"
	"os"
	"sync"
	"testing"

	"github.com/rancher/distros-test-framework/config"
	"github.com/rancher/distros-test-framework/pkg/aws"
	"github.com/rancher/distros-test-framework/pkg/customflag"
	"github.com/rancher/distros-test-framework/shared"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var cluster *shared.Cluster

func TestMain(m *testing.M) {
	flag.Var(&customflag.ServiceFlag.Destroy, "destroy", "Destroy cluster after test")
	flag.Parse()

	_, err := config.AddEnv()
	if err != nil {
		shared.LogLevel("error", "error adding env vars: %w\n", err)
		os.Exit(1)
	}

	if os.Getenv("create_eip") == "" || os.Getenv("create_eip") != "true" {
		shared.LogLevel("error", "create_eip not set")
		os.Exit(1)
	}

	cluster = shared.ClusterConfig()

	os.Exit(m.Run())
}

func TestRebootInstancesSuite(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Reboot Instances Test Suite")
}

var _ = AfterSuite(func() {
	if customflag.ServiceFlag.Destroy {
		status, err := shared.DestroyCluster()
		Expect(err).NotTo(HaveOccurred())
		Expect(status).To(Equal("cluster destroyed"))
	}

	awsDependencies, err := aws.AddAWSClient(cluster)
	Expect(err).NotTo(HaveOccurred())

	cleanEIPs(awsDependencies)
})

// cleanEIPs release elastic ips from instances used on test.
func cleanEIPs(awsDependencies *aws.Client) {
	eips := append(cluster.ServerIPs, cluster.AgentIPs...)

	var wg sync.WaitGroup
	for _, ip := range eips {
		ip := ip
		wg.Add(1)
		go func(ip string) {
			defer wg.Done()
			releaseEIPsErr := awsDependencies.ReleaseElasticIps(ip)
			if releaseEIPsErr != nil {
				shared.LogLevel("error", "on %w", releaseEIPsErr)
				return
			}
		}(ip)
		wg.Wait()
	}
}
