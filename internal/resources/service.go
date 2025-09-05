package resources

import (
	"fmt"
	"strings"
	"time"

	"github.com/avast/retry-go"
)

const (
	rotate  = "rotate"
	status  = "status"
	restart = "restart"
	start   = "start"
	stop    = "stop"
	enable  = "enable"
)

const (
	rke2 = "rke2"
	k3s  = "k3s"
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
// If action is "restart" or "start", it will not retry the command.
//
// ip IP of the node where the systemctl service call is performed.
// actions slice of ServiceAction struct contains all the service actions.
func (ms *ManageService) ManageService(ip string, actions []ServiceAction) (string, error) {
	err := validateActions(ip, actions)
	if err != nil {
		return "", fmt.Errorf("action validation failed for %s: %w", ip, err)
	}
	for _, act := range actions {
		LogLevel("info", "Running %s %s service on node: %s", act.Action, act.Service, ip)
		if act.Action == rotate {
			rotateErr := CertRotate(act.Service, ip)
			if rotateErr != nil {
				LogLevel("error", "Error rotating certificate: %v", rotateErr)
				return "", fmt.Errorf("certificate rotation failed for %s: %w", ip, rotateErr)
			}

			continue
		}
		var svcName string
		var svcNameErr error
		if strings.Contains(act.Service, rke2) || strings.Contains(act.Service, k3s) {
			svcName, svcNameErr = productService(act.Service, act.NodeType)
			if svcNameErr != nil {
				return "", fmt.Errorf("service name failed for %s: %w", ip, svcNameErr)
			}
		} else { // if its not rke2 or k3s, then proceed with the service name as is.
			svcName = act.Service
		}

		cmd, buildCmdErr := SystemCtlCmd(svcName, act.Action)
		if buildCmdErr != nil {
			return "", fmt.Errorf("command build failed for %s: %w", ip, buildCmdErr)
		}
		if act.ExplicitDelay > 0 {
			delay := act.ExplicitDelay * time.Second
			LogLevel("info", "Waiting for %v before running systemctl %s %s on node: %s", delay, act.Action, svcName, ip)
			<-time.After(delay)
		}
		LogLevel("debug", "Command: %s on node: %s", cmd, ip)
		output, execErr := ms.execute(cmd, act.Action, ip)
		if execErr != nil {
			LogLevel("error", "Error running command %s on %s: %v", cmd, ip, execErr)
			return "", fmt.Errorf("action %s failed on %s: %w", act.Action, ip, execErr)
		}
		if act.Action == status {
			LogLevel("debug", "service %s output: \n %s", act.Action, output)
			return strings.TrimSpace(output), nil
		}
		LogLevel("info", "Finished running systemctl %s %s on node: %s\n", act.Action, svcName, ip)
		if output != "" {
			LogLevel("warn", "Output: %s", strings.TrimSpace(output))
		}
	}

	return "", nil
}

func validateActions(ip string, actions []ServiceAction) error {
	validActions := map[string]bool{
		rotate:  true,
		stop:    true,
		status:  true,
		restart: true,
		start:   true,
		enable:  true,
	}

	for _, act := range actions {
		if !validActions[act.Action] {
			return fmt.Errorf("invalid action %s for %s", act.Action, ip)
		}
	}

	if len(actions) == 0 {
		return fmt.Errorf("no actions provided for %s", ip)
	}

	return nil
}

func (ms *ManageService) execute(cmd, action, ip string) (string, error) {
	var result string
	var err error

	// thats terrible. But for now we need to handle the restart and start cases separately.
	// because we cant retry those.
	if action == start || action == restart {
		result, err = RunCommandOnNode(cmd, ip)
		if err != nil {
			LogLevel("error", "Error running command %s on %s: %v", cmd, ip, err)
			return "", fmt.Errorf("error running command on %s: %w", ip, err)
		}

		return result, nil
	}

	timeNow := time.Now()
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
				LogLevel("warn", "Retry %d/%d failed for %s on %s: %v at (%s)",
					n+1, ms.MaxRetries, cmd, ip, err, timeNow.Format("2006-01-02 15:04:05"))
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
			Action:   enable,
			NodeType: nodeType,
		},
		{
			Service:  cluster.Config.Product,
			Action:   start,
			NodeType: nodeType,
		},
		{
			Service:  cluster.Config.Product,
			Action:   status,
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

// SystemCtlCmd returns the systemctl command based on the action and service.
// it can be used alone to create the command and be ran by another function caller.
//
// Action can be: start | stop | restart | status | enable.
func SystemCtlCmd(service, action string) (string, error) {
	systemctlCmdMap := map[string]string{
		"stop":    "sudo systemctl --no-block stop",
		"start":   "sudo systemctl --no-block start",
		"restart": "sudo systemctl --no-block restart",
		"status":  "sudo systemctl --no-block status",
		"enable":  "sudo systemctl --no-block enable",
	}

	sysctlPrefix, ok := systemctlCmdMap[action]
	if !ok {
		return "", ReturnLogError("action value should be: start | stop | restart | status | enable")
	}

	return fmt.Sprintf("%s %s", sysctlPrefix, service), nil
}

// productService gets the service name for a specific distro product and nodeType.
func productService(product, nodeType string) (string, error) {
	if nodeType == "" {
		return "", fmt.Errorf("nodeType required for %s service", product)
	}

	serviceNameMap := map[string]string{
		"k3s-server":  "k3s",
		"k3s-agent":   "k3s-agent",
		"rke2-server": "rke2-server",
		"rke2-agent":  "rke2-agent",
	}

	svcName, ok := serviceNameMap[fmt.Sprintf("%s-%s", product, nodeType)]
	if !ok {
		return "", ReturnLogError("nodeType needs to be one of: server | agent")
	}

	return svcName, nil
}
