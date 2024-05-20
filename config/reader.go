package config

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"github.com/rancher/distros-test-framework/pkg/logger"
)

var (
	product *Product
	once    sync.Once
	log     = logger.AddLogger()
)

type Product struct {
	TFVars  string
	Product string
}

// AddEnv sets environment variables from the .env file and returns the Product configuration.
func AddEnv() (*Product, error) {
	var err error
	once.Do(func() {
		product, err = loadEnv()
		if err != nil {
			os.Exit(1)
		}
	})

	return product, nil
}

func loadEnv() (*Product, error) {
	_, callerFilePath, _, _ := runtime.Caller(0)
	dir := filepath.Join(filepath.Dir(callerFilePath), "..")

	fullPath := fmt.Sprintf("%s/config/.env", dir)
	if err := SetEnv(fullPath); err != nil {
		log.Errorf("failed to set environment variables: %v\n", err)
		return nil, err
	}

	productConfig := &Product{
		TFVars:  os.Getenv("ENV_TFVARS"),
		Product: os.Getenv("ENV_PRODUCT"),
	}
	if productConfig.TFVars == "" || (productConfig.TFVars != "k3s.tfvars" && productConfig.TFVars != "rke2.tfvars") {
		log.Errorf("unknown tfvars: %s\n", productConfig.TFVars)
		os.Exit(1)
	}

	if productConfig.Product == "" || (productConfig.Product != "k3s" && productConfig.Product != "rke2") {
		log.Errorf("unknown product: %s\n", productConfig.Product)
		os.Exit(1)
	}

	return productConfig, nil
}

func SetEnv(fullPath string) error {
	file, err := os.Open(fullPath)
	if err != nil {
		log.Errorf("failed to open file: %v\n", err)
		return err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])
		err = os.Setenv(strings.Trim(key, "\""), strings.Trim(value, "\""))
		if err != nil {
			log.Errorf("failed to set environment variables: %v\n", err)
			return err
		}
	}

	return scanner.Err()
}
