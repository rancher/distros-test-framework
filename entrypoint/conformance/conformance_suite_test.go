package sonobuoyconformance

import (
	"flag"
	"os"
	"testing"

	"github.com/rancher/distros-test-framework/config"
	"github.com/rancher/distros-test-framework/entrypoint"
	"github.com/rancher/distros-test-framework/internal/pkg/customflag"
	"github.com/rancher/distros-test-framework/internal/provisioning/driver"
	"github.com/rancher/distros-test-framework/internal/resources"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var (
	cluster       *driver.Cluster
	infraConfig   *driver.InfraConfig
	flags         *customflag.FlagConfig
	cfg           *config.Env
	reportSummary string
	reportErr     error
	err           error
)

func TestMain(m *testing.M) {
	flags = &customflag.ServiceFlag
	flag.StringVar(&customflag.ServiceFlag.External.SonobuoyVersion, "sonobuoyVersion", "0.57.3", "Sonobuoy binary version")
	flag.Var(&customflag.ServiceFlag.Destroy, "destroy", "Destroy cluster after test")
	flag.Parse()

	cfg, err = config.AddEnv()
	if err != nil {
		resources.LogLevel("error", "error adding env vars: %w\n", err)
		os.Exit(1)
	}

	cluster, infraConfig = entrypoint.SetupClusterInfra(cfg)

	os.Exit(m.Run())
}

func TestConformance(t *testing.T) {
	RegisterFailHandler(entrypoint.FailWithReport)

	RunSpecs(t, "Run Conformance Suite")
}

var _ = ReportAfterSuite("Conformance Suite",
	entrypoint.ReportAfterSuite(&cluster, &reportSummary))

var _ = AfterSuite(entrypoint.AfterSuite(
	&cluster, &infraConfig, &reportSummary, &reportErr))

// Inside the Ginkgo lifecycle so a failed check still runs AfterSuite and
// destroys the provisioned cluster instead of leaking it via os.Exit.
var _ = BeforeSuite(func() {
	resources.LogLevel("info", "verifying cluster configuration matches minimum requirements for conformance tests")
	// Topology comes from the provisioner (qainfra nodes[] block or legacy
	// tfvars); the NO_OF_* env params are not set on qainfra jobs.
	Expect(cluster.NumServers >= 1 || cluster.NumAgents >= 1).To(BeTrue(),
		"cluster must have at least one node (servers=%d, agents=%d)",
		cluster.NumServers, cluster.NumAgents)
})
