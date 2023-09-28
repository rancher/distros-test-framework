package config

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

var (
	product *ProductConfig
	once    sync.Once
)

type ProductConfig struct {
	TFVars  string
	Product string
}

func AddConfigEnv(path string) (*ProductConfig, error) {
	once.Do(func() {
		var err error
		product, err = loadConfigEnv(path)
		if err != nil {
			return
		}
	})

	return product, nil
}

func loadConfigEnv(path string) (config *ProductConfig, err error) {
	dir, err := os.Getwd()
	if err != nil {
		fmt.Println("failed to get current working directory", err)
		return nil, err
	}
	fullPath := filepath.Join(dir, path)

	file, err := os.Open(fullPath)
	if err != nil {
		fmt.Println("failed to open file:", err)
		return nil, err
	}
	defer file.Close()

	config = &ProductConfig{}

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}

		key, value := parts[0], parts[1]
		err = os.Setenv(key, value)
		if err != nil {
			return nil, err
		}

		switch key {
		case "ENV_TFVARS":
			if value == "" || (value != "k3s.tfvars" && value != "rke2.tfvars") {
				fmt.Printf("unknown tfvars: %s\n", value)
				os.Exit(1)
			}
		case "ENV_PRODUCT":
			if value == "" || (value != "k3s" && value != "rke2") {
				fmt.Printf("unknown product: %s\n", value)
				os.Exit(1)
			}
		}

		config.TFVars = os.Getenv("ENV_TFVARS")
		config.Product = os.Getenv("ENV_PRODUCT")
	}

	if err = scanner.Err(); err != nil {
		return nil, err
	}

	return config, nil
}
