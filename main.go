package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/rancher/distros-test-framework/shared"
)

func main() {
	_ = os.Setenv("access_user", "ubuntu")

	// exportKubectl("18.116.50.46")

	// res2, errs2 := shared.RunCommandOnNode(`LL="KUBECONFIG=/etc/rancher/rke2/rke2.yaml"`, "18.116.50.46")
	// if errs2 != nil {
	// 	fmt.Printf("error: %v\n", errs2)
	// }
	// fmt.Println(res2)
	//
	// // res1, errs := shared.RunCommandOnNode("echo PP=$(PATH=$PATH:/var/rancher/rke2/bin:/opt/rke2/bin)", "18.116.50.46")
	// // if errs != nil {
	// // 	fmt.Printf("error: %v\n", errs)
	// // }
	// // fmt.Println(res1)
	//

	serverFlags := "profile: cis\nwrite-kubeconfig-mode: 644\ncni: cilium"

	serverFlags = strings.ReplaceAll(serverFlags, `\n`, "\n")

	tempFile, err := os.Create("/tmp/config.yaml")
	if err != nil {
		fmt.Printf("error creating file: %v\n", err)
		return
	}
	defer tempFile.Close()

	_, writeErr := tempFile.WriteString(fmt.Sprintf("node-external-ip: %s\n", "3.145.151.117"))
	if writeErr != nil {
		fmt.Printf("error: %v\n", writeErr)
	}

	flagEntries := strings.Split(serverFlags, "\n")
	for _, entry := range flagEntries {
		entry = strings.TrimSpace(entry)
		if entry != "" {
			_, err := tempFile.WriteString(fmt.Sprintf("%s\n", entry))
			if err != nil {
				fmt.Printf("error: %v\n", err)
			}

		}
	}

	remoteDir := fmt.Sprintf("/etc/rancher/%s/", "rke2")

	cmd := fmt.Sprintf("sudo mkdir -p %s && sudo chown %s %s ", remoteDir, "ubuntu", remoteDir)
	_, mkdirCmdErr := shared.RunCommandOnNode(cmd, "3.145.151.117")
	if mkdirCmdErr != nil {
		fmt.Printf("error: %v\n", mkdirCmdErr)
	}

	scpErr := RunScp("3.145.151.117", []string{tempFile.Name()}, []string{remoteDir + "config.yaml"})
	if scpErr != nil {
		fmt.Printf("error: %v\n", scpErr)
	}
	//
	// moveCmd := fmt.Sprintf("sudo mv %s %s", tempRemoteFilePath, remoteFilePath)
	// _, moveCmdErr := shared.RunCommandOnNode(moveCmd, "3.145.151.117")
	// if moveCmdErr != nil {
	// 	fmt.Printf("error: %v\n", moveCmdErr)
	// }

	// product := "rke2"
	// createConfigFileCmd := fmt.Sprintf("sudo  bash -c 'cat <<EOF>/etc/rancher/%s/config.yaml\n"+
	// 	"write-kubeconfig-mode: 644\n"+
	// 	"EOF'", product)
	//
	// path := fmt.Sprintf("/etc/rancher/%s/", product)
	// cmd := fmt.Sprintf("sudo  mkdir -p %s && %s", path, createConfigFileCmd)
	// fmt.Println(cmd)
	// res, err := shared.RunCommandOnNode(cmd, "3.144.103.113")
	// if err != nil {
	// 	fmt.Printf("error: %v\n", err)
	// }
	// fmt.Println(res)

	// res, err := shared.RunCommandOnNode("export KUBECONFIG=/etc/rancher/rke2/rke2.yaml && PATH=$PATH:/var/lib/rancher/rke2/bin && /var/lib/rancher/rke2/bin/kubectl get nodes,pods -A -o wide ", "3.144.103.113")
	// if err != nil {
	// 	fmt.Printf("error: %v\n", err)
	// }
	// fmt.Println(res)
}

