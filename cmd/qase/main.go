package main

import (
	"flag"
	"os"

	"github.com/rancher/distros-test-framework/internal/pkg/qase"
	"github.com/rancher/distros-test-framework/internal/resources"
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
		resources.LogLevel("error", "-p flag is required")
		os.Exit(1)
	}

	if fileName == "" {
		resources.LogLevel("error", "-f flag is required")
		os.Exit(1)
	}

	qaseClient, err := qase.AddQase()
	if err != nil {
		resources.LogLevel("error", "error adding qase: %w\n", err)
		os.Exit(1)
	}

	reportErr := qaseClient.ReportE2ETestRun(fileName, product)
	if reportErr != nil {
		resources.LogLevel("error", "error reporting test data to qase: %w\n", reportErr)
		os.Exit(1)
	}
}
