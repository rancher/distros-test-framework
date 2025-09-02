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

const (
	defaultQAInfraModule = "aws"
	DefaultInfraProvider = "legacy"
)

var (
	envConfig *Env
	once      sync.Once
	log       = logger.AddLogger()

	supportedProducts       = []string{"k3s", "rke2"}
	supportedQAinfraModules = []string{"aws", "vsphere"}
	supportedProviders      = []string{"legacy", "qa-infra"}
	supportedLegacyTFVars   = []string{"k3s.tfvars", "rke2.tfvars"}
)

// type QaseEnv struct {
// 	ReportToQase        bool
// 	QaseProjectID       string
// 	QaseRunID           int64
// 	QaseTestCaseID      int64
// 	QaseAutomationToken string
// }

type Env struct {
	TFVars         string
	Product        string
	InstallVersion string
	Module         string
	ResourceName   string
	InfraProvider  string
	QAInfraModule  string
	SSHUser        string
	SSHKeyPath     string
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

	// set the environment variables from the .env file related to infrastructure/framework configuration.
	// TODO: this should be refactored remove install version from here and update accordingly.
	env := &Env{
		TFVars:         os.Getenv("ENV_TFVARS"),
		Product:        os.Getenv("ENV_PRODUCT"),
		InstallVersion: os.Getenv("INSTALL_VERSION"),
		Module:         os.Getenv("ENV_MODULE"),
		InfraProvider:  os.Getenv("INFRA_PROVIDER"),
		QAInfraModule:  os.Getenv("QA_INFRA_MODULE"),
		SSHUser:        os.Getenv("SSH_USER"),
		SSHKeyPath:     os.Getenv("SSH_KEY_PATH"),
		ResourceName:   os.Getenv("RESOURCE_NAME"),
	}

	validateInitVars(env)

	if (env.InfraProvider == "legacy" || env.InfraProvider == "") && env.TFVars != "" {
		tfPath := fmt.Sprintf("%s/config/%s", dir, env.TFVars)
		if err := setEnv(tfPath); err != nil {
			log.Errorf("failed to set environment variables: %v\n", err)
			return nil, err
		}
	}

	return env, nil
}

func validateInitVars(env *Env) {
	normalize(env)

	if env.InstallVersion == "" {
		log.Errorf("install version for %s is not set\n", env.Product)
		os.Exit(1)
	}

	if !isSupported(env.Product, supportedProducts) {
		log.Errorf("unknown product: %s; supported products are: %v\n", env.Product, supportedProducts)
		os.Exit(1)
	}

	if env.QAInfraModule == "" {
		env.QAInfraModule = defaultQAInfraModule
	}
	if !isSupported(env.QAInfraModule, supportedQAinfraModules) {
		log.Errorf("unsupported module: %s; %v\n", env.Module, supportedQAinfraModules)
		os.Exit(1)
	}

	if env.InfraProvider == "" {
		env.InfraProvider = DefaultInfraProvider
	}
	if !isSupported(env.InfraProvider, supportedProviders) {
		log.Errorf("unsupported infra provider: %s;\nsupported providers are: %v\n", env.InfraProvider, supportedProviders)
		os.Exit(1)
	}

	// tfvars is required for legacy provider, optional for qa-infra provider.
	if env.InfraProvider == DefaultInfraProvider {
		if env.TFVars == "" || !isSupported(env.TFVars, supportedLegacyTFVars) {
			log.Errorf("tfvars is required for legacy provider and must be one of %v, got: %s\n",
				supportedLegacyTFVars, env.TFVars)
			os.Exit(1)
		}
	}

	if env.InfraProvider == "qa-infra" {
		user := env.SSHUser
		keyPath := env.SSHKeyPath
		resourceUsageName := env.ResourceName

		if user == "" {
			log.Errorf("ssh user is required for %s provider\n", env.InfraProvider)
			os.Exit(1)
		}

		if keyPath == "" {
			log.Errorf("ssh key path is required for %s provider\n", env.InfraProvider)
			os.Exit(1)
		}

		if resourceUsageName == "" {
			log.Errorf("resource name is required for %s provider\n", env.InfraProvider)
			os.Exit(1)
		}

	}

}

func normalize(env *Env) {
	env.Product = strings.ToLower(strings.TrimSpace(env.Product))
	env.Module = strings.ToLower(strings.TrimSpace(env.Module))
	env.TFVars = strings.ToLower(strings.TrimSpace(env.TFVars))
	env.InfraProvider = strings.ToLower(strings.TrimSpace(env.InfraProvider))
	env.QAInfraModule = strings.ToLower(strings.TrimSpace(env.QAInfraModule))
	env.SSHUser = strings.TrimSpace(env.SSHUser)
	env.SSHKeyPath = strings.TrimSpace(env.SSHKeyPath)
}

func isSupported(s string, list []string) bool {
	for _, value := range list {
		if s == value {
			return true
		}
	}

	return false
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
