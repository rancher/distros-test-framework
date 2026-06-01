package clusterrestore

import (
	"flag"
	"os"
	"strings"
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
	cfg           *config.Env
	cluster       *driver.Cluster
	infraConfig   *driver.InfraConfig
	reportSummary string
	reportErr     error
	err           error
	awsClient     *aws.Client
)

func TestMain(m *testing.M) {
	flags = &customflag.ServiceFlag
	flag.Var(&flags.Destroy, "destroy", "Destroy cluster after test")
	flag.Var(&flags.Channel, "channel", "channel to use on install")
	flag.StringVar(&flags.S3Flags.Bucket, "s3Bucket", "distrosqa", "s3 bucket to store snapshots")
	flag.StringVar(&flags.S3Flags.Folder, "s3Folder", "snapshots", "s3 folder to store snapshots")
	flag.Parse()

	cfg, err = config.AddEnv()
	if err != nil {
		resources.LogLevel("error", "error adding env vars: %w\n", err)
		os.Exit(1)
	}

	// checkUnsupportedFlags validates the hardening flags are not passed as they are not supported for now.
	checkUnsupportedFlags()
	cluster, infraConfig = entrypoint.SetupClusterInfra(cfg)
	awsClient, err = aws.AddClient(cluster)
	if err != nil {
		resources.LogLevel("error", "error adding aws nodes: %s", err)
		os.Exit(1)
	}

	os.Exit(m.Run())
}

func TestClusterResetRestoreSuite(t *testing.T) {
	RegisterFailHandler(entrypoint.FailWithReport)
	RunSpecs(t, "Cluster Reset Restore Test Suite")
}

var _ = AfterSuite(entrypoint.AfterSuite(
	&cluster, &infraConfig, &reportSummary, &reportErr))

var _ = ReportAfterSuite("Cluster Reset Restore Test Suite",
	entrypoint.ReportAfterSuite(&cluster, &reportSummary))

func checkUnsupportedFlags() {
	serverFlags := os.Getenv("server_flags")

	if strings.Contains(serverFlags, "profile") ||
		strings.Contains(serverFlags, "selinux") ||
		strings.Contains(serverFlags, "protect-kernel-defaults") ||
		strings.Contains(serverFlags, "/etc/rancher/rke2/custom-psa.yaml") {
		resources.LogLevel("error", "hardening flags are not supported for now")
		os.Exit(1)
	}
}