func RunScp(ip string, localPaths, remotePaths []string) error {
	for i, localPath := range localPaths {
		remotePath := remotePaths[i]
		privateKeyPath := "/Users/moral/jenkins-keys/jenkins-rke-validation.pem "
		scpCmd := fmt.Sprintf(
			"scp -i %s %s %s@%s:%s",
			privateKeyPath,
			localPath,
			"ubuntu",
			ip,
			remotePath,
		)

		res, cmdErr := shared.RunCommandHost(scpCmd)
		fmt.Printf("res: %v\n", res)
		if cmdErr != nil {
			return cmdErr
		}

		chmod := "sudo chmod +wx " + remotePath
		ress, cmdErr := shared.RunCommandOnNode(chmod, ip)
		fmt.Printf("res: %v\n", ress)
		if cmdErr != nil {

			return cmdErr
		}
	}

	return nil
}

func exportKubectl(newClusterIP string) {
	// update data directory for rpm installs (rhel systems)
	exportCmd := fmt.Sprintf("sudo cat <<EOF >>.bashrc\n" +
		"export KUBECONFIG=/etc/rancher/rke2/rke2.yaml PATH=$PATH:/var/lib/rancher/rke2/bin:/opt/rke2/bin " +
		"CRI_CONFIG_FILE=/var/lib/rancher/rke2/agent/etc/crictl.yaml && \n" +
		"alias k=kubectl\n" +
		"EOF")

	sourceCmd := "source .bashrc"

	_, exportCmdErr := shared.RunCommandOnNode(exportCmd, newClusterIP)
	if exportCmdErr != nil {
		fmt.Printf("error: %v\n", exportCmdErr)
	}

	_, sourceCmdErr := shared.RunCommandOnNode(sourceCmd, newClusterIP)
	if sourceCmdErr != nil {
		fmt.Printf("error: %v\n", sourceCmdErr)
	}
}

// kubeconfig = os.Getenv("KUBE_CONFIG")

// c, err := Add()
// if err != nil {
// 	shared.LogLevel("error", "error adding k8s: %w\n", err)
// 	os.Exit(1)
// }
//
// pods, err := c.ListResources("pods", "kube-system", "app=nginx")
// if err != nil {
// 	shared.LogLevel("error", "error listing pods: %w\n", err)
// 	os.Exit(1)
// }
//
// if p, ok := pods.([]v1.Pod); ok {
// 	for _, pod := range p {
// 		fmt.Printf("Pod: %v\n", pod)
// 	}
//
// 	ctx := context.Background()
// 	nodes, err := c.WatchResources(ctx, "kube-system", "node", "")
// 	if err != nil {
// 		shared.LogLevel("error", "error watching nodes: %w\n", err)
// 		os.Exit(1)
// 	}
//
// 	if nodes {
// 		fmt.Printf("Nodes: %v\n", nodes)
// 	}

// path, findErr := FindPath("rke2"+"-killall.sh", "3.135.216.236")
// if findErr != nil {
// 	fmt.Println("failed to find path for product:  error: %w\n", "rke2", findErr)
// }
//
// fmt.Println(path)
// _, err := shared.RunCommandOnNode(fmt.Sprintf("sudo %s", path), "3.135.216.236")

// err := assert.ValidateOnHost(
// 	"curl --max-time 30 -sL --insecure http://3.141.30.78:81/name.html",
// 	"test-loadbalancer",
// )
// if err != nil {
// 	fmt.Println(err)
// }
// fmt.Println(err)/**/

//
// fmt.Println(path)
//
// cmd := fmt.Sprintf("%s -v", path)
// v, err := shared.RunCommandOnNode(cmd, "3.135.216.236")
// if err != nil {
// 	fmt.Println("failed to get version for product:  error: %w\n", "rke2", err)
// }
//
// fmt.Println(v)

