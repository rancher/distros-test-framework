package main

import (
	"flag"
	"os"
	"path/filepath"

	"github.com/rancher/distros-test-framework/pkg/qase"
	"github.com/rancher/distros-test-framework/shared"
)

var (
	fileName string
	product  string
	ciArch   string
)

func main() {
	flag.StringVar(&fileName, "f", "", "path to rke2/k3s e2e tests log file")
	flag.StringVar(&product, "p", "", "product name")
	flag.StringVar(&ciArch, "a", "amd64", "architecture (amd64 or arm64)")
	flag.Parse()

	if product == "" {
		shared.LogLevel("error", "-p flag is required")
		os.Exit(1)
	}

	if fileName == "" {
		shared.LogLevel("error", "-f flag is required")
		os.Exit(1)
	}

	if ciArch == "" {
		shared.LogLevel("debug", "-a arch flag not being set, defaulting to amd64")
	}

	var runID int32
	var qaseErr error

	shared.LogLevel("info", "Starting report processing for %s...", product)

	qaseClient, err := qase.AddQase()
	if err != nil {
		shared.LogLevel("warn", "Qase client not configured: %v - will skip Qase reporting", err)
	} else {
		shared.LogLevel("info", "Qase client initialized, attempting to report to Qase...")
		runID, qaseErr = qaseClient.ReportE2ETestRun(fileName, product, ciArch)
		if qaseErr != nil {
			shared.LogLevel("error", "Failed to report to Qase: %v", qaseErr)
			shared.LogLevel("info", "Continuing with Slack reporting despite Qase failure...")
		} else {
			shared.LogLevel("info", "Successfully reported to Qase, run ID: %d", runID)
		}
	}

	shared.LogLevel("info", "Attempting to report to Slack...")
	baseDir := filepath.Dir(filepath.Dir(fileName))
	if slackErr := qase.ReportToSlack(fileName, product, ciArch, baseDir, runID); slackErr != nil {
		shared.LogLevel("error", "Failed to report to Slack: %v", slackErr)
		if qaseErr != nil {
			shared.LogLevel("error", "Both Qase and Slack reporting failed")
			os.Exit(1)
		}
	} else {
		shared.LogLevel("info", "Successfully reported to Slack")
	}

	if qaseErr != nil {
		shared.LogLevel("warn", "Completed with Qase errors - Slack report was sent successfully")
	}

	shared.LogLevel("info", "Report processing completed successfully")
}
