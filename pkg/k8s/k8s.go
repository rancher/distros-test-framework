package k8s

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/avast/retry-go"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/rancher/distros-test-framework/shared"
)

type Client struct {
	Clientset     *kubernetes.Clientset
	DynamicClient dynamic.Interface
}

func AddClient() (*Client, error) {
	config, err := clientcmd.BuildConfigFromFlags("", shared.KubeConfigFile)
	if err != nil {
		return nil, fmt.Errorf("failed to build config from kubeconfig: %w", err)
	}

	clientset, configErr := kubernetes.NewForConfig(config)
	if configErr != nil {
		return nil, fmt.Errorf("failed to create Kubernetes clientset: %w", configErr)
	}

	return &Client{
		Clientset:     clientset,
		DynamicClient: dynamic.NewForConfigOrDie(config),
	}, nil
}

// CheckClusterHealth checks the health of the cluster by checking the API server and node status.
//
// minReadyNodes is the minimum number of ready nodes required for the cluster to be considered healthy.
//
// if minReadyNodes is 0, it will be set to the number of nodes in the cluster.
//
// if ip and port not passed, it will check the health of the current cluster context.
func (k *Client) CheckClusterHealth(minReadyNodes int) (bool, error) {
	res, err := k.GetAPIServerHealth()
	if err != nil {
		return false, fmt.Errorf("API server health check failed: %w", err)
	}

	if nodesErr := k.WaitForNodesReady(minReadyNodes); nodesErr != nil {
		return false, fmt.Errorf("node status check failed: %w", nodesErr)
	}

	if !strings.Contains(res, "ok") {
		return false, fmt.Errorf("API server health check failed: %s", res)
	}

	return true, nil
}

// ListResources search the resource type on preferred resources using the GVR and returns a list of resources.
//
// it uses the namespace or/and labelSelector to filter the resources.
func (k *Client) ListResources(
	resourceType ResourceType,
	namespace, labelSelector string,
) (*unstructured.UnstructuredList, error) {
	gvr, err := k.getGVR(resourceType)
	if err != nil {
		return nil, fmt.Errorf("failed to get GVR: %w", err)
	}

	var res dynamic.ResourceInterface
	if namespace != "" {
		res = k.DynamicClient.Resource(gvr).Namespace(namespace)
	} else {
		res = k.DynamicClient.Resource(gvr)
	}

	ctx := context.Background()
	list, err := res.List(ctx, meta.ListOptions{
		LabelSelector: labelSelector,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list resources: %w", err)
	}

	return list, nil
}

// GetAPIServerHealth checks the health of the API server by sending a GET request to /healthz.
func (k *Client) GetAPIServerHealth() (string, error) {
	var (
		response string
		err      error
	)

	err = retry.Do(
		func() error {
			restClient := k.Clientset.RESTClient()
			req := restClient.Get().AbsPath("/healthz")

			result := req.Do(context.Background())
			rawResponse, resErr := result.Raw()
			if resErr != nil {
				return fmt.Errorf("failed to get API server health: %w", resErr)
			}

			response = string(rawResponse)
			if response != "ok" {
				return fmt.Errorf("API server health check failed: %s", response)
			}

			return nil
		},
		retry.Attempts(21),
		retry.Delay(3*time.Second),
		retry.DelayType(retry.FixedDelay),
	)
	if err != nil {
		return "", fmt.Errorf("failed to get API server health: %w", err)
	}

	return response, nil
}

// getGVR gets the GroupVersionResource for the specified resource type.
func (k *Client) getGVR(resourceType ResourceType) (schema.GroupVersionResource, error) {
	var gvr schema.GroupVersionResource

	err := retry.Do(
		func() error {
			discovery := k.Clientset.Discovery()
			apiResourceList, err := discovery.ServerPreferredResources()
			if err != nil {
				return fmt.Errorf("failed to get preferred resources: %w", err)
			}

			for _, apiResource := range apiResourceList {
				groupVersion, parseErr := schema.ParseGroupVersion(apiResource.GroupVersion)
				if parseErr != nil {
					continue
				}

				for i := range apiResource.APIResources {
					resource := &apiResource.APIResources[i]

					if resource.Kind == string(resourceType) {
						gvr = groupVersion.WithResource(resource.Name)

						return nil
					}
				}
			}

			return fmt.Errorf("resource type %s not found", resourceType)
		},
		retry.Attempts(21),
		retry.Delay(3*time.Second),
		retry.DelayType(retry.FixedDelay),
		retry.OnRetry(func(n uint, err error) {
			if n == 0 || n == 20 {
				shared.LogLevel("warn", "Failed to get preferred resources: Attempt-%v\nError: %v", n+1, err)
			}
		}),
	)
	if err != nil {
		return schema.GroupVersionResource{}, fmt.Errorf("resource type %s not found", resourceType)
	}

	return gvr, nil
}
