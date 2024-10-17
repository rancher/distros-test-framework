package k8s

import (
	"fmt"
	"reflect"

	apps "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

func (k *Client) ListDeployments(namespace, labelSelector string) ([]apps.Deployment, error) {
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
		deployments = append(deployments, deployment)
	}

	return deployments, nil
}

// CompareDeployments compares two slices of deployments and returns true if they are the same.
func CompareDeployments(oldDeployments, newDeployments []apps.Deployment) bool {
	if len(oldDeployments) != len(newDeployments) {
		return false
	}

	oldMap := make(map[string]apps.Deployment)
	for _, dep := range oldDeployments {
		key := fmt.Sprintf("%s/%s", dep.Namespace, dep.Name)
		oldMap[key] = dep
	}

	for _, dep := range newDeployments {
		key := fmt.Sprintf("%s/%s", dep.Namespace, dep.Name)
		if _, exists := oldMap[key]; !exists {
			return false
		}
	}

	return true
}

// GetDeploymentDifferences to get the differences between two slices of deployments.
func GetDeploymentDifferences(oldDeployments, newDeployments []apps.Deployment) (added, removed []apps.Deployment) {
	oldMap := make(map[string]apps.Deployment)
	for _, dep := range oldDeployments {
		key := fmt.Sprintf("%s/%s", dep.Namespace, dep.Name)
		oldMap[key] = dep
	}

	newMap := make(map[string]apps.Deployment)
	for _, dep := range newDeployments {
		key := fmt.Sprintf("%s/%s", dep.Namespace, dep.Name)
		newMap[key] = dep
	}

	for key, dep := range newMap {
		if _, exists := oldMap[key]; !exists {
			added = append(added, dep)
		}
	}

	for key, dep := range oldMap {
		if _, exists := newMap[key]; !exists {
			removed = append(removed, dep)
		}
	}

	return added, removed
}

// DeepCompareDeployments compares two slices of deployments deeply.
func DeepCompareDeployments(oldDeployments, newDeployments []apps.Deployment) bool {
	if len(oldDeployments) != len(newDeployments) {
		return false
	}

	oldMap := make(map[string]apps.Deployment)
	for _, dep := range oldDeployments {
		key := fmt.Sprintf("%s/%s", dep.Namespace, dep.Name)
		oldMap[key] = dep
	}

	for _, dep := range newDeployments {
		key := fmt.Sprintf("%s/%s", dep.Namespace, dep.Name)
		if oldDep, exists := oldMap[key]; !exists || !reflect.DeepEqual(dep, oldDep) {
			return false
		}
	}

	return true
}

func getDeploymentNames(deployments []apps.Deployment) []string {
	var names []string
	for _, dep := range deployments {
		names = append(names, fmt.Sprintf("%s/%s", dep.Namespace, dep.Name))
	}

	return names
}

func isDeploymentAvailable(deployment *apps.Deployment) bool {
	return deployment.Status.AvailableReplicas >= *deployment.Spec.Replicas
}

func CompareDeploymentSpecs(oldDep, newDep *apps.Deployment) bool {
	return reflect.DeepEqual(oldDep.Spec, newDep.Spec)
}
