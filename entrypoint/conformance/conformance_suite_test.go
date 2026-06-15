package sonobuoyconformance

import (
	"flag"
	"os"
	"strconv"
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

	verifyClusterNodes()
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

func verifyClusterNodes() {
	resources.LogLevel("info", "verying cluster configuration matches minimum requirements for conformance tests")
	s, serverErr := strconv.Atoi(os.Getenv("NO_OF_SERVER_NODES"))
	w, workerErr := strconv.Atoi(os.Getenv("NO_OF_WORKER_NODES"))

	if serverErr != nil || workerErr != nil {
		resources.LogLevel("error", "Failed to convert node counts to integers: %v, %v", serverErr, workerErr)
		os.Exit(1)
	}

	if s < 1 && w < 1 {
		resources.LogLevel("error", "%s", "cluster must at least consist of 1 server and 1 agent")
		os.Exit(1)
	}
}
