package shared

import (
	"fmt"
	"strings"
	"time"

	"github.com/avast/retry-go"
)

type ManageService struct {
	MaxRetries    uint
	RetryDelay    time.Duration
	ExplicitDelay time.Duration
}

// ServiceAction represents a service operation to perform on a given node type.
type ServiceAction struct {
	Service       string
	Action        string
	NodeType      string
	ExplicitDelay time.Duration
}

// NewManageService creates a ManageService instance with the given maxRetries and retryDelay parameters.
// maxRetries number of maximum retries on failed command response.
// retryDelay time duration after which the failed command will be retried.
func NewManageService(maxRetries uint, retryDelay time.Duration) *ManageService {
	return &ManageService{
		MaxRetries: maxRetries,
		RetryDelay: retryDelay * time.Second,
	}
}

// ManageService performs a service operations on a node based on the given actions.
//
// Action can be used as a single one or multiple actions.
//
// If action is "rotate", it will rotate the certificate for the given service.
// If action is "status", it will return the status of the service.
//
// ip IP of the node where the systemctl service call is performed.
// actions slice of ServiceAction struct contains all the service actions.
func (ms *ManageService) ManageService(ip string, actions []ServiceAction) (string, error) {
	for _, act := range actions {
		LogLevel("info", "Running %s %s service on node: %s", act.Action, act.Service, ip)
		switch act.Action {
		case "rotate":
			rotateErr := CertRotate(act.Service, ip)

			if rotateErr != nil {
				return "", fmt.Errorf("certificate rotation failed for %s: %w", ip, rotateErr)
			}
			continue

		default:
			var svcName string
			var svcNameErr error
			if strings.Contains(act.Service, "rke2") || strings.Contains(act.Service, "k3s") {
				svcName, svcNameErr = productService(act.Service, act.NodeType)
				if svcNameErr != nil {
					return "", fmt.Errorf("service name failed for %s: %w", ip, svcNameErr)
				}
			} else {
				// if its not rke2 or k3s, then proceed with the service name as is.
				svcName = act.Service
			}

			cmd, err := SystemCtlCmd(svcName, act.Action)
			if err != nil {
				return "", fmt.Errorf("command build failed for %s: %w", ip, err)
			}

			if act.ExplicitDelay > 0 {
				delay := act.ExplicitDelay * time.Second
				LogLevel("info", "Waiting for %v before running systemctl %s %s on node: %s",
					delay, act.Action, svcName, ip)
				<-time.After(delay)
			}

			LogLevel("debug", "Command: %s on node: %s", cmd, ip)
			output, err := ms.execute(cmd, ip)
			if err != nil {
				return "", fmt.Errorf("action %s failed on %s: %w", act.Action, ip, err)
			}

			if act.Action == "status" {
				LogLevel("debug", "service %s output: \n %s", act.Action, output)
				return strings.TrimSpace(output), nil
			}
			LogLevel("info", "Finished running systemctl %s %s on node: %s\n", act.Action, svcName, ip)
			if output != "" {
				LogLevel("debug", "Output: %s", strings.TrimSpace(output))
			}
		}
	}

	return "", nil
}

func (ms *ManageService) execute(cmd, ip string) (string, error) {
	var result string
	var err error

	retryErr := retry.Do(
		func() error {
			result, err = RunCommandOnNode(cmd, ip)
			if err != nil {
				return fmt.Errorf("error running command on %s: %w", ip, err)
			}

			return nil
		},
		retry.Attempts(ms.MaxRetries),
		retry.Delay(ms.RetryDelay),
		retry.OnRetry(func(n uint, err error) {
			if n == 0 || n == ms.MaxRetries-1 {
				LogLevel("warn", "Retry %d/%d failed for %s on %s: %v",
					n+1, ms.MaxRetries, cmd, ip, err)
			}
		}),
	)

	if retryErr != nil {
		return "", fmt.Errorf("%w (last error: %v)", retryErr, err)
	}

	return result, nil
}

func EnableAndStartService(cluster *Cluster, publicIP, nodeType string) error {
	ms := NewManageService(10, 5)
	actions := []ServiceAction{
		{
			Service:  cluster.Config.Product,
			Action:   "enable",
			NodeType: nodeType,
		},
		{
			Service:  cluster.Config.Product,
			Action:   "start",
			NodeType: nodeType,
		},
		{
			Service:  cluster.Config.Product,
			Action:   "status",
			NodeType: nodeType,
		},
	}

	output, err := ms.ManageService(publicIP, actions)
	if err != nil {
		return fmt.Errorf("%w: failed to start %s-%s on %s", err, cluster.Config.Product, nodeType, publicIP)
	}
	if output != "" {
		if !strings.Contains(output, "active ") {
			return ReturnLogError("failed with output: %s", output)
		}
	}

	LogLevel("info", "%s-%s service successfully enabled", cluster.Config.Product, nodeType)

	return nil
}
