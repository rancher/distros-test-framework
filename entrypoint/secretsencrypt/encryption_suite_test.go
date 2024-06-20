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

var cluster *factory.Cluster

func TestMain(m *testing.M) {
	flag.Var(&customflag.ServiceFlag.Destroy, "destroy", "Destroy cluster after test")
	flag.Parse()

	cluster = factory.ClusterConfig()

	os.Exit(m.Run())
}

func TestSecretsEncryptionSuite(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Secrets Encryption Test Suite")
}

var _ = BeforeSuite(func() {
	if err := config.SetEnv(shared.BasePath() + fmt.Sprintf("/config/%s.tfvars", cluster.Config.Product)); err != nil {
		Expect(err).To(BeNil(), fmt.Sprintf("error loading tf vars: %v\n", err))
	}
	if cluster.Config.Product == "k3s" {
		Expect(os.Getenv("server_flags")).To(ContainSubstring("secrets-encryption:"),
			"ERROR: Add secrets-encryption:true to server_flags for this test")
	}

	version := os.Getenv(fmt.Sprintf("%s_version", cluster.Config.Product))

	var envErr error
	if strings.Contains(version, "1.27") || strings.Contains(version, "1.26") {
		envErr = os.Setenv("TEST_TYPE", "classic")
		Expect(envErr).To(BeNil(), fmt.Sprintf("error setting env var: %v\n", envErr))
	} else {
		envErr = os.Setenv("TEST_TYPE", "both")
		Expect(envErr).To(BeNil(), fmt.Sprintf("error setting env var: %v\n", envErr))
	}
})

var _ = AfterSuite(func() {
	if customflag.ServiceFlag.Destroy {
		status, err := factory.DestroyCluster()
		Expect(err).NotTo(HaveOccurred())
		Expect(status).To(Equal("cluster destroyed"))
	}
})
