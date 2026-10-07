package kata

import (
	"flag"
	"os"
	"testing"

	"github.com/rancher/distros-test-framework/config"
	"github.com/rancher/distros-test-framework/entrypoint"
	"github.com/rancher/distros-test-framework/internal/pkg/customflag"
	"github.com/rancher/distros-test-framework/internal/pkg/testcase/support"
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
	kataConfig    support.KataConfig
	reportSummary string
	reportErr     error
	err           error
)

func TestMain(m *testing.M) {
	flags = &customflag.ServiceFlag
	flag.Var(&flags.Destroy, "destroy", "Destroy cluster after test")
	flag.Parse()
	cfg, err = config.AddEnv()
	if err == nil {
		kataConfig, err = support.LoadKataConfig(cfg)
	}
	if err != nil {
		resources.LogLevel("error", "Kata P0 preflight: %v", err)
		os.Exit(1)
	}

	cluster, infraConfig = entrypoint.SetupClusterInfra(cfg)
	os.Exit(m.Run())
}

func TestKataSuite(t *testing.T) {
	RegisterFailHandler(entrypoint.FailWithReport)
	RunSpecs(t, "RKE2 Prime Kata P0 Test Suite (amd64, no NVIDIA)")
}

var (
	_ = ReportAfterSuite("Kata P0 Test Suite", entrypoint.ReportAfterSuite(&cluster, &reportSummary))
	_ = AfterSuite(func() {
		defer entrypoint.AfterSuite(&cluster, &infraConfig, &reportSummary, &reportErr)()
		Expect(support.CleanupKata(&kataConfig)).To(Succeed())
	})
)
