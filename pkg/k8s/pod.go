package k8s

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/avast/retry-go"
	v1 "k8s.io/api/core/v1"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/rancher/distros-test-framework/shared"
)

var (
	ciliumPodsRunning, ciliumPodsNotRunning int
)

// WaitForPods validates pods that are running or completed on the given namespace.
func (k *Client) WaitForPods(namespaces []string, label string, cluster *shared.Cluster) error {
	if namespaces == nil {
		ns, err := k.ListNamespaces()
		if err != nil {
			return fmt.Errorf("failed to list namespaces: %w", err)
		}
		if len(ns) == 0 {
			return fmt.Errorf("no namespaces found")
		}

		namespaces = ns
	}

	fmt.Printf("Namespaces: %v\n", namespaces)

	podsReady, err := k.validatePods(context.Background(), namespaces, label, cluster)
	if err != nil {
		return fmt.Errorf("failed to list pods and check status: %w", err)
	}

	if !podsReady {
		return fmt.Errorf("pods are not ready")
	} else {
		shared.LogLevel("info", "All pods are ready.")
	}

	return nil
}

func (k *Client) validatePods(ctx context.Context, namespaces []string, label string, cluster *shared.Cluster) (bool, error) {
	var ciliumChecked bool

	retryErr := retry.Do(
		func() error {

			podsReady := true
			for _, namespace := range namespaces {
				pods, err := k.Clientset.CoreV1().Pods(namespace).List(ctx, meta.ListOptions{
					LabelSelector: label,
				})
				if err != nil {
					return fmt.Errorf("failed to list pods: %w", err)
				}

				for _, pod := range pods.Items {
					if !podIsOk(&pod) {
						podsReady = false
						shared.LogLevel("debug", "Pod %s is not ready. Status: %s", pod.Name, pod.Status.Phase)
						break
					}
					shared.LogLevel("debug", "Pod %s is ready. Status: %s", pod.Name, pod.Status.Phase)

					if !ciliumChecked && isCiliumCNI(&pod, cluster) {
						if processCiliumPodStatus(&pod) {
							ciliumChecked = true
						} else {
							podsReady = false
							break
						}
					}
				}

				if !podsReady {
					break
				}
			}

			if podsReady {
				return nil
			}

			return fmt.Errorf("pods are not ready yet")
		},
		retry.Context(ctx),
		retry.Delay(5*time.Second),
		retry.Attempts(39),
	)
	if retryErr != nil {
		return false, fmt.Errorf("failed to validate pods: %w", retryErr)
	}

	shared.LogLevel("info", "All pods are ready.")

	return true, nil
}

// isCiliumCNI checks if there is pod is a cilium-operator under specific cluster conditions like:
//
// RKE2 with a single server and no agents.
func isCiliumCNI(pod *v1.Pod, cluster *shared.Cluster) bool {
	return strings.Contains(pod.Name, "cilium-operator") &&
		cluster.Config.Product == "rke2" &&
		cluster.NumServers == 1 &&
		cluster.NumAgents == 0
}

func processCiliumPodStatus(pod *v1.Pod) bool {
	if pod.Status.Phase == v1.PodPending && podReady(pod) == "0/1" {
		ciliumPodsNotRunning++
	} else if pod.Status.Phase == v1.PodRunning && podReady(pod) == "1/1" {
		ciliumPodsRunning++
	}

	switch {
	case ciliumPodsRunning == 0 && ciliumPodsNotRunning == 1:
		shared.LogLevel("warn", "No Cilium operator pods running yet, only pending.")
		return false

	case ciliumPodsRunning == 0 && ciliumPodsNotRunning > 1:
		shared.LogLevel("error", "No Cilium operator pods running, only pending: Name:%s, Status:%s", pod.Name, pod.Status)
		return false

	case ciliumPodsRunning >= 1 && ciliumPodsNotRunning == 1:
		shared.LogLevel("info", "At least one Cilium operator pod is running.")
		return true

	case ciliumPodsRunning >= 1:
		shared.LogLevel("info", "At least one Cilium operator pod is running.")
		return true

	default:
		shared.LogLevel("error", "No Cilium operator pods running.")
		return false
	}
}

func podIsOk(pod *v1.Pod) bool {
	if pod.Status.Phase == v1.PodRunning || pod.Status.Phase == v1.PodSucceeded {
		readyCount := 0
		for _, c := range pod.Status.ContainerStatuses {
			if c.Ready {
				readyCount++
			}
		}

		return readyCount == len(pod.Status.ContainerStatuses)
	}

	return false
}

// podReady returns the status of the pod in "ready/total" format.
func podReady(pod *v1.Pod) string {
	totalContainers := len(pod.Spec.Containers) + len(pod.Spec.InitContainers)
	readyPodsContainers := 0

	for _, ss := range pod.Status.ContainerStatuses {
		if ss.Ready {
			readyPodsContainers++
		}
	}

	for _, ics := range pod.Status.InitContainerStatuses {
		if ics.Ready {
			readyPodsContainers++
		}
	}

	return fmt.Sprintf("%d/%d", readyPodsContainers, totalContainers)
}