// type Client struct {
// 	Clientset *kubernetes.Clientset
// }
//
// func Add() (*Client, error) {
// 	config, err := clientcmd.BuildConfigFromFlags("", shared.KubeConfigFile)
// 	if err != nil {
// 		return nil, fmt.Errorf("failed to build config from kubeconfig: %v", err)
// 	}
//
// 	clientset, err := kubernetes.NewForConfig(config)
// 	if err != nil {
// 		return nil, fmt.Errorf("failed to create Kubernetes clientset: %v", err)
// 	}
//
// 	return &Client{
// 		Clientset: clientset,
// 	}, nil
// }
//
// func (k *Client) ListResources(resourceType, namespace, labelSelector string) (interface{}, error) {
// 	listOptions := meta.ListOptions{
// 		LabelSelector: labelSelector,
// 		Watch:         true,
// 	}
//
// 	switch resourceType {
// 	case "pods":
// 		pods, err := k.Clientset.CoreV1().Pods(namespace).List(context.TODO(), listOptions)
// 		if err != nil {
// 			return nil, fmt.Errorf("failed to list pods: %v", err)
// 		}
//
// 		return pods.Items, nil
// 	case "deployments":
// 		deployments, err := k.Clientset.AppsV1().Deployments(namespace).List(context.TODO(), listOptions)
// 		if err != nil {
// 			return nil, fmt.Errorf("failed to list deployments: %v", err)
// 		}
//
// 		return deployments.Items, nil
// 	case "services":
// 		services, err := k.Clientset.CoreV1().Services(namespace).List(context.TODO(), listOptions)
// 		if err != nil {
// 			return nil, fmt.Errorf("failed to list services: %v", err)
// 		}
//
// 		return services.Items, nil
//
// 	default:
// 		return nil, fmt.Errorf("resource type %s not implemented", resourceType)
// 	}
// }
//
// func (k *Client) WatchResources(ctx context.Context, namespace, resourceType, labelSelector string) (bool, error) {
// 	var watcher watch.Interface
// 	var err error
//
// 	listOptions := meta.ListOptions{
// 		LabelSelector: labelSelector,
// 		Watch:         true,
// 	}
//
// 	switch resourceType {
// 	case strings.ToLower("pod"):
// 		watcher, err = k.Clientset.CoreV1().Pods(namespace).Watch(ctx, listOptions)
// 	case strings.ToLower("deployment"):
// 		watcher, err = k.Clientset.AppsV1().Deployments(namespace).Watch(ctx, listOptions)
// 	case strings.ToLower("service"):
// 		watcher, err = k.Clientset.CoreV1().Services(namespace).Watch(ctx, listOptions)
// 	case strings.ToLower("configMap"):
// 		watcher, err = k.Clientset.CoreV1().ConfigMaps(namespace).Watch(ctx, listOptions)
// 	case strings.ToLower("node"):
// 		watcher, err = k.Clientset.CoreV1().Nodes().Watch(ctx, listOptions)
// 	default:
// 		return false, fmt.Errorf("resource type %s not implemented", resourceType)
// 	}
//
// 	if err != nil {
// 		return false, fmt.Errorf("failed to watch resource %s: %v", resourceType, err)
// 	}
//
// 	defer watcher.Stop()
//
// 	for {
// 		select {
// 		case <-ctx.Done():
// 			return false, fmt.Errorf("timeout or context canceled while watching %s", resourceType)
// 		case event, ok := <-watcher.ResultChan():
// 			if !ok {
// 				return false, fmt.Errorf("%s watcher channel closed", resourceType)
// 			}
// 			shared.LogLevel("info", "Event Type: %v, Object: %v", event.Type, event.Object)
//
// 			fmt.Printf(event.Object.GetObjectKind().GroupVersionKind().String())
// 			fmt.Printf(event.Object.GetObjectKind().GroupVersionKind().Kind)
// 			fmt.Printf(event.Object.GetObjectKind().GroupVersionKind().Group)
// 			fmt.Printf(event.Object.GetObjectKind().GroupVersionKind().Version)
//
// 			return true, nil
// 		}
// 	}
// }
//
// func FindPath(name, ip string) (string, error) {
// 	searchPath := fmt.Sprintf("sudo find / -type f -executable -name %s 2>/dev/null | sed 1q", name)
// 	fullPath, err := shared.RunCommandOnNode(searchPath, ip)
// 	if err != nil {
// 		return "", err
// 	}
//
// 	fullPath = strings.TrimSpace(fullPath)
// 	if fullPath == "" {
// 		return "", fmt.Errorf("script %s not found", name)
// 	}
//
// 	return fullPath, nil
// }
