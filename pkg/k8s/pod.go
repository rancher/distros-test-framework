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

// WaitForPodsReady validates readiness of pods by checking how many/which pods are ready, with a minimum threshold.
func (k *Client) WaitForPodsReady(namespace string) error {
	readyPodsMap, podsReady, podsTotal, err := k.checkInitialPodsReady(namespace)
	if err != nil {
		return fmt.Errorf("failed to check initial pods ready: %w", err)
	}

	shared.LogLevel("info", "Waiting for pods to become ready... (%d/%d ready)", podsReady, podsTotal)

	err = k.watchPodsReady(context.Background(), namespace, readyPodsMap, &podsReady, podsTotal)
	if err != nil {
		return fmt.Errorf("failed to watch pods ready: %w", err)
	}

	return nil
}

// checkInitialPodsReady checks the initial state of the pods.
func (k *Client) checkInitialPodsReady(namespace string) (
	podMap map[string]bool,
	ready int,
	total int,
	err error,
) {
	podList, err := k.ListResources(ResourceTypePod, namespace, "")
	if err != nil {
		return nil, 0, 0, fmt.Errorf("failed to list pods: %w", err)
	}

	podsTotal := len(podList.Items)
	if podsTotal == 0 {
		return nil, 0, 0, errors.New("no pods found")
	}

	podsReady := 0
	readyPodsMap := make(map[string]bool)

	for _, res := range podList.Items {
		var pod v1.Pod
		convertErr := runtime.DefaultUnstructuredConverter.FromUnstructured(res.Object, &pod)
		if convertErr != nil {
			return nil, 0, 0, fmt.Errorf("failed to convert to Pod: %w", convertErr)
		}

		podCurrentReady := podReady(&pod)
		readyPodsMap[pod.Name] = podCurrentReady
		if podCurrentReady {
			podsReady++
		}
	}

	return readyPodsMap, podsReady, podsTotal, nil
}

// watchPodsReady watches the pods and updates the ready count based on:
//
// podsReady is the number of pods that are ready.
// podsTotal is the total number of pods in the namespace.
func (k *Client) watchPodsReady(
	ctx context.Context,
	namespace string,
	readyPodsMap map[string]bool,
	podsReady *int,
	podsTotal int,
) error {
	gvr, err := k.getGVR(ResourceTypePod)
	if err != nil {
		return fmt.Errorf("failed to get GVR: %w", err)
	}

	resource := k.DynamicClient.Resource(gvr).Namespace(namespace)
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

			var pod v1.Pod
			convertErr := runtime.DefaultUnstructuredConverter.FromUnstructured(objUnstructured.Object, &pod)
			if convertErr != nil {
				return fmt.Errorf("failed to convert to Pod: %w", convertErr)
			}

			podPreviousReady := readyPodsMap[pod.Name]
			podCurrentReady := podReady(&pod)
			readyPodsMap[pod.Name] = podCurrentReady

			if !podPreviousReady && podCurrentReady {
				*podsReady++
				shared.LogLevel("info", "Pod %s became ready (%d/%d)", pod.Name, *podsReady, podsTotal)
			} else if podPreviousReady && !podCurrentReady {
				*podsReady--
				shared.LogLevel("info", "Pod %s is no longer ready (%d/%d)", pod.Name, *podsReady, podsTotal)
			}
		}
	}
}

func podReady(pod *v1.Pod) bool {
	for _, condition := range pod.Status.Conditions {
		if condition.Type == v1.PodReady && condition.Status == v1.ConditionTrue {
			return true
		}
	}

	return false
}
