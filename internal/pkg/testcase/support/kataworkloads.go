package support

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	core "k8s.io/api/core/v1"
	nodeapi "k8s.io/api/node/v1"
)

// Coexistence implements KATA-03 in both directions, on the same and different workers.
func (s *KataRun) Coexistence(ctx context.Context) error {
	for _, pair := range [][2]string{
		{"kata-local", "runc-local"},
		{"runc-local", "kata-local"},
		{"kata-local", "runc-remote"},
		{"runc-remote", "kata-local"},
	} {
		if err := s.traffic(ctx, pair[0], pair[1]); err != nil {
			return err
		}
	}

	return nil
}

func (s *KataRun) traffic(ctx context.Context, client, backend string) error {
	return pollKata(ctx, 2*time.Minute, func(ctx context.Context) error {
		var endpoints core.Endpoints
		if err := s.get(ctx, &endpoints, "endpoints", backend, "-n", s.ns); err != nil {
			return err
		}
		if len(endpoints.Subsets) != 1 || len(endpoints.Subsets[0].Addresses) != 1 {
			return fmt.Errorf("service %s must have exactly one Ready backend", backend)
		}

		ref := endpoints.Subsets[0].Addresses[0].TargetRef
		if ref == nil || string(ref.UID) != s.pods[backend].UID {
			return fmt.Errorf("service %s endpoint does not identify backend pod UID %s", backend, s.pods[backend].UID)
		}

		name := backend + "." + s.ns + ".svc"
		if _, err := s.exec(ctx, s.pods[client], "nslookup", name); err != nil {
			return fmt.Errorf("DNS %s -> %s: %w", client, backend, err)
		}

		output, err := s.exec(ctx, s.pods[client], "wget", "-T", "10", "-qO-", "http://"+name+":8080")
		if err != nil {
			return fmt.Errorf("HTTP %s -> %s: %w", client, backend, err)
		}

		if strings.TrimSpace(output) != s.ns+"-"+backend {
			return fmt.Errorf("HTTP %s -> %s returned an unexpected backend token", client, backend)
		}

		return s.evidence(client+"-to-"+backend, map[string]string{"dns": name, "token": strings.TrimSpace(output)})
	})
}

func kataSelectorRejects(class *nodeapi.RuntimeClass, node *core.Node) bool {
	if class.Scheduling == nil {
		return false
	}
	for key, value := range class.Scheduling.NodeSelector {
		if node.Labels[key] != value {
			return true
		}
	}

	return false
}

func kataExpectedRejection(pod *core.Pod, events *core.EventList) bool {
	if pod.Spec.NodeName != "" {
		return false
	}

	unscheduled := false
	for _, c := range pod.Status.Conditions {
		if c.Type == core.PodScheduled && c.Status == core.ConditionFalse && c.Reason == "Unschedulable" {
			unscheduled = true
		}
	}

	for i := range events.Items {
		e := &events.Items[i]
		if e.InvolvedObject.UID == pod.UID && e.Reason == "FailedScheduling" && unscheduled &&
			strings.Contains(e.Message, "didn't match Pod's node affinity/selector") {
			return true
		}
	}

	return false
}

// RejectIneligible implements KATA-04; unrelated admission/pull/runtime errors never count as a pass.
func (s *KataRun) RejectIneligible(ctx context.Context) error {
	var class nodeapi.RuntimeClass
	var ordinary core.Node
	if err := s.get(ctx, &class, "runtimeclass", kataClass); err != nil {
		return err
	}

	if err := s.get(ctx, &ordinary, "node", s.pods["runc-remote"].Node); err != nil {
		return err
	}

	if !kataSelectorRejects(&class, &ordinary) {
		return errors.New("ordinary worker is not excluded by the RuntimeClass scheduling selector")
	}

	if _, err := s.ready(ctx, s.ns, "runc-remote"); err != nil {
		return err
	}

	pod := s.workload("kata-rejected", s.ordinary, kataClass, s.opts.Image)
	if err := s.create(ctx, pod.Name, pod); err != nil {
		return err
	}

	rejectionErr := pollKata(ctx, time.Minute, func(ctx context.Context) error {
		var events core.EventList
		if podErr := s.get(ctx, &pod, "pod", pod.Name, "-n", s.ns); podErr != nil {
			return podErr
		}
		selector := "involvedObject.uid=" + string(pod.UID)
		if eventsErr := s.get(ctx, &events, "events", "-n", s.ns, "--field-selector", selector); eventsErr != nil {
			return eventsErr
		}
		if !kataExpectedRejection(&pod, &events) {
			return errors.New("waiting for RuntimeClass selector-specific Unschedulable condition and event")
		}

		return s.evidence("rejection-events", events)
	})
	if rejectionErr != nil {
		return fmt.Errorf("verify scheduling rejection of pod %s: %w", pod.Name, rejectionErr)
	}

	return s.verifyNoSandbox(ctx, &pod)
}

