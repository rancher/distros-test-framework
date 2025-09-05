package legacy

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/gruntwork-io/terratest/modules/terraform"

	"github.com/rancher/distros-test-framework/internal/provisioning"
	"github.com/rancher/distros-test-framework/internal/resources"
)

// Provision provisions infrastructure using the legacy terraform approach
func Provision(product, module string) (*provisioning.Cluster, error) {
	return ClusterConfig(product, module), nil
}

// newCluster creates a new cluster and returns his values from terraform config and vars.
func newCluster(product, module string) (*provisioning.Cluster, error) {
	c := &provisioning.Cluster{}
	t := &testing.T{}

	terraformOptions, varDir, err := setTerraformOptions(product, module)
	if err != nil {
		return nil, err
	}

	numServers, err := strconv.Atoi(terraform.GetVariableAsStringFromVarFile(
		t, varDir, "no_of_server_nodes"))
	if err != nil {
		return nil, fmt.Errorf("error getting no_of_server_nodes from var file: %w", err)
	}
	numAgents, err := strconv.Atoi(terraform.GetVariableAsStringFromVarFile(
		t, varDir, "no_of_worker_nodes"))
	if err != nil {
		return nil, fmt.Errorf("error getting no_of_worker_nodes from var file: %w", err)
	}
	numBastion, err := terraform.GetVariableAsStringFromVarFileE(t, varDir, "no_of_bastion_nodes")
	if err != nil {
		resources.LogLevel("debug", "no_of_bastion_nodes not found in tfvars")
		c.NumBastion = 0
	} else {
		c.NumBastion, _ = strconv.Atoi(numBastion)
	}

	resources.LogLevel("debug", "Applying Terraform config and Creating cluster\n")
	_, err = terraform.InitAndApplyE(t, terraformOptions)
	if err != nil {
		return nil, fmt.Errorf("\nTerraform apply Failed: %w", err)
	}
	resources.LogLevel("debug", "Applying Terraform config completed!\n")

	splitRoles := terraform.GetVariableAsStringFromVarFile(t, varDir, "split_roles")
	if splitRoles == "true" || os.Getenv("split_roles") == "true" {
		resources.LogLevel("debug", "Checking and adding split roles...")
		numServers, err = addSplitRole(t, &c.Config.SplitRoles, varDir, numServers)
		if err != nil {
			return nil, fmt.Errorf("error adding split roles: %w", err)
		}
	}
	c.NumServers = numServers
	c.NumAgents = numAgents

	resources.LogLevel("debug", "Loading TF Configs...")
	c, err = loadTFconfig(t, c, product, module, varDir, terraformOptions)
	if err != nil {
		return nil, fmt.Errorf("error loading TF config: %w", err)
	}
	c.Status = "cluster created"
	resources.LogLevel("debug", "Cluster has been created successfully...")

	return c, nil
}

