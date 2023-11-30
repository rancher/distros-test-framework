package config

import (
	"bufio"
	"fmt"
	"os"
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
		product, err = loadEnv(path)
		if err != nil {
			return
		}
	})

	return product, nil
}

func loadEnv(fullPath string) (config *ProductConfig, err error) {
	if err = setEnv(fullPath); err != nil {
		return nil, err
	}

	config = &ProductConfig{}
	config.TFVars = os.Getenv("ENV_TFVARS")
	config.Product = os.Getenv("ENV_PRODUCT")

	if config.TFVars == "" || (config.TFVars != "k3s.tfvars" && config.TFVars != "rke2.tfvars") {
		fmt.Printf("unknown tfvars: %s\n", config.TFVars)
		os.Exit(1)
	}

	if config.Product == "" || (config.Product != "k3s" && config.Product != "rke2") {
		fmt.Printf("unknown product: %s\n", config.Product)
		os.Exit(1)
	}

	return config, nil
}

func setEnv(fullPath string) error {
	file, err := os.Open(fullPath)
	if err != nil {
		fmt.Println("failed to open file:", err)
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

		key, value := parts[0], parts[1]
		err = os.Setenv(key, value)
		if err != nil {
			return err
		}
	}

	return scanner.Err()
}
