package activity

import (
	"fmt"
	"path/filepath"
	"strconv"
	"sync"
	"strings"
	"github.com/gruntwork-io/terratest/modules/terraform"
	"github.com/rancher/distros-test-framework/lib/shared"

	. "github.com/onsi/ginkgo/v2"
)

type Cluster struct {
	Status           string
	ServerIPs        []string
	AgentIPs         []string
	NumServers       int
	NumAgents        int
	RenderedTemplate string
	ExternalDb       string
	ClusterType      string
	Datastore		 string
}

var (
	once    sync.Once
	cluster *Cluster
)

// NewCluster creates a new cluster and returns his values from terraform config and vars
func createCluster(g GinkgoTInterface, product string) (*Cluster, error) {
	tfDir, err := filepath.Abs(shared.BasePath() + fmt.Sprintf("/distros/%s/modules", product))
	if err != nil {
		return nil, err
	}

	varDir, err := filepath.Abs(shared.BasePath() + fmt.Sprintf("/distros/config/%s.local.tfvars", product))
	if err != nil {
		return nil, err
	}

	ClusterType := terraform.GetVariableAsStringFromVarFile(g, varDir, "cluster_type")

	terraformOptions := &terraform.Options{
		TerraformDir: tfDir,
		VarFiles:     []string{varDir},
	}

	NumServers, err := strconv.Atoi(terraform.GetVariableAsStringFromVarFile(g, varDir, "no_of_server_nodes"))
	if err != nil {
		return nil, err
	}

	NumAgents, err := strconv.Atoi(terraform.GetVariableAsStringFromVarFile(g, varDir, "no_of_worker_nodes"))
	if err != nil {
		return nil, err
	}

	splitRoles := terraform.GetVariableAsStringFromVarFile(g, varDir, "split_roles")
	if splitRoles == "true" {
		etcdNodes, err := strconv.Atoi(terraform.GetVariableAsStringFromVarFile(g, varDir,
			"etcd_only_nodes"))
		if err != nil {
			return nil, err
		}
		etcdCpNodes, err := strconv.Atoi(terraform.GetVariableAsStringFromVarFile(g, varDir,
			"etcd_cp_nodes"))
		if err != nil {
			return nil, err
		}
		etcdWorkerNodes, err := strconv.Atoi(terraform.GetVariableAsStringFromVarFile(g, varDir,
			"etcd_worker_nodes"))
		if err != nil {
			return nil, err
		}
		cpNodes, err := strconv.Atoi(terraform.GetVariableAsStringFromVarFile(g, varDir,
			"cp_only_nodes"))
		if err != nil {
			return nil, err
		}
		cpWorkerNodes, err := strconv.Atoi(terraform.GetVariableAsStringFromVarFile(g, varDir,
			"cp_worker_nodes"))
		if err != nil {
			return nil, err
		}
		NumServers = NumServers + etcdNodes + etcdCpNodes + etcdWorkerNodes +
			+cpNodes + cpWorkerNodes
	}

	fmt.Println("Creating Cluster")

	terraform.InitAndApply(g, terraformOptions)

	ServerIPs := strings.Split(terraform.Output(g, terraformOptions, "master_ips"), ",")
	AgentIPs := strings.Split(terraform.Output(g, terraformOptions, "worker_ips"), ",")
	
	shared.AwsUser = terraform.GetVariableAsStringFromVarFile(g, varDir, "aws_user")
	shared.AccessKey = terraform.GetVariableAsStringFromVarFile(g, varDir, "access_key")
	shared.KubeConfigFile = terraform.Output(g, terraformOptions, "kubeconfig")

	if product == "k3s" {
		Datastore := terraform.GetVariableAsStringFromVarFile(g, varDir, "datastore")
		ExternalDb := terraform.GetVariableAsStringFromVarFile(g, varDir, "external_db")
		RenderedTemplate := terraform.Output(g, terraformOptions, "rendered_template")
		return &Cluster{
			Status:           "cluster created",
			ServerIPs:        ServerIPs,
			AgentIPs:         AgentIPs,
			NumServers:       NumServers,
			NumAgents:        NumAgents,
			RenderedTemplate: RenderedTemplate,
			ExternalDb:       ExternalDb,
			ClusterType:      ClusterType,
			Datastore: 	  	  Datastore,
		}, nil
	} else {
		return &Cluster{
			Status:           "cluster created",
			ServerIPs:        ServerIPs,
			AgentIPs:         AgentIPs,
			NumServers:       NumServers,
			NumAgents:        NumAgents,
			ClusterType:      ClusterType,
		}, nil
	}
	
}

// GetCluster returns a singleton cluster
func GetCluster(g GinkgoTInterface, product string) *Cluster {
	var err error
	fmt.Println("Product GetCluster: ", product)
	once.Do(func() {
		cluster, err = createCluster(g, product)
		if err != nil {
			g.Errorf("error getting cluster: %v", err)
		}
	})
	return cluster
}

// DestroyCluster destroys the cluster and returns a message
func DestroyCluster(g GinkgoTInterface, product string) (string, error) {
	basepath := shared.BasePath()
	tfDir, err := filepath.Abs(basepath + fmt.Sprintf("/distros/%s/modules", product))
	if err != nil {
		return "", err
	}
	varDir, err := filepath.Abs(basepath + fmt.Sprintf("/distros/config/%s.local.tfvars", product))
	if err != nil {
		return "", err
	}

	terraformOptions := terraform.Options{
		TerraformDir: tfDir,
		VarFiles:     []string{varDir},
	}
	terraform.Destroy(g, &terraformOptions)

	return "cluster destroyed", nil
}
