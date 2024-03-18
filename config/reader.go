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
	l       = logger.AddLogger()
)

type Product struct {
	TFVars  string
	Product string
}

func AddEnv() (*Product, error) {
	once.Do(func() {
		var err error
		product, err = loadEnv()
		if err != nil {
			l.Errorf("error adding environment variables: %v\n", err)
			return
		}
	})

	return product, nil
}

func loadEnv() (config *Product, err error) {
	_, callerFilePath, _, _ := runtime.Caller(0)
	dir := filepath.Join(filepath.Dir(callerFilePath), "..")

	fullPath := fmt.Sprintf("%s/config/.env", dir)
	if err = SetEnv(fullPath); err != nil {
		l.Errorf("failed to set environment variables: %v\n", err)
		return nil, err
	}

	config = &Product{}
	config.TFVars = os.Getenv("ENV_TFVARS")
	config.Product = os.Getenv("ENV_PRODUCT")
	if config.TFVars == "" || (config.TFVars != "k3s.tfvars" && config.TFVars != "rke2.tfvars") {
		l.Errorf("unknown tfvars: %s\n", config.TFVars)
		os.Exit(1)
	}

	if config.Product == "" || (config.Product != "k3s" && config.Product != "rke2") {
		l.Errorf("unknown product: %s\n", config.Product)
		os.Exit(1)
	}

	return config, nil
}

func SetEnv(fullPath string) error {
	file, err := os.Open(fullPath)
	if err != nil {
		l.Errorf("failed to open file: %v\n", err)
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
			l.Errorf("failed to set environment variables: %v\n", err)
			return err
		}
	}

	return scanner.Err()
}