//nolint:funlen // yep, but this makes more clear being one function.
func addClusterFromKubeConfig(nodes []provisioning.Node) (*provisioning.Cluster, error) {
	// if it is configureSSH() call then return the cluster with only aws key/user.
	if nodes == nil {
		return &provisioning.Cluster{
			SSH: provisioning.SSHConfig{
				KeyPath: os.Getenv("access_key"),
				User:    os.Getenv("aws_user"),
			},
		}, nil
	}
	var serverIPs, agentIPs []string
	for i := range nodes {
		if nodes[i].Roles == "<none>" && nodes[i].Roles != "control-plane" {
			agentIPs = append(agentIPs, nodes[i].ExternalIP)
		} else {
			serverIPs = append(serverIPs, nodes[i].ExternalIP)
		}
	}

	product := os.Getenv("ENV_PRODUCT")

	return &provisioning.Cluster{
		Status:     "cluster created",
		ServerIPs:  serverIPs,
		AgentIPs:   agentIPs,
		NumAgents:  len(agentIPs),
		NumServers: len(serverIPs),
		SSH: provisioning.SSHConfig{
			KeyPath: os.Getenv("access_key"),
			User:    os.Getenv("aws_user"),
		},
		Aws: provisioning.AwsConfig{
			Region:           os.Getenv("region"),
			Subnets:          os.Getenv("subnets"),
			SgId:             os.Getenv("sg_id"),
			AvailabilityZone: os.Getenv("availability_zone"),
			EC2: provisioning.EC2{
				Ami:           os.Getenv("aws_ami"),
				VolumeSize:    os.Getenv("volume_size"),
				InstanceClass: os.Getenv("ec2_instance_class"),
				KeyName:       os.Getenv("key_name"),
			},
		},
		Config: provisioning.Config{
			Product:             product,
			Version:             getVersion(),
			ServerFlags:         getFlags("server"),
			WorkerFlags:         getFlags("worker"),
			Channel:             getChannel(product),
			InstallMethod:       os.Getenv("install_method"),
			InstallMode:         os.Getenv("install_mode"),
			DataStore:           os.Getenv("datastore_type"),
			ExternalDb:          os.Getenv("external_db"),
			ExternalDbVersion:   os.Getenv("external_db_version"),
			ExternalDbGroupName: os.Getenv("db_group_name"),
			ExternalDbNodeType:  os.Getenv("instance_class"),
			ExternalDbEndpoint:  os.Getenv("rendered_template"),
			Arch:                os.Getenv("arch"),
			SplitRoles: provisioning.SplitRolesConfig{
				Add: os.Getenv("split_roles") == "true",
				NumServers: parseEnvInt("etcd_only_nodes", 0) +
					parseEnvInt("etcd_cp_nodes", 0) +
					parseEnvInt("etcd_worker_nodes", 0) +
					parseEnvInt("cp_only_nodes", 0) +
					parseEnvInt("cp_worker_nodes", 0),
				ControlPlaneOnly:   parseEnvInt("cp_only_nodes", 0),
				ControlPlaneWorker: parseEnvInt("cp_worker_nodes", 0),
				EtcdOnly:           parseEnvInt("etcd_only_nodes", 0),
				EtcdCP:             parseEnvInt("etcd_cp_nodes", 0),
				EtcdWorker:         parseEnvInt("etcd_worker_nodes", 0),
			},
		},
		Bastion: provisioning.BastionConfig{
			PublicIPv4Addr: os.Getenv("BASTION_IP"),
			PublicDNS:      os.Getenv("bastion_dns"),
		},
		NodeOS: os.Getenv("node_os"),
	}, nil
}

// TODO: aux functions for loading data while we dont standardize from one source of truth,
//
// this is being really messy and painful. remove after.
func loadVersion(t *testing.T, c *provisioning.Cluster, varDir string) {
	// defaults first always to get from env, because both local and jenkins we update this file
	if install := os.Getenv("INSTALL_VERSION"); install != "" {
		c.Config.Version = install
		resources.LogLevel("info", "Using install version from env: %s", install)
		return
	}

	version := c.Config.Product + "_version"
	if tf := terraform.GetVariableAsStringFromVarFile(t, varDir, version); tf != "" {
		c.Config.Version = tf
		resources.LogLevel("info", "Using install version from tfvars: %s", tf)
		return
	}
}

func loadChannel(t *testing.T, c *provisioning.Cluster, varDir string) {
	// defaults first always to get from env, because both local and jenkins we update this file
	if envInstallChannel := os.Getenv("INSTALL_CHANNEL"); envInstallChannel != "" {
		c.Config.Channel = envInstallChannel
		resources.LogLevel("info", "Using install channel from env INSTALL_CHANNEL: %s", envInstallChannel)
		return
	}

	tfChannel := c.Config.Product + "_channel"

	if tf := terraform.GetVariableAsStringFromVarFile(t, varDir, tfChannel); tf != "" {
		c.Config.Channel = tf
		resources.LogLevel("info", "Using install channel from tfvars: %s", tf)
		return
	}

	if install := os.Getenv("install_channel"); install != "" {
		c.Config.Channel = install
		resources.LogLevel("info", "Using install channel from env install_channel: %s", install)
		return
	}

	channelUp := strings.ToUpper(tfChannel)
	if env := os.Getenv(channelUp); env != "" {
		c.Config.Channel = env
		resources.LogLevel("info", "Using install channel from env %s: %s", channelUp, env)
		return
	}
}

// getVersion retrieves the install version from environment variables.
// used for addClusterFromKubeConfig().
func getVersion() string {
	installVersion := os.Getenv("install_version")
	installVersionEnv := os.Getenv("INSTALL_VERSION")

	if installVersion != "" {
		return installVersion
	}

	if installVersionEnv != "" {
		return installVersionEnv
	}

	return ""
}

