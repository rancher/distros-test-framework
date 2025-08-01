package main

import (
	"flag"
	"os"

	"github.com/rancher/distros-test-framework/pkg/qase"
	"github.com/rancher/distros-test-framework/shared"
)

var (
	fileName string
	product  string
)

func main() {
	flag.StringVar(&fileName, "f", "", "path to rke2/k3s e2e tests log file")
	flag.StringVar(&product, "p", "", "product name")
	flag.Parse()

	if product == "" {
		shared.LogLevel("error", "-p flag is required")
		os.Exit(1)
	}

	if fileName == "" {
		shared.LogLevel("error", "-f flag is required")
		os.Exit(1)
	}

	qaseClient, err := qase.AddQase()
	if err != nil {
		shared.LogLevel("error", "error adding qase: %w\n", err)
		os.Exit(1)
	}

	reportErr := qaseClient.ReportE2ETestRun(fileName, product)
	if reportErr != nil {
		shared.LogLevel("error", "error reporting test data to qase: %w\n", reportErr)
		os.Exit(1)
	}
}
