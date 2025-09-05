package qainfra

import (
	"fmt"
	"os"
	"strings"

	"github.com/rancher/distros-test-framework/internal/resources"
)

// InfraNode represents a cluster node from infrastructure
type InfraNode struct {
	Name     string
	PublicIP string
	Role     string
}

// NodeExtractor defines the interface for extracting node information from different providers
type NodeExtractor interface {
	SupportsResourceType(resourceType string) bool
	ExtractNode(resourceValues map[string]interface{}) (*InfraNode, error)
}

// getNodeExtractors returns available node extractors for different providers
func getNodeExtractors() ([]NodeExtractor, error) {
	infraModule := strings.ToLower(strings.TrimSpace(os.Getenv("INFRA_MODULE")))
	switch infraModule {
	case "aws":
		return []NodeExtractor{&AWSNodeExtractor{}}, nil
	case "vsphere":
		return []NodeExtractor{&VSphereNodeExtractor{}}, nil
	default:
		return nil, fmt.Errorf("unknown infrastructure module: %s", infraModule)
	}
}

// AWSNodeExtractor handles AWS-specific node extraction
type AWSNodeExtractor struct{}

func (e *AWSNodeExtractor) SupportsResourceType(resourceType string) bool {
	return resourceType == "aws_instance"
}

func (e *AWSNodeExtractor) ExtractNode(values map[string]interface{}) (*InfraNode, error) {
	node := &InfraNode{}

	// Extract name from tags
	if tags, ok := values["tags"].(map[string]interface{}); ok {
		if name, ok := tags["Name"].(string); ok {
			node.Name = cleanNodeName(name)
		}
	}

	// Extract public IP
	if publicIP, ok := values["public_ip"].(string); ok {
		node.PublicIP = publicIP
	}

	// Determine role based on name
	node.Role = determineNodeRole(node.Name)

	if node.Name == "" || node.PublicIP == "" {
		return nil, fmt.Errorf("incomplete node data: name=%s, ip=%s", node.Name, node.PublicIP)
	}

	resources.LogLevel("debug", "Extracted AWS node: %+v", node)

	return node, nil
}

// VSphereNodeExtractor handles vSphere-specific node extraction
type VSphereNodeExtractor struct{}

func (e *VSphereNodeExtractor) SupportsResourceType(resourceType string) bool {
	return resourceType == "vsphere_virtual_machine"
}

func (e *VSphereNodeExtractor) ExtractNode(values map[string]interface{}) (*InfraNode, error) {
	node := &InfraNode{}

	// Extract name from VM name or custom attributes
	if name, ok := values["name"].(string); ok {
		node.Name = cleanNodeName(name)
	}

	// Extract IP from guest info or network interfaces
	if guestInfo, ok := values["guest_ip_addresses"].([]interface{}); ok && len(guestInfo) > 0 {
		if ip, ok := guestInfo[0].(string); ok {
			node.PublicIP = ip
		}
	}

	// Determine role based on name
	node.Role = determineNodeRole(node.Name)

	if node.Name == "" || node.PublicIP == "" {
		return nil, fmt.Errorf("incomplete vSphere node data: name=%s, ip=%s", node.Name, node.PublicIP)
	}

	return node, nil
}

// determineNodeRole determines the role based on node name patterns
func determineNodeRole(nodeName string) string {
	lowerName := strings.ToLower(nodeName)

	// TODO: needs to add proper logic and support for custom/split roles
	switch {
	case strings.Contains(lowerName, "master") || strings.Contains(lowerName, "control"):
		return "etcd,cp,worker"
	case strings.Contains(lowerName, "worker") || strings.Contains(lowerName, "agent"):
		return "worker"
	default:

		resources.LogLevel("debug", "Unknown node role pattern for '%s', defaulting to worker", nodeName)
		return "worker"
	}
}

// isValidNode checks if a node has all required fields
func isValidNode(node *InfraNode) bool {
	return node.Name != "" && node.PublicIP != "" && node.Role != ""
}

// logExtractedNodes logs the extracted node information
func logExtractedNodes(nodes []InfraNode) {
	resources.LogLevel("info", "Found %d nodes in OpenTofu state", len(nodes))
	for _, node := range nodes {
		resources.LogLevel("info", "Node: %s (%s) - Role: %s", node.Name, node.PublicIP, node.Role)
	}
}

// cleanNodeName removes provider-specific prefixes from node names
func cleanNodeName(rawName string) string {
	nameParts := strings.Split(rawName, "-")
	if len(nameParts) > 2 {
		return strings.Join(nameParts[2:], "-")
	}

	return rawName
}