func (s *KataRun) verifyNoSandbox(ctx context.Context, pod *core.Pod) error {
	for range 5 {
		if err := s.get(ctx, pod, "pod", pod.Name, "-n", s.ns); err != nil {
			return err
		}

		if pod.Spec.NodeName != "" {
			return errors.New("negative pod became scheduled")
		}

		for _, ip := range append([]string{s.cluster.ServerIPs[0]}, s.cluster.AgentIPs...) {
			if err := s.noSandbox(ctx, ip, string(pod.UID)); err != nil {
				return err
			}
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(6 * time.Second):
		}
	}
	_, err := s.kubectl(ctx, "delete", "pod", pod.Name, "-n", s.ns, "--wait=false")

	return err
}

// Restart implements KATA-07, checking existing workloads before creating fresh ones.
func (s *KataRun) Restart(ctx context.Context) error {
	if securityErr := s.checkSecurity(ctx, "before-restart"); securityErr != nil {
		return securityErr
	}
	if err := s.evidence("before-restart", s.pods); err != nil {
		return err
	}

	if _, err := s.node(ctx, s.eligibleIP, "sudo", "-n", "systemctl", "restart", "rke2-agent"); err != nil {
		return fmt.Errorf("restart rke2-agent on worker %s: %w", s.eligible, err)
	}

	if err := pollKata(ctx, 5*time.Minute, s.recoverWorkloads); err != nil {
		return fmt.Errorf("recover preexisting workloads after rke2-agent restart: %w", err)
	}

	if chartErr := pollKata(ctx, 5*time.Minute, s.chartReady); chartErr != nil {
		return fmt.Errorf("recover Kata installer after rke2-agent restart: %w", chartErr)
	}
	if securityErr := pollKata(ctx, 3*time.Minute, func(ctx context.Context) error {
		return s.checkSecurity(ctx, "after-restart")
	}); securityErr != nil {
		return securityErr
	}

	if err := s.Coexistence(ctx); err != nil {
		return err
	}

	for _, fixture := range []struct{ name, class string }{{"kata-fresh", kataClass}, {"runc-fresh", ""}} {
		if _, err := s.startPod(ctx, fixture.name, s.eligible, s.eligibleIP, fixture.class, s.opts.Image); err != nil {
			return err
		}
		if err := s.create(ctx, fixture.name+"-service", s.service(fixture.name)); err != nil {
			return err
		}
	}

	if err := s.traffic(ctx, "kata-fresh", "runc-fresh"); err != nil {
		return err
	}

	return s.traffic(ctx, "runc-fresh", "kata-fresh")
}

func (s *KataRun) recoverWorkloads(ctx context.Context) error {
	var node core.Node
	if err := s.get(ctx, &node, "node", s.pods["runc-local"].Node); err != nil {
		return err
	}

	ready := false
	for _, condition := range node.Status.Conditions {
		ready = ready || condition.Type == core.NodeReady && condition.Status == core.ConditionTrue
	}
	if !ready {
		return errors.New("restarted worker is not Ready")
	}

	for name, previous := range s.pods {
		var pod core.Pod
		if err := s.get(ctx, &pod, "pod", name, "-n", previous.Namespace); err != nil {
			return err
		}

		if !kataPodReady(&pod) {
			return errors.New("preexisting workload has not recovered: " + name)
		}

		current := *previous
		current.UID = string(pod.UID)
		identity, err := s.identity(ctx, &current)
		if err != nil {
			return err
		}

		if vmErr := s.checkReplacedVM(ctx, previous, identity); vmErr != nil {
			return fmt.Errorf("check replaced VM for pod %s: %w", name, vmErr)
		}

		current.Identity = identity
		s.pods[name] = &current
	}

	return s.evidence("after-restart", s.pods)
}

func (s *KataRun) checkReplacedVM(ctx context.Context, previous *kataPod, current json.RawMessage) error {
	var before, after kataIdentity
	if beforeErr := json.Unmarshal(previous.Identity, &before); beforeErr != nil {
		return fmt.Errorf("decode pre-restart identity for pod %s: %w", previous.Name, beforeErr)
	}

	if afterErr := json.Unmarshal(current, &after); afterErr != nil {
		return fmt.Errorf("decode post-restart identity for pod %s: %w", previous.Name, afterErr)
	}

	if before.Sandbox != after.Sandbox {
		return s.oldVMsGone(ctx, previous.IP, previous.Identity)
	}

	return nil
}
