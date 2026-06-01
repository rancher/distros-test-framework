package nvidia

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
	cfg           *config.Env
	err           error
	nvidiaVersion string
)

func TestMain(m *testing.M) {
	flag.Var(&customflag.ServiceFlag.Destroy, "destroy", "Destroy cluster after test")
	flag.StringVar(&nvidiaVersion, "nvidiaVersion", "570.133.20", "Nvidia version")
	flag.Parse()

	cfg, err = config.AddEnv()
	if err != nil {
		resources.LogLevel("error", "error adding env vars: %w\n", err)
		os.Exit(1)
	}
	cluster, infraConfig = entrypoint.SetupClusterInfra(cfg)
	os.Exit(m.Run())
}

func TestNvidiaSuite(t *testing.T) {
	RegisterFailHandler(entrypoint.FailWithReport)
	RunSpecs(t, "Nvidia Test Suite")
}

var _ = AfterSuite(entrypoint.DestroyOnlyAfterSuite(&infraConfig))
