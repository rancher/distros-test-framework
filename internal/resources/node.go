package resources

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/rancher/distros-test-framework/internal/provisioning/driver"
)

// Node represents a cluster node.
type Node struct {
	Name              string
	Status            string
	Roles             string
	Version           string
	InternalIP        string
	ExternalIP        string
	OperationalSystem string
}

// GetNodes returns nodes parsed from kubectl get nodes.
func GetNodes(display bool) ([]Node, error) {
	res, err := RunCommandHost("kubectl get nodes -o wide --no-headers --kubeconfig=" + KubeConfigFile)
	if err != nil {
		return nil, fmt.Errorf("failed to get nodes: %w", err)
	}

	nodes := ParseNodes(res)
	if display {
		LogLevel("info", "\n\nCluster nodes:\n")
		fmt.Println(res)
	}

	return nodes, nil
}

// GetNodes returns nodes parsed from kubectl get nodes.
// TODO kubectl path needs to be fixed for rke2 different OS.
func GetNodesForK3k(display bool, ip, kubeconfigFile string) ([]Node, error) {
	cmd := "kubectl get nodes -o wide --no-headers --kubeconfig=" + kubeconfigFile
	res, err := RunCommandOnNode(cmd, ip)
	LogLevel("debug", "Running command: \n%s\n", cmd)
	if err != nil {
		return nil, fmt.Errorf("failed to get nodes: %w", err)
	}

	nodes := ParseNodes(res)
	if display {
		LogLevel("info", "\n\nCluster nodes:\n")
		fmt.Println(res)
	}

	return nodes, nil
}

// GetNodesByRoles takes in one or multiple node roles and returns the slice of nodes that have those roles.
// Valid values for roles are: etcd, control-plane, worker.
func GetNodesByRoles(roles ...string) ([]Node, error) {
	var nodes []Node
	var matchedNodes []Node

	if roles == nil {
		return nil, ReturnLogError("no roles provided")
	}

	validRoles := map[string]bool{
		"etcd":          true,
		"control-plane": true,
		"worker":        true,
	}

	for _, role := range roles {
		if !validRoles[role] {
			return nil, ReturnLogError("invalid role: %s", role)
		}

		cmd := "kubectl get nodes -o wide --sort-by '{.metadata.name}'" +
			" --no-headers --kubeconfig=" + KubeConfigFile +
			" -l role-" + role
		res, err := RunCommandHost(cmd)
		if err != nil {
			return nil, err
		}

		matchedNodes = append(matchedNodes, ParseNodes(res)...)
	}

	for i := range matchedNodes {
		nodes = appendNodeIfMissing(nodes, &matchedNodes[i])
	}

	return nodes, nil
}

// ParseNodes returns nodes parsed from kubectl get nodes.
func ParseNodes(res string) []Node {
	nodes := make([]Node, 0, 10)
	nodeList := strings.Split(strings.TrimSpace(res), "\n")
	for _, rec := range nodeList {
		if strings.TrimSpace(rec) == "" {
			continue
		}

		fields := strings.Fields(rec)
		if len(fields) < 8 {
			continue
		}

		n := Node{
			Name:              fields[0],
			Status:            fields[1],
			Roles:             fields[2],
			Version:           fields[4],
			InternalIP:        fields[5],
			ExternalIP:        fields[6],
			OperationalSystem: fields[7],
		}
		nodes = append(nodes, n)
	}

	return nodes
}

// GetNodeArgsMap returns list of nodeArgs map.
func GetNodeArgsMap(cluster *driver.Cluster, nodeType string) (map[string]string, error) {
	res, err := KubectlCommand(
		cluster,
		"host",
		"get",
		"nodes "+
			fmt.Sprintf(
				`-o jsonpath='{range .items[*]}{.metadata.annotations.%s\.io/node-args}{end}'`,
				cluster.Config.Product),
	)
	if err != nil {
		return nil, err
	}

	nodeArgsMapSlice := processNodeArgs(res)

	for _, nodeArgsMap := range nodeArgsMapSlice {
		if nodeArgsMap["node-type"] == nodeType {
			return nodeArgsMap, nil
		}
	}

	return nil, nil
}

func processNodeArgs(nodeArgs string) (nodeArgsMapSlice []map[string]string) {
	nodeArgsSlice := strings.Split(nodeArgs, "]")

	for _, item := range nodeArgsSlice[:(len(nodeArgsSlice) - 1)] {
		items := strings.Split(item, `","`)
		nodeArgsMap := map[string]string{}

		for range items[1:] {
			nodeArgsMap["node-type"] = strings.Trim(items[0], `["`)
			regxCompile := regexp.MustCompile(`--|"`)

			for i := 1; i < len(items); i += 2 {
				if i < (len(items) - 1) {
					key := regxCompile.ReplaceAllString(items[i], "")
					value := regxCompile.ReplaceAllString(items[i+1], "")
					nodeArgsMap[key] = value
				}
			}
		}
		nodeArgsMapSlice = append(nodeArgsMapSlice, nodeArgsMap)
	}

	return nodeArgsMapSlice
}

// DeleteNode deletes a node from the cluster filtering the name out by the IP.
func DeleteNode(ip string) error {
	if ip == "" {
		return ReturnLogError("must send a ip: %s\n", ip)
	}

	name, err := GetNodeNameByIP(ip)
	if err != nil {
		return ReturnLogError("failed to get node name by ip: %w\n", err)
	}

	_, delErr := RunCommandHost("kubectl delete node " + name + " --wait=false  --kubeconfig=" + KubeConfigFile)
	if delErr != nil {
		return ReturnLogError("failed to delete node: %w\n", delErr)
	}

	// delay not meant to wait if node is deleted.
	// but rather to give time for the node to be removed from the cluster.
	delay := time.After(20 * time.Second)
	<-delay

	return nil
}

// GetNodeNameByIP returns the node name by the given IP.
func GetNodeNameByIP(ip string) (string, error) {
	ticker := time.NewTicker(3 * time.Second)
	timeout := time.After(45 * time.Second)
	defer ticker.Stop()

	cmd := "kubectl get nodes -o custom-columns=NAME:.metadata.name,INTERNAL-IP:.status.addresses[*].address --kubeconfig=" +
		KubeConfigFile + " | grep " + ip + " | awk '{print $1}'"

	for {
		select {
		case <-timeout:
			return "", ReturnLogError("kubectl get nodes timed out for cmd: %s\n", cmd)
		case <-ticker.C:
			i := 0
			nodeName, err := RunCommandHost(cmd)
			if err != nil {
				i++
				LogLevel("warn", "error from RunCommandHost: %v\nwith res: %s  Retrying...", err, nodeName)
				if i > 5 {
					return "", ReturnLogError("kubectl get nodes returned error: %w\n", err)
				}

				continue
			}
			if nodeName == "" {
				continue
			}

			return strings.TrimSpace(nodeName), nil
		}
	}
}

// appendNodeIfMissing appends a value to a slice if that value does not already exist in the slice.
func appendNodeIfMissing(slice []Node, i *Node) []Node {
	for _, ele := range slice {
		if ele == *i {
			return slice
		}
	}

	return append(slice, *i)
}
