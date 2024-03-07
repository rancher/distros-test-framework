package secretsencrypt

import (
	"flag"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/rancher/distros-test-framework/config"
	"github.com/rancher/distros-test-framework/factory"
	"github.com/rancher/distros-test-framework/pkg/customflag"
	"github.com/rancher/distros-test-framework/shared"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var cfg *config.Product

func TestMain(m *testing.M) {
	var err error
	flag.Var(&customflag.ServiceFlag.ClusterConfig.Destroy, "destroy", "Destroy cluster after test")
	flag.Parse()

	cfg, err = shared.EnvConfig()
	if err != nil {
		return
	}

	os.Exit(m.Run())
}

func TestSecretsEncryptionSuite(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Secrets Encryption Test Suite")
}

var _ = BeforeSuite(func() {
	if err := config.SetEnv(shared.BasePath() + fmt.Sprintf("/config/%s.tfvars", cfg.Product)); err != nil {
		Expect(err).To(BeNil(), fmt.Sprintf("error loading tf vars: %v\n", err))
	}
	Expect(os.Getenv("server_flags")).To(ContainSubstring("secrets-encryption:"),
		"FATAL: Add secrets-encryption:true to server_flags for this test")

	version := os.Getenv(fmt.Sprintf("%s_version", cfg.Product))
	if strings.Contains(version, "1.27") || strings.Contains(version, "1.26") {
		os.Setenv("TEST_TYPE", "classic")
	} else {
		os.Setenv("TEST_TYPE", "both")
	}

})

var _ = AfterSuite(func() {
	g := GinkgoT()
	if customflag.ServiceFlag.ClusterConfig.Destroy {
		status, err := factory.DestroyCluster(g)
		Expect(err).NotTo(HaveOccurred())
		Expect(status).To(Equal("cluster destroyed"))
	}
})
