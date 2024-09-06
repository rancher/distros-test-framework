package shared

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"sync"
	"testing"

	"github.com/gruntwork-io/terratest/modules/terraform"

	"github.com/rancher/distros-test-framework/config"
	"github.com/rancher/distros-test-framework/pkg/customflag"
)

var (
	once    sync.Once
	cluster *Cluster
)

type Cluster struct {
	Status    string
	ServerIPs []string
	AgentIPs  []string
	// EIPs          []string
	WinAgentIPs   []string
	NumWinAgents  int
	NumServers    int
	NumAgents     int
	FQDN          string
	Config        clusterConfig
	AwsEc2        awsEc2Config
	GeneralConfig generalConfig
}

type awsEc2Config struct {
	AccessKey        string
	AwsUser          string
	Ami              string
	Region           string
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
}

type generalConfig struct {
	BastionIP string
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

// ClusterConfig returns a singleton cluster with all terraform config and vars.
func ClusterConfig() *Cluster {
	once.Do(func() {
		var err error
		cluster, err = newCluster()
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
			AwsEc2: awsEc2Config{
				AccessKey: os.Getenv("access_key"),
				AwsUser:   os.Getenv("aws_user"),
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
		// EIPs:       awsDependencies.GetElasticIps(cluster.ServerIPs),
		AwsEc2: awsEc2Config{
			AccessKey:        os.Getenv("access_key"),
			AwsUser:          os.Getenv("aws_user"),
			Ami:              os.Getenv("aws_ami"),
			Region:           os.Getenv("region"),
			VolumeSize:       os.Getenv("volume_size"),
			InstanceClass:    os.Getenv("ec2_instance_class"),
			Subnets:          os.Getenv("subnets"),
			AvailabilityZone: os.Getenv("availability_zone"),
			SgId:             os.Getenv("sg_id"),
			KeyName:          os.Getenv("key_name"),
		},
		Config: clusterConfig{
			Product:          os.Getenv("ENV_PRODUCT"),
			RenderedTemplate: os.Getenv("rendered_template"),
			DataStore:        os.Getenv("datastore_type"),
			ExternalDb:       os.Getenv("external_db"),
			Arch:             os.Getenv("arch"),
		},
		GeneralConfig: generalConfig{
			BastionIP: os.Getenv("BASTION_IP"),
		},
	}, nil
}

// newCluster creates a new cluster and returns his values from terraform config and vars.
func newCluster() (*Cluster, error) {
	product := os.Getenv("ENV_PRODUCT")
	terraformOptions, varDir, err := addTerraformOptions(product)
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
			"error getting no_of_worker_nodes from var file: %w\n", err)
	}

	LogLevel("info", "Applying Terraform config and Creating cluster\n")
	_, err = terraform.InitAndApplyE(t, terraformOptions)
	if err != nil {
		return nil, fmt.Errorf("\nTerraform apply Failed: %w", err)
	}

	numServers, err = addSplitRole(t, varDir, numServers)
	if err != nil {
		return nil, err
	}

	c, err := loadTFconfig(t, varDir, terraformOptions, product)
	if err != nil {
		return nil, err
	}

	c.NumServers = numServers
	c.NumAgents = numAgents
	c.Status = "cluster created"

	return c, nil
}

// DestroyCluster destroys the cluster and returns it.
func DestroyCluster() (string, error) {
	cfg, err := config.AddEnv()
	if err != nil {
		return "", err
	}

	_, callerFilePath, _, _ := runtime.Caller(0)
	dir := filepath.Join(filepath.Dir(callerFilePath), "..")
	varDir, err := filepath.Abs(dir +
		fmt.Sprintf("/config/%s.tfvars", cfg.Product))
	if err != nil {
		return "", fmt.Errorf("invalid product: %s\n", cfg.Product)
	}

	tfDir, err := filepath.Abs(dir + "/modules/" + cfg.Product)
	if err != nil {
		return "", fmt.Errorf("no module found for product: %s\n", cfg.Product)
	}

	terraformOptions := terraform.Options{
		TerraformDir: tfDir,
		VarFiles:     []string{varDir},
	}
	terraform.Destroy(&testing.T{}, &terraformOptions)

	return "cluster destroyed", nil
}
