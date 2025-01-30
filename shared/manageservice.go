package shared

import (
	"fmt"
	"strings"
	"time"

	"github.com/avast/retry-go"
)

type ManageService struct {
	MaxRetries uint
	RetryDelay time.Duration
}

// ServiceAction represents a service operation to perform on a given node type.
type ServiceAction struct {
	Service  string
	Action   string
	NodeType string
}

// NewManageService creates a ManageService instance with the given maxRetries and retryDelay parameters.
// maxRetries/retryDelay are used in order to retry the service operation in case of failure or delays.
func NewManageService(maxRetries uint, retryDelay time.Duration) *ManageService {
	return &ManageService{
		MaxRetries: maxRetries,
		RetryDelay: retryDelay * time.Second,
	}
}

// ManageService performs a service operations on a node based on the given actions.
//
// action can be used as a single one or multiple actions.
//
// If action is "rotate", it will rotate the certificate for the given service.
// If action is "status", it will return the status of the service.
func (ms *ManageService) ManageService(ip string, actions []ServiceAction) (string, error) {
	for _, act := range actions {
		LogLevel("debug", "Starting %s on %s@%s", act.Action, act.Service, ip)
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

			LogLevel("debug", "Command: %s on %s@%s", cmd, svcName, ip)
			output, err := ms.execute(cmd, ip)
			if err != nil {
				return "", fmt.Errorf("action %s failed on %s: %w", act.Action, ip, err)
			}

			if act.Action == "status" {
				LogLevel("debug", "service %s output: \n %s", act.Action, output)
				return strings.TrimSpace(output), nil
			}

			LogLevel("debug", "Completed %s on %s@%s\nOutput: %s", act.Action, svcName, ip, strings.TrimSpace(output))
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
