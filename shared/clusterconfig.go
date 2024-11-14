package shared

import (
	"fmt"
	"os"
	"strconv"
	"sync"
	"testing"

	"github.com/gruntwork-io/terratest/modules/terraform"

	"github.com/rancher/distros-test-framework/pkg/customflag"
)

var (
	once    sync.Once
	cluster *Cluster
	product string
	module  string
)

type Cluster struct {
	Status        string
	ServerIPs     []string
	AgentIPs      []string
	WinAgentIPs   []string
	NumWinAgents  int
	NumServers    int
	NumAgents     int
	FQDN          string
	Config        clusterConfig
	Aws           AwsConfig
	BastionConfig bastionConfig
}

type AwsConfig struct {
	AccessKeyID     string
	SecretAccessKey string
	Region          string
	EC2Config
}

type EC2Config struct {
	AccessKey        string
	AwsUser          string
	Ami              string
	VolumeSize       string
	InstanceClass    string
	Subnets          string
	AvailabilityZone string
	SgId             string
	KeyName          string
}

type clusterConfig struct {
	RenderedTemplate string
	ExternalDb       string
	DataStore        string
	Product          string
	Arch             string
	Version          string
	ServerFlags      string
}

type bastionConfig struct {
	PublicIPv4Addr string
	PublicDNS      string
}

type Node struct {
	Name              string
	Status            string
	Roles             string
	Version           string
	InternalIP        string
	ExternalIP        string
	OperationalSystem string
}

type Pod struct {
	NameSpace      string
	Name           string
	Ready          string
	Status         string
	Restarts       string
	Age            string
	IP             string
	Node           string
	NominatedNode  string
	ReadinessGates string
}

// setConfig gets env configs and sets on local vars.
func setConfig() {
	product = os.Getenv("ENV_PRODUCT")
	module = os.Getenv("ENV_MODULE")
}

// ClusterConfig returns a singleton cluster with all terraform config and vars.
func ClusterConfig() *Cluster {
	setConfig()
	once.Do(func() {
		var err error
		cluster, err = newCluster(product, module)
		if err != nil {
			LogLevel("error", "error getting cluster: %w\n", err)
			if customflag.ServiceFlag.Destroy {
				LogLevel("info", "\nmoving to start destroy operation\n")
				status, destroyErr := DestroyCluster()
				if destroyErr != nil {
					LogLevel("error", "error destroying cluster: %w\n", destroyErr)
					os.Exit(1)
				}
				if status != "cluster destroyed" {
					LogLevel("error", "cluster not destroyed: %s\n", status)
					os.Exit(1)
				}
			}
			os.Exit(1)
		}
	})

	return cluster
}

func addClusterFromKubeConfig(nodes []Node) (*Cluster, error) {
	// if it is configureSSH() call then return the cluster with only aws key/user.
	if nodes == nil {
		return &Cluster{
			Aws: AwsConfig{
				EC2Config: EC2Config{
					AccessKey: os.Getenv("access_key"),
					AwsUser:   os.Getenv("aws_user"),
				},
			},
		}, nil
	}

	var (
		serverIPs []string
		agentIPs  []string
	)

	// separate the nodes IPs based on roles.
	for i := range nodes {
		if nodes[i].Roles == "<none>" && nodes[i].Roles != "control-plane" {
			agentIPs = append(agentIPs, nodes[i].ExternalIP)
		} else {
			serverIPs = append(serverIPs, nodes[i].ExternalIP)
		}
	}

	return &Cluster{
		Status:     "cluster created",
		ServerIPs:  serverIPs,
		AgentIPs:   agentIPs,
		NumAgents:  len(agentIPs),
		NumServers: len(serverIPs),
		Aws: AwsConfig{
			Region: os.Getenv("region"),
			EC2Config: EC2Config{
				AccessKey: os.Getenv("access_key"),
				AwsUser:   os.Getenv("aws_user"),
				Ami:       os.Getenv("aws_ami"),

				VolumeSize:       os.Getenv("volume_size"),
				InstanceClass:    os.Getenv("ec2_instance_class"),
				Subnets:          os.Getenv("subnets"),
				AvailabilityZone: os.Getenv("availability_zone"),
				SgId:             os.Getenv("sg_id"),
				KeyName:          os.Getenv("key_name"),
			},
		},
		Config: clusterConfig{
			Product:          os.Getenv("ENV_PRODUCT"),
			RenderedTemplate: os.Getenv("rendered_template"),
			DataStore:        os.Getenv("datastore_type"),
			ExternalDb:       os.Getenv("external_db"),
			Arch:             os.Getenv("arch"),
		},
		BastionConfig: bastionConfig{
			PublicIPv4Addr: os.Getenv("BASTION_IP"),
		},
	}, nil
}

// newCluster creates a new cluster and returns his values from terraform config and vars.
func newCluster(product, module string) (*Cluster, error) {
	terraformOptions, varDir, err := setTerraformOptions(product, module)
	if err != nil {
		return nil, err
	}

	t := &testing.T{}
	numServers, err := strconv.Atoi(terraform.GetVariableAsStringFromVarFile(
		t,
		varDir,
		"no_of_server_nodes",
	))
	if err != nil {
		return nil, fmt.Errorf(
			"error getting no_of_server_nodes from var file: %w", err)
	}

	numAgents, err := strconv.Atoi(terraform.GetVariableAsStringFromVarFile(
		t,
		varDir,
		"no_of_worker_nodes",
	))
	if err != nil {
		return nil, fmt.Errorf(
			"error getting no_of_worker_nodes from var file: %w", err)
	}

	LogLevel("info", "Applying Terraform config and Creating cluster\n")
	_, err = terraform.InitAndApplyE(t, terraformOptions)
	if err != nil {
		return nil, fmt.Errorf("\nTerraform apply Failed: %w", err)
	}
	LogLevel("info", "Applying Terraform config completed!\n")

	LogLevel("info", "Checking and adding split roles...")
	numServers, err = addSplitRole(t, varDir, numServers)
	if err != nil {
		return nil, err
	}

	LogLevel("info", "Loading TF Configs...")
	c, err := loadTFconfig(t, product, module, varDir, terraformOptions)
	if err != nil {
		return nil, err
	}

	c.NumServers = numServers
	c.NumAgents = numAgents
	c.Status = "cluster created"
	LogLevel("info", "Cluster has been created successfully...")

	return c, nil
}

// DestroyCluster destroys the cluster and returns it.
func DestroyCluster() (string, error) {
	terraformOptions, _, err := setTerraformOptions(product, module)
	if err != nil {
		return "", err
	}
	terraform.Destroy(&testing.T{}, terraformOptions)

	return "cluster destroyed", nil
}
