### K8s go client

## [Client documentation](#client-documentation)
## [How it is implemented](#how-it-is-implemented)
## [What is the purpose](#what-is-the-purpose)
## [What we have](#what-we-have)
## [Next steps](#next-steps)


#### Client documentation
- [K8s client](https://github.com/kubernetes/client-go)


#### How it is implemented

- The client itself is implemented with:
    - `(Clientset     *kubernetes.Clientset)`   : The clientset is the main interface for interacting with the API server.
    - `(DynamicClient dynamic.Interface)`       : The dynamic client is a client that can perform generic operations on arbitrary resources.

- The client is created for now for simplicity wise using `BuildConfigFromFlags` function, which uses our current kubeconfig file to create the clientset.

#### What is the purpose
- The purpose is to have a way to interact with the k8s API server in some use cases where we might encounter ourselves in lack or not being available to use the kubectl command.

- The purpose is to have also a more reliable way to make sure cluster is healthy and ready to be used, after some critical operations.

- The purpose IS NOT to replace kubectl in any shape or form.

- Using enum to handle better the amount of resource types.

#### What we have

- `CheckClusterHealth`  : This function is used to check if the overall cluster is healthy.
- `ListResources`       : This function is used to list resources in a given namespace or not.
- `GetAPIServerHealth`  : This function is used to check if the API server is healthy and ready to be used.
- `WaitForNodesReady`   : This function is used to wait for all nodes to be ready.
- `ListDeployments`     : This function is used to list deployments in a given namespace or not.

- Other functions are basically auxiliary functions to help the main functions to work properly.


#### Next steps
-  Whatever we want! The sky is the limit. Bye. :)
