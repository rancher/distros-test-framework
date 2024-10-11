package k8s

import (
	"context"
	"fmt"

	"strings"

	meta "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/rancher/distros-test-framework/shared"
)

type Client struct {
	Clientset *kubernetes.Clientset
}

func Add() (*Client, error) {
	config, err := clientcmd.BuildConfigFromFlags("", shared.KubeConfigFile)
	if err != nil {
		return nil, fmt.Errorf("failed to build config from kubeconfig: %v", err)
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create Kubernetes clientset: %v", err)
	}

	return &Client{
		Clientset: clientset,
	}, nil
}

func (k *Client) ListResources(resourceType, namespace, labelSelector string) (interface{}, error) {
	listOptions := meta.ListOptions{
		LabelSelector: labelSelector,
		Watch:         true,
	}

	switch resourceType {
	case "pods":
		pods, err := k.Clientset.CoreV1().Pods(namespace).List(context.TODO(), listOptions)
		if err != nil {
			return nil, fmt.Errorf("failed to list pods: %v", err)
		}

		return pods.Items, nil
	case "deployments":
		deployments, err := k.Clientset.AppsV1().Deployments(namespace).List(context.TODO(), listOptions)
		if err != nil {
			return nil, fmt.Errorf("failed to list deployments: %v", err)
		}

		return deployments.Items, nil
	case "services":
		services, err := k.Clientset.CoreV1().Services(namespace).List(context.TODO(), listOptions)
		if err != nil {
			return nil, fmt.Errorf("failed to list services: %v", err)
		}

		return services.Items, nil

	default:
		return nil, fmt.Errorf("resource type %s not implemented", resourceType)
	}
}

func (k *Client) WatchResources(ctx context.Context, namespace, resourceType, labelSelector string) (bool, error) {
	var watcher watch.Interface
	var err error

	listOptions := meta.ListOptions{
		LabelSelector: labelSelector,
		Watch:         true,
	}

	switch resourceType {
	case strings.ToLower("pod"):
		watcher, err = k.Clientset.CoreV1().Pods(namespace).Watch(ctx, listOptions)
	case strings.ToLower("deployment"):
		watcher, err = k.Clientset.AppsV1().Deployments(namespace).Watch(ctx, listOptions)
	case strings.ToLower("service"):
		watcher, err = k.Clientset.CoreV1().Services(namespace).Watch(ctx, listOptions)
	case strings.ToLower("configMap"):
		watcher, err = k.Clientset.CoreV1().ConfigMaps(namespace).Watch(ctx, listOptions)
	case strings.ToLower("node"):
		watcher, err = k.Clientset.CoreV1().Nodes().Watch(ctx, listOptions)
	default:
		return false, fmt.Errorf("resource type %s not implemented", resourceType)
	}

	if err != nil {
		return false, fmt.Errorf("failed to watch resource %s: %v", resourceType, err)
	}

	defer watcher.Stop()

	for {
		select {
		case <-ctx.Done():
			return false, fmt.Errorf("timeout or context canceled while watching %s", resourceType)
		case event, ok := <-watcher.ResultChan():
			if !ok {
				return false, fmt.Errorf("%s watcher channel closed", resourceType)
			}
			shared.LogLevel("info", "Event Type: %v, Object: %v", event.Type, event.Object)

			fmt.Printf(event.Object.GetObjectKind().GroupVersionKind().String())
			fmt.Printf(event.Object.GetObjectKind().GroupVersionKind().Kind)
			fmt.Printf(event.Object.GetObjectKind().GroupVersionKind().Group)
			fmt.Printf(event.Object.GetObjectKind().GroupVersionKind().Version)

			return true, nil
		}
	}
}

//
// func processEvent(event interface{}) error {
// 	obj, ok := event.(watch.Event)
// 	if !ok {
// 		return fmt.Errorf("failed to convert event to watch.Event")
// 	}
//
// 	a := obj.Object.(*
//
// 	name := obj.GetName()
// 	namespace := obj.GetNamespace()
//
// 	shared.LogLevel("info", "Event Type: %v, Resource Name: %s, Namespace: %s", event.Type, name, namespace)
//
// 	spec, found, err := unstructured.NestedMap(obj.Object, "spec")
// 	if err != nil || !found {
// 		return fmt.Errorf("failed to get spec: %v", err)
// 	}
//
// 	shared.LogLevel("info", "Resource Spec: %v", spec)
//
// 	err = unstructured.SetNestedMap(obj.Object, spec, "spec")
// 	if err != nil {
// 		return fmt.Errorf("failed to set spec: %v", err)
// 	}
//
// 	return nil
// }
