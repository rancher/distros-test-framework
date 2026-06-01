package secretsencrypt

import (
	"flag"
	"os"
	"strings"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/rancher/distros-test-framework/config"
	"github.com/rancher/distros-test-framework/entrypoint"
	"github.com/rancher/distros-test-framework/internal/pkg/customflag"
	"github.com/rancher/distros-test-framework/internal/provisioning/driver"
	"github.com/rancher/distros-test-framework/internal/resources"
)

var (
	cluster       *driver.Cluster
	flags         *customflag.FlagConfig
	cfg           *config.Env
	infraConfig   *driver.InfraConfig
	reportSummary string
	reportErr     error
	err           error
)

func TestMain(m *testing.M) {
	flags = &customflag.ServiceFlag
	flag.Var(&flags.Destroy, "destroy", "Destroy cluster after test")
	flag.StringVar(&flags.SecretsEncrypt.Method, "secretsEncryptMethod", "both", "method to perform secrets encryption")
	flag.Parse()

	cfg, err = config.AddEnv()
	if err != nil {
		resources.LogLevel("error", "error adding env vars: %w\n", err)
		os.Exit(1)
	}

	validateSecretsEncryptFlag()
	cluster, infraConfig = entrypoint.SetupClusterInfra(cfg)
	os.Exit(m.Run())
}

func TestSecretsEncryptionSuite(t *testing.T) {
	RegisterFailHandler(entrypoint.FailWithReport)
	RunSpecs(t, "Secrets Encryption Test Suite")
}

func validateSecretsEncryptFlag() {
	if cfg.Product == "k3s" {
		if !strings.Contains(os.Getenv("server_flags"), "secrets-encryption:") {
			resources.LogLevel("error", "Add secrets-encryption:true to server_flags for this test")
			os.Exit(1)
		}
	}

	if strings.Contains(os.Getenv("server_flags"), "secretbox") &&
		flags.SecretsEncrypt.Method != "rotate-keys" {
		resources.LogLevel("info", "secretbox provider is supported only with rotate-keys operation")
		flags.SecretsEncrypt.Method = "rotate-keys"
	}
}

var _ = ReportAfterSuite("Secrets Encryption Test Suite",
	entrypoint.ReportAfterSuite(&cluster, &reportSummary))

var _ = AfterSuite(entrypoint.AfterSuite(
	&cluster, &infraConfig, &reportSummary, &reportErr))
