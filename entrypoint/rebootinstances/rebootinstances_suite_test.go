package rebootinstances

import (
	"flag"
	"os"
	"sync"
	"testing"

	"github.com/rancher/distros-test-framework/config"
	"github.com/rancher/distros-test-framework/entrypoint"
	"github.com/rancher/distros-test-framework/internal/pkg/aws"
	"github.com/rancher/distros-test-framework/internal/pkg/customflag"
	"github.com/rancher/distros-test-framework/internal/provisioning"
	"github.com/rancher/distros-test-framework/internal/provisioning/driver"
	"github.com/rancher/distros-test-framework/internal/report"
	"github.com/rancher/distros-test-framework/internal/resources"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var (
	flags         = &customflag.ServiceFlag
	cluster       *driver.Cluster
	infraConfig   *driver.InfraConfig
	cfg           *config.Env
	reportSummary string
	reportErr     error
	err           error
)

func TestMain(m *testing.M) {
	flags = &customflag.ServiceFlag
	flag.Var(&flags.Destroy, "destroy", "Destroy cluster after test")
	flag.Parse()

	cfg, err = config.AddEnv()
	if err != nil {
		resources.LogLevel("error", "error adding env vars: %w\n", err)
		os.Exit(1)
	}

	validateEIP()
	cluster, infraConfig = entrypoint.SetupClusterInfra(cfg)
	os.Exit(m.Run())
}

func TestRebootInstancesSuite(t *testing.T) {
	RegisterFailHandler(entrypoint.FailWithReport)
	RunSpecs(t, "Reboot Instances Test Suite")
}

var _ = ReportAfterSuite("Reboot Instances Test Suite",
	entrypoint.ReportAfterSuite(&cluster, &reportSummary))

var _ = AfterSuite(func() {
	reportSummary, reportErr = report.SummaryReportData(cluster, flags)
	if reportErr != nil {
		resources.LogLevel("error", "error getting report summary data: %v\n", reportErr)
	}

	if customflag.ServiceFlag.Destroy {
		status, err := provisioning.DestroyInfrastructure(
			infraConfig.ProvisionerModule, infraConfig.Product, infraConfig.Module)
		Expect(err).NotTo(HaveOccurred())
		Expect(status).To(Equal("cluster destroyed"))
	}

	cleanEIPs()
})

func validateEIP() {
	if os.Getenv("CREATE_EIP") == "" || os.Getenv("CREATE_EIP") != "true" {
		resources.LogLevel("error", "CREATE_EIP not set")
		os.Exit(1)
	}
}

// cleanEIPs release elastic ips from instances used on test.
func cleanEIPs() {
	release := os.Getenv("RELEASE_EIP")
	if release != "" && release == "false" {
		resources.LogLevel("info", "EIPs not released, being used to run test with kubeconfig")
	} else {
		awsDependencies, err := aws.AddClient(cluster)
		Expect(err).NotTo(HaveOccurred())

		eips := append(cluster.ServerIPs, cluster.AgentIPs...)

		var wg sync.WaitGroup
		for _, ip := range eips {
			ip := ip
			wg.Add(1)
			go func(ip string) {
				defer wg.Done()
				releaseEIPsErr := awsDependencies.ReleaseElasticIps(ip)
				if releaseEIPsErr != nil {
					resources.LogLevel("error", "on %w", releaseEIPsErr)
					return
				}
			}(ip)
		}
		wg.Wait()
	}
}
