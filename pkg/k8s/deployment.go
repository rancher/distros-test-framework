package k8s

import (
	"fmt"

	apps "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/rancher/distros-test-framework/shared"
)

// ListDeployments returns a list of deployment names in the given namespace.
//
// It can filter by labelSelector or send an empty string to list all deployments.
//
// also checks if the deployment is available.
func (k *Client) ListDeployments(namespace, labelSelector string) ([]string, error) {
	deploymentList, err := k.ListResources(ResourceTypeDeployment, namespace, labelSelector)
	if err != nil {
		return nil, fmt.Errorf("failed to list deployments: %w", err)
	}

	var deployments []apps.Deployment

	for _, res := range deploymentList.Items {
		var deployment apps.Deployment
		convertErr := runtime.DefaultUnstructuredConverter.FromUnstructured(res.Object, &deployment)
		if convertErr != nil {
			return nil, fmt.Errorf("failed to convert to Deployment: %w", convertErr)
		}

		isDeploymentAvailable(&deployment)

		deployments = append(deployments, deployment)
	}

	return getDeploymentNames(deployments), nil
}

func getDeploymentNames(deployments []apps.Deployment) []string {
	var names []string
	for i := range deployments {
		names = append(names, deployments[i].Name)
	}

	return names
}

func isDeploymentAvailable(deployment *apps.Deployment) bool {
	switch {
	case deployment.Status.AvailableReplicas < *deployment.Spec.Replicas:
		shared.LogLevel("info", "Deployment %s is not available", deployment.Name)

		return false
	case deployment.Status.AvailableReplicas >= *deployment.Spec.Replicas:
		shared.LogLevel("info", "Deployment %s is available", deployment.Name)

		return true

	default:
		return false
	}
}