// getFlags retrieves the flags for a given node type from environment variables.
// used for addClusterFromKubeConfig().
func getFlags(nodeType string) string {
	flags := os.Getenv(nodeType + "_flags")
	flagsEnv := os.Getenv(nodeType + "_FLAGS")

	if flags != "" {
		return flags
	}

	if flagsEnv != "" {
		return flagsEnv
	}

	return ""
}

// getChannel retrieves the install channel for a given product from environment variables.
// used for addClusterFromKubeConfig().
func getChannel(product string) string {
	c := os.Getenv("install_channel")
	if c != "" {
		return c
	}

	channel := os.Getenv(product + "_channel")
	if channel != "" {
		return channel
	}

	return "testing"
}

// parseEnvInt helper that parses an environment variable as an integer.
// If the variable is not set or cannot be parsed, it returns the default value.
func parseEnvInt(key string, defaultValue int) int {
	if val := os.Getenv(key); val != "" {
		if value, err := strconv.Atoi(val); err == nil {
			return value
		}
	}

	return defaultValue
}
func loadTestConfig(tc *provisioning.TestConfig) error {
	// extracting test tag from environment variables.
	// should pull from jenkins or local env.
	// for now only dealing with test tag.
	argsFromJenkins := os.Getenv("TEST_ARGS")
	if argsFromJenkins != "" {
		cmdStart := strings.Index(argsFromJenkins, "-tags=")
		if cmdStart == -1 {
			resources.LogLevel("debug", "tags value not found in test args %v", argsFromJenkins)
			return nil
		}
		testTag := strings.TrimSpace(argsFromJenkins[cmdStart+len("-tags="):])
		// take the first word after -tags=.
		testTag = strings.Split(testTag, " ")[0]
		if testTag != "" {
			tc.Tag = testTag
			resources.LogLevel("debug", "Test tag extracted from Jenkins: %s", tc.Tag)
			return nil
		}
		resources.LogLevel("debug", "No test tag found in Jenkins args: %v", argsFromJenkins)

		return nil
	}

	if tag := os.Getenv("TEST_TAG"); tag != "" {
		tc.Tag = tag
		resources.LogLevel("debug", "Test tag extracted from local env: %s", tc.Tag)

		return nil
	}

	return nil
}

func addSplitRole(t *testing.T, sp *provisioning.SplitRolesConfig, varDir string, numServers int) (int, error) {
	etcdNodes, err := strconv.Atoi(terraform.GetVariableAsStringFromVarFile(
		t,
		varDir,
		"etcd_only_nodes",
	))
	if err != nil {
		return 0, fmt.Errorf("error getting etcd_only_nodes %w", err)
	}
	etcdCpNodes, err := strconv.Atoi(terraform.GetVariableAsStringFromVarFile(
		t,
		varDir,
		"etcd_cp_nodes",
	))
	if err != nil {
		return 0, fmt.Errorf("error getting etcd_cp_nodes %w", err)
	}
	etcdWorkerNodes, err := strconv.Atoi(terraform.GetVariableAsStringFromVarFile(
		t,
		varDir,
		"etcd_worker_nodes",
	))
	if err != nil {
		return 0, fmt.Errorf("error getting etcd_worker_nodes %w", err)
	}
	cpNodes, err := strconv.Atoi(terraform.GetVariableAsStringFromVarFile(
		t,
		varDir,
		"cp_only_nodes",
	))
	if err != nil {
		return 0, fmt.Errorf("error getting cp_only_nodes %w", err)
	}
	cpWorkerNodes, err := strconv.Atoi(terraform.GetVariableAsStringFromVarFile(
		t,
		varDir,
		"cp_worker_nodes",
	))
	if err != nil {
		return 0, fmt.Errorf("error getting cp_worker_nodes %w", err)
	}

	numServers = numServers + etcdNodes + etcdCpNodes + etcdWorkerNodes + cpNodes + cpWorkerNodes

	sp.Add = true
	sp.ControlPlaneOnly = cpNodes
	sp.EtcdOnly = etcdNodes
	sp.EtcdCP = etcdCpNodes
	sp.EtcdWorker = etcdWorkerNodes
	sp.ControlPlaneWorker = cpWorkerNodes
	sp.NumServers = numServers

	return numServers, nil
}

func Destroy(product string, module string) (string, error) {
	terraformOptions, _, err := setTerraformOptions(product, module)
	if err != nil {
		return "", err
	}
	terraform.Destroy(&testing.T{}, terraformOptions)

	return "cluster destroyed", nil
}
