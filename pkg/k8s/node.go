package k8s

import (
	"context"
	"errors"
	"fmt"

	v1 "k8s.io/api/core/v1"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/rancher/distros-test-framework/shared"
)

// WaitForNodesReady validates readiness of nodes by checking how much/if/which nodes are ready, with a minimum threshold.
//
// minReadyNodes is the minimum number of ready nodes required for the cluster to be considered healthy.
//
// if minReadyNodes is 0, it will be set to the number of nodes in the cluster.
func (k *Client) WaitForNodesReady(minReadyNodes int) error {
	readyNodesMap, nodesReady, nodesTotal, minReadyNodes, err := k.checkInitialNodesReady(minReadyNodes)
	if err != nil {
		return fmt.Errorf("failed to check initial nodes ready: %w", err)
	}

	shared.LogLevel("info", "Initial nodes ready/total: %d/%d", nodesReady, nodesTotal)

	if nodesReady >= minReadyNodes {
		shared.LogLevel("info", "Required number of nodes are already ready: %d/%d", nodesReady, nodesTotal)
		return nil
	}

	shared.LogLevel("info", "Waiting for nodes to become ready... (%d/%d ready)", nodesReady, nodesTotal)

	err = k.watchNodesReady(context.Background(), readyNodesMap, &nodesReady, nodesTotal, minReadyNodes)
	if err != nil {
		return fmt.Errorf("failed to watch nodes ready: %w", err)
	}

	return nil
}

// checkInitialNodesReady receive the amount of minReadyNodes.
// checks the initial state of the nodes and returns:
// - nodeMap: a map of node names to their readiness status.
// - ready: the number of nodes that are ready.
// - total: the total number of nodes in the cluster.
// - minNode: the minimum number of nodes required to be ready.
func (k *Client) checkInitialNodesReady(minReadyNodes int) (
	nodeMap map[string]bool,
	ready int,
	total int,
	minNode int,
	err error,
) {
	nodeList, err := k.ListResources(ResourceTypeNode, "", "")
	if err != nil {
		return nil, 0, 0, 0, fmt.Errorf("failed to list nodes: %w", err)
	}

	nodesTotal := len(nodeList.Items)
	if nodesTotal == 0 {
		return nil, 0, 0, 0, errors.New("no nodes found")
	}

	if minReadyNodes == 0 || minReadyNodes > nodesTotal {
		minReadyNodes = nodesTotal
	}

	nodesReady := 0
	readyNodesMap := make(map[string]bool)

	for _, res := range nodeList.Items {
		var node v1.Node
		convertErr := runtime.DefaultUnstructuredConverter.FromUnstructured(res.Object, &node)
		if convertErr != nil {
			return nil, 0, 0, 0, fmt.Errorf("failed to convert to Node: %w", convertErr)
		}

		nodeCurrentReady := nodeReady(&node)
		readyNodesMap[node.Name] = nodeCurrentReady
		if nodeCurrentReady {
			nodesReady++
		}
	}

	return readyNodesMap, nodesReady, nodesTotal, minReadyNodes, nil
}

// watchNodesReady watches the nodes and updates the ready count based on:
//
// minReadyNodes is the minimum number of ready nodes required for the cluster to be considered healthy.
// nodesReady is the number of nodes that are ready.
// nodesTotal is the total number of nodes in the cluster.
func (k *Client) watchNodesReady(
	ctx context.Context,
	readyNodesMap map[string]bool,
	nodesReady *int,
	nodesTotal, minReadyNodes int,
) error {
	gvr, err := k.getGVR(ResourceTypeNode)
	if err != nil {
		return fmt.Errorf("failed to get GVR: %w", err)
	}

	resource := k.DynamicClient.Resource(gvr)
	watcher, err := resource.Watch(ctx, meta.ListOptions{})
	if err != nil {
		return fmt.Errorf("failed to set up watch: %w", err)
	}
	defer watcher.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case event, ok := <-watcher.ResultChan():
			if !ok {
				return errors.New("watcher channel closed")
			}

			objUnstructured, ok := event.Object.(*unstructured.Unstructured)
			if !ok {
				return fmt.Errorf("unexpected type %T", event.Object)
			}

			var node v1.Node
			convertErr := runtime.DefaultUnstructuredConverter.FromUnstructured(objUnstructured.Object, &node)
			if convertErr != nil {
				return fmt.Errorf("failed to convert to Node: %w", convertErr)
			}

			nodePreviousReady := readyNodesMap[node.Name]
			nodeCurrentReady := nodeReady(&node)
			readyNodesMap[node.Name] = nodeCurrentReady

			if !nodePreviousReady && nodeCurrentReady {
				*nodesReady++
				shared.LogLevel("info", "Node %s became ready (%d/%d)", node.Name, *nodesReady, nodesTotal)
			} else if nodePreviousReady && !nodeCurrentReady {
				*nodesReady--
				shared.LogLevel("info", "Node %s is no longer ready (%d/%d)", node.Name, *nodesReady, nodesTotal)
			}

			if *nodesReady >= minReadyNodes {
				shared.LogLevel("info", "Required number of nodes are now ready: %d/%d", *nodesReady, nodesTotal)
				return nil
			}
		}
	}
}

func nodeReady(node *v1.Node) bool {
	for _, condition := range node.Status.Conditions {
		if condition.Type == v1.NodeReady && condition.Status == v1.ConditionTrue {
			return true
		}
	}

	return false
}
