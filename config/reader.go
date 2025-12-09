package config

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"

	"github.com/rancher/distros-test-framework/internal/pkg/logger"
)

const (
	defaultQAInfraModule = "aws"
	DefaultInfraProvider = "legacy"
)

var (
	envConfig *Env
	once      sync.Once
	log       = logger.AddLogger()

	supportedProducts       = []string{"k3s", "rke2", "k3k"}
	supportedQAinfraModules = []string{"aws", "vsphere"}
	supportedProviders      = []string{"legacy", "qainfra"}
	supportedLegacyTFVars   = []string{"k3s.tfvars", "rke2.tfvars"}
)

type Env struct {
	TFVars            string
	Product           string
	InstallVersion    string
	Module            string
	ResourceName      string
	ProvisionerModule string
	ProvisionerType   string
	QAInfraProvider   string
	SSHUser           string
	SSHKeyPath        string
	SSHKeyName        string
	NodeOS            string
	Channel           string
	CNI               string
	ServerFlags       string
	WorkerFlags       string
	Arch              string
	// K3k related configs
	ServerIP        string
	HostClusterType string
	K3kcliVersion   string
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
	// also needs to add all other variables needed for configuration.
	env := &Env{
		TFVars:            os.Getenv("ENV_TFVARS"),
		Product:           os.Getenv("ENV_PRODUCT"),
		InstallVersion:    os.Getenv("INSTALL_VERSION"),
		Module:            os.Getenv("ENV_MODULE"),
		ProvisionerModule: os.Getenv("PROVISIONER_MODULE"),
		QAInfraProvider:   os.Getenv("QA_INFRA_PROVIDER"),
		ProvisionerType:   os.Getenv("PROVISIONER_TYPE"),
		SSHUser:           os.Getenv("SSH_USER"),
		SSHKeyPath:        os.Getenv("SSH_LOCAL_KEY_PATH"),
		SSHKeyName:        os.Getenv("SSH_KEY_NAME"),
		ResourceName:      os.Getenv("RESOURCE_NAME"),
		NodeOS:            os.Getenv("NODE_OS"),
		Channel:           os.Getenv("CHANNEL"),
		CNI:               os.Getenv("CNI"),
		ServerFlags:       os.Getenv("SERVER_FLAGS"),
		WorkerFlags:       os.Getenv("WORKER_FLAGS"),
		Arch:              os.Getenv("ARCH"),
		// K3k related configs
		ServerIP:        os.Getenv("SERVER_IP"),
		HostClusterType: os.Getenv("HOST_CLUSTER_TYPE"),
		K3kcliVersion:   os.Getenv("K3KCLI_VERSION"),
	}
	if env.Product == "k3s" || env.Product == "rke2" {
		validateInitVars(env)
	} else {
		validateK3kVars(env)
	}
	// set env vars from respective infra module.
	switch env.ProvisionerModule {
	case "legacy", "":
		if env.TFVars != "" {
			tfPath := fmt.Sprintf("%s/config/%s", dir, env.TFVars)
			if err := setEnv(tfPath); err != nil {
				log.Errorf("failed to set environment variables: %v\n", err)
				return nil, err
			}
		}

	case "qainfra":
		if err := setEnv(dir + "/infrastructure/qainfra/vars.tfvars"); err != nil {
			log.Errorf("failed to set environment variables: %v\n", err)
			return nil, err
		}
	}

	return env, nil
}

func validateInitVars(env *Env) {
	normalizeInitVars(env)

	if env.InstallVersion == "" {
		log.Errorf("install version for %s is not set\n", env.Product)
		os.Exit(1)
	}

	if !isSupported(env.Product, supportedProducts) {
		log.Errorf("unknown product: %s; supported products are: %v\n", env.Product, supportedProducts)
		os.Exit(1)
	}

	if env.QAInfraProvider == "" {
		log.Info("QA_INFRA_MODULE is not set, defaulting to 'aws'")
		env.QAInfraProvider = defaultQAInfraModule
	}
	if !isSupported(env.QAInfraProvider, supportedQAinfraModules) {
		log.Errorf("unsupported module: %s; %v\n", env.Module, supportedQAinfraModules)
		os.Exit(1)
	}

	if env.ProvisionerModule == "" {
		env.ProvisionerModule = DefaultInfraProvider
	}
	if !isSupported(env.ProvisionerModule, supportedProviders) {
		log.Errorf("unsupported infra provider: %s;\nsupported providers are: %v\n",
			env.ProvisionerModule, supportedProviders)
		os.Exit(1)
	}

	// tfvars is required for legacy provider, optional for qainfra provider.
	if env.ProvisionerModule == DefaultInfraProvider {
		if env.TFVars == "" || !isSupported(env.TFVars, supportedLegacyTFVars) {
			log.Errorf("tfvars is required for legacy provider and must be one of %v, got: %s\n",
				supportedLegacyTFVars, env.TFVars)
			os.Exit(1)
		}
	}

	validateQainfraVars(env)
}

func validateQainfraVars(env *Env) {
	if env.ProvisionerModule == "qainfra" {
		user := env.SSHUser
		keyPath := env.SSHKeyPath
		resourceUsageName := env.ResourceName

		if user == "" {
			log.Errorf("ssh user is required for %s provider\n", env.ProvisionerModule)
			os.Exit(1)
		}

		if keyPath == "" {
			log.Errorf("ssh key path is required for %s provider\n", env.ProvisionerModule)
			os.Exit(1)
		}

		if resourceUsageName == "" {
			log.Errorf("resource name is required for %s provider\n", env.ProvisionerModule)
			os.Exit(1)
		}

		if env.NodeOS == "" {
			log.Errorf("node os is required for %s provider\n", env.ProvisionerModule)
			os.Exit(1)
		}
	}
}

func validateK3kVars(env *Env) {
	if env.K3kcliVersion == "" {
		log.Errorf("K3KCLI_VERSION is required for k3k product\n")
		os.Exit(1)
	}
	if env.ServerIP == "" {
		log.Errorf("SERVER_IP is required for k3k product\n")
		os.Exit(1)
	}
	if env.HostClusterType == "" {
		log.Errorf("HOST_CLUSTER_TYPE is required for k3k product\n")
		os.Exit(1)
	}
}

func normalizeInitVars(env *Env) {
	env.Product = strings.ToLower(strings.TrimSpace(env.Product))
	env.Module = strings.ToLower(strings.TrimSpace(env.Module))
	env.TFVars = strings.ToLower(strings.TrimSpace(env.TFVars))
	env.ProvisionerModule = strings.ToLower(strings.TrimSpace(env.ProvisionerModule))
	env.QAInfraProvider = strings.ToLower(strings.TrimSpace(env.QAInfraProvider))
	env.SSHUser = strings.TrimSpace(env.SSHUser)
	env.SSHKeyPath = strings.TrimSpace(env.SSHKeyPath)
	if env.ServerFlags != "" {
		env.ServerFlags = strings.ToLower(strings.TrimSpace(env.ServerFlags))
	}
	if env.WorkerFlags != "" {
		env.WorkerFlags = strings.ToLower(strings.TrimSpace(env.WorkerFlags))
	}
}

func isSupported(s string, list []string) bool {
	return slices.Contains(list, s)
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
