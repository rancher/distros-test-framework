package ipv6only

import (
	"flag"
	"os"
	"testing"

	"github.com/rancher/distros-test-framework/config"
	"github.com/rancher/distros-test-framework/entrypoint"
	"github.com/rancher/distros-test-framework/internal/pkg/aws"
	"github.com/rancher/distros-test-framework/internal/pkg/customflag"
	"github.com/rancher/distros-test-framework/internal/provisioning/driver"
	"github.com/rancher/distros-test-framework/internal/resources"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var (
	flags         *customflag.FlagConfig
	cluster       *driver.Cluster
	infraConfig   *driver.InfraConfig
	cfg           *config.Env
	err           error
	reportSummary string
	reportErr     error
	awsClient     *aws.Client
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

	// This is required in .env file as param ENV_MODULE=ipv6only.
	if cfg.Module == "" || cfg.Module != "ipv6only" {
		resources.LogLevel("info", "ENV_MODULE is not set with value ipv6only. Setting the value...\n")
		cfg.Module = "ipv6only"
	}

	cluster, infraConfig = entrypoint.SetupClusterInfra(cfg)
	awsClient, err = aws.AddClient(cluster)
	if err != nil {
		resources.LogLevel("error", "error adding aws nodes: %s", err)
		os.Exit(1)
	}

	os.Exit(m.Run())
}

func TestIPv6OnlySuite(t *testing.T) {
	RegisterFailHandler(entrypoint.FailWithReport)
	RunSpecs(t, "Create IPv6 Only Cluster Test Suite")
}

var _ = ReportAfterSuite("Create IPv6 Only Cluster Test Suite",
	entrypoint.ReportAfterSuite(&cluster, &reportSummary))

var _ = AfterSuite(entrypoint.AfterSuite(
	&cluster, &infraConfig, &reportSummary, &reportErr))
