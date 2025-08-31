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
	envConfig *Env
	once      sync.Once
	log       = logger.AddLogger()
)

type Env struct {
	TFVars         string
	Product        string
	InstallVersion string
	Module         string
	InfraProvider  string
}

// AddEnv sets environment variables from the .env file,tf vars and returns the environment configuration.
func AddEnv() (*Env, error) {
	var err error
	once.Do(func() {
		envConfig, err = loadEnv()
		if err != nil {
			os.Exit(1)
		}
	})

	return envConfig, nil
}

func loadEnv() (*Env, error) {
	_, callerFilePath, _, _ := runtime.Caller(0)
	dir := filepath.Join(filepath.Dir(callerFilePath), "..")

	// set the environment variables from the .env file.
	dotEnvPath := dir + "/config/.env"
	if err := setEnv(dotEnvPath); err != nil {
		log.Errorf("failed to set environment variables: %v\n", err)
		return nil, err
	}

	env := &Env{
		TFVars:         os.Getenv("ENV_TFVARS"),
		Product:        os.Getenv("ENV_PRODUCT"),
		InstallVersion: os.Getenv("INSTALL_VERSION"),
		Module:         os.Getenv("ENV_MODULE"),
		InfraProvider:  os.Getenv("INFRA_PROVIDER"),
	}

	validateInitVars(env)

	// set the environment variables from the tfvars file (legacy provider only)
	if env.InfraProvider == "legacy" && env.TFVars != "" {
		tfPath := fmt.Sprintf("%s/config/%s", dir, env.TFVars)
		if err := setEnv(tfPath); err != nil {
			log.Errorf("failed to set environment variables: %v\n", err)
			return nil, err
		}
	}

	return env, nil
}

func validateInitVars(env *Env) {
	if env.InstallVersion == "" {
		log.Errorf("install version for %s is not set\n", env.Product)
		os.Exit(1)
	}

	// For qa-infra provider, TFVars can be empty (uses vars.tfvars internally)
	// For legacy provider, TFVars must be k3s.tfvars or rke2.tfvars
	if env.InfraProvider == "legacy" && (env.TFVars == "" ||
		(env.TFVars != "k3s.tfvars" && env.TFVars != "rke2.tfvars")) {
		log.Errorf("legacy provider requires tfvars to be k3s.tfvars or rke2.tfvars, "+
			"got: %s\n", env.TFVars)
		os.Exit(1)
	}

	if env.Product == "" || (env.Product != "k3s" && env.Product != "rke2") {
		log.Errorf("unknown product: %s\n", env.Product)
		os.Exit(1)
	}
}

func setEnv(fullPath string) error {
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
