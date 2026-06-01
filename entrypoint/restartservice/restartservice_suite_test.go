package restartservice

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
	cluster, infraConfig = entrypoint.SetupClusterInfra(cfg)
	os.Exit(m.Run())
}

func TestRestartServiceSuite(t *testing.T) {
	RegisterFailHandler(entrypoint.FailWithReport)
	RunSpecs(t, "Restart Service Test Suite")
}

var _ = ReportAfterSuite("Restart Service Test Suite",
	entrypoint.ReportAfterSuite(&cluster, &reportSummary))

var _ = AfterSuite(entrypoint.AfterSuite(
	&cluster, &infraConfig, &reportSummary, &reportErr))
