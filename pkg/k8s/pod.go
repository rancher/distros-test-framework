package k8s

// func (k *Client) WaitForPodsRunning(timeout int) error {
// 	readyPodsMap, podsReady, podsTotal, minReadyPods, err := k.checkInitialPodsRunning()
// 	if err != nil {
// 		return fmt.Errorf("failed to check initial pods running: %w", err)
// 	}
//
// 	shared.LogLevel("info", "Initial pods running/total: %d/%d", podsReady, podsTotal)
//
// 	if podsReady >= minReadyPods {
// 		shared.LogLevel("info", "Required number of pods are already running: %d/%d", podsReady, podsTotal)
// 		return nil
// 	}
//
// 	shared.LogLevel("info", "Waiting for pods to become running... (%d/%d running)", podsReady, podsTotal)
//
// 	err = k.watchPodsRunning(context.Background(), readyPodsMap, &podsReady, podsTotal, minReadyPods, timeout)
// 	if err != nil {
// 		return fmt.Errorf("failed to watch pods running: %w", err)
// 	}
//
// 	return nil
// }

// func (k *Client) checkInitialPodsRunning() (
// 	podMap map[string]bool,
// 	ready int,
// 	total int,
// 	minPod int,
// 	err error,
// ) {
// 	podList, err := k.ListResources(ResourceTypePod, "", "")
// 	if err != nil {
// 		return nil, 0, 0, 0, fmt.Errorf("failed to list pods: %w", err)
// 	}
//
// 	return podMap, podsReady, podsTotal, minPod, nil
// }
//
// func (k *Client) watchPodsRunning(ctx context.Context, podMap map[string]bool, ready *int, total int, minPod int, timeout int) error {
// 	return nil
// }
