package aws

import (
	"errors"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/rancher/distros-test-framework/shared"
)

type ec2Response struct {
	nodeId     string
	externalIP string
	privateIP  string
}

func (c Client) CreateInstances(names ...string) (externalIPs, privateIPs, ids []string, err error) {
	if len(names) == 0 {
		return nil, nil, nil, shared.ReturnLogError("must sent name for the instance")
	}

	errChan := make(chan error, len(names))
	resChan := make(chan ec2Response, len(names))
	var wg sync.WaitGroup

	for _, n := range names {
		wg.Add(1)
		go func(n string) {
			defer wg.Done()

			res, err := c.create(n)
			if err != nil {
				errChan <- shared.ReturnLogError("error creating instance: %w\n", err)
				return
			}

			nodeID, err := extractID(res)
			if err != nil {
				errChan <- shared.ReturnLogError("error extracting instance id: %w\n", err)
				return
			}

			externalIP, privateIP, err := c.fetchIP(nodeID)
			if err != nil {
				errChan <- shared.ReturnLogError("error fetching ip: %w\n", err)
				return
			}

			resChan <- ec2Response{nodeId: nodeID, externalIP: externalIP, privateIP: privateIP}
		}(n)
	}
	go func() {
		wg.Wait()
		close(resChan)
		close(errChan)
	}()

	for e := range errChan {
		if e != nil {
			return nil, nil, nil, shared.ReturnLogError("error from errChan: %w\n", e)
		}
	}

	var nodeIds []string
	for i := range resChan {
		nodeIds = append(nodeIds, i.nodeId)
		externalIPs = append(externalIPs, i.externalIP)
		privateIPs = append(privateIPs, i.privateIP)
	}

	return externalIPs, privateIPs, nodeIds, nil
}

func (c Client) DeleteInstance(ip string) error {
	if ip == "" {
		return shared.ReturnLogError("must sent a ip")
	}

	data := &ec2.DescribeInstancesInput{
		Filters: []*ec2.Filter{
			{
				Name:   aws.String("ip-address"),
				Values: aws.StringSlice([]string{ip}),
			},
		},
	}

	res, err := c.ec2.DescribeInstances(data)
	if err != nil {
		return shared.ReturnLogError("error describing instances: %w\n", err)
	}

	found := false
	for _, r := range res.Reservations {
		for _, node := range r.Instances {
			if *node.State.Name != "running" {
				continue
			}

			found = true
			terminateInput := &ec2.TerminateInstancesInput{
				InstanceIds: aws.StringSlice([]string{*node.InstanceId}),
			}

			_, err := c.ec2.TerminateInstances(terminateInput)
			if err != nil {
				return fmt.Errorf("error terminating instance: %w", err)
			}
			instanceName := "Unknown"
			if len(node.Tags) > 0 {
				instanceName = *node.Tags[0].Value
			}
			shared.LogLevel("info", fmt.Sprintf("Terminated instance: %s (ID: %s)",
				instanceName, *node.InstanceId))
		}
	}

	if !found {
		return shared.ReturnLogError("no running instances found for ip: %s\n", ip)
	}

	return nil
}

func (c Client) GetInstanceIDByIP(ipAddress string) (string, error) {
	if ipAddress == "" {
		return "", shared.ReturnLogError("calling GetInstanceIDByIP with empty ip address, must send a valid ip address")
	}

	input := &ec2.DescribeInstancesInput{
		Filters: []*ec2.Filter{
			{
				Name:   aws.String("ip-address"),
				Values: []*string{aws.String(ipAddress)},
			},
		},
	}

	result, err := c.ec2.DescribeInstances(input)
	if err != nil {
		return "", shared.ReturnLogError("failed to describe instances: %v", err)
	}

	// Check if any instances were found.
	if len(result.Reservations) == 0 {
		return "", shared.ReturnLogError("no instances found with the public IP address: %s", ipAddress)
	}

	// Extract and return the instance ID.
	for _, reservation := range result.Reservations {
		for _, instance := range reservation.Instances {
			return *instance.InstanceId, nil
		}
	}

	return "", shared.ReturnLogError("no instances found with the public IP address: ")
}

func (c Client) StopInstance(instanceID string) error {
	if instanceID == "" {
		return shared.ReturnLogError("calling StopInstance with empty instance ID, must send a valid instance ID")
	}

	input := &ec2.StopInstancesInput{
		InstanceIds: []*string{aws.String(instanceID)},
	}

	_, err := c.ec2.StopInstances(input)
	if err != nil {
		return shared.ReturnLogError("failed to stop instance %s: %v", instanceID, err)
	}

	stopErr := c.waitInstanceStop(instanceID)
	if stopErr != nil {
		return shared.ReturnLogError("timed out on stop instance %s: %v", instanceID, stopErr)
	}

	return nil
}

func (c Client) StartInstance(instanceID string) error {
	if instanceID == "" {
		return shared.ReturnLogError("calling StartInstance with empty instance ID, must send a valid instance ID")
	}

	input := &ec2.StartInstancesInput{
		InstanceIds: []*string{aws.String(instanceID)},
	}

	_, err := c.ec2.StartInstances(input)
	if err != nil {
		return shared.ReturnLogError("failed to start instance %s: %v", instanceID, err)
	}

	startErr := c.waitForInstanceRunning(instanceID)
	if startErr != nil {
		return shared.ReturnLogError("timed out on start instance %s: %v", instanceID, startErr)
	}

	return nil
}

func (c Client) ReleaseElasticIps(ipAddress string) error {
	if ipAddress == "" {
		return shared.ReturnLogError("calling ReleaseElasticIps with empty ip address, must send a valid ip address")
	}

	input := &ec2.DescribeInstancesInput{
		Filters: []*ec2.Filter{
			{
				Name:   aws.String("ip-address"),
				Values: aws.StringSlice([]string{ipAddress}),
			},
		},
	}

	result, err := c.ec2.DescribeInstances(input)
	if err != nil {
		return shared.ReturnLogError("error describing instances: %w\n", err)
	}

	for _, r := range result.Reservations {
		for _, i := range r.Instances {
			if i.PublicIpAddress != nil {
				addressesInput := &ec2.DescribeAddressesInput{
					Filters: []*ec2.Filter{
						{
							Name:   aws.String("domain"),
							Values: []*string{aws.String("vpc")},
						},
						{
							Name:   aws.String("public-ip"),
							Values: []*string{i.PublicIpAddress},
						},
					},
				}

				addressesOutput, addrErr := c.ec2.DescribeAddresses(addressesInput)
				if addrErr != nil {
					return shared.ReturnLogError("error describing elastic ip: %w\n", addrErr)
				}

				if len(addressesOutput.Addresses) > 0 {
					allocationID := addressesOutput.Addresses[0].AllocationId
					_, addrErr = c.ec2.ReleaseAddress(&ec2.ReleaseAddressInput{AllocationId: allocationID})
					if addrErr != nil {
						return shared.ReturnLogError("error releasing elastic ip: %w\n", addrErr)
					}

					shared.LogLevel("info", "released eips from intances: %v", *i.InstanceId)
				}
			}
		}
	}

	return nil
}

func (c Client) create(name string) (*ec2.Reservation, error) {
	volume, err := strconv.ParseInt(c.infra.Aws.EC2Config.VolumeSize, 10, 64)
	if err != nil {
		return nil, shared.ReturnLogError("error converting volume size to int64: %w\n", err)
	}

	input := &ec2.RunInstancesInput{
		ImageId:      aws.String(c.infra.Aws.EC2Config.Ami),
		InstanceType: aws.String(c.infra.Aws.EC2Config.InstanceClass),
		MinCount:     aws.Int64(1),
		MaxCount:     aws.Int64(1),
		KeyName:      aws.String(c.infra.Aws.EC2Config.KeyName),
		NetworkInterfaces: []*ec2.InstanceNetworkInterfaceSpecification{
			{
				AssociatePublicIpAddress: aws.Bool(true),
				DeviceIndex:              aws.Int64(0),
				SubnetId:                 aws.String(c.infra.Aws.EC2Config.Subnets),
				Groups:                   aws.StringSlice([]string{c.infra.Aws.EC2Config.SgId}),
			},
		},
		BlockDeviceMappings: []*ec2.BlockDeviceMapping{
			{
				DeviceName: aws.String("/dev/sda1"),
				Ebs: &ec2.EbsBlockDevice{
					VolumeSize: aws.Int64(volume),
					VolumeType: aws.String("gp2"),
				},
			},
		},
		Placement: &ec2.Placement{
			AvailabilityZone: aws.String(c.infra.Aws.EC2Config.AvailabilityZone),
		},
		TagSpecifications: []*ec2.TagSpecification{
			{
				ResourceType: aws.String("instance"),
				Tags: []*ec2.Tag{
					{
						Key:   aws.String("Name"),
						Value: aws.String(name),
					},
				},
			},
		},
	}

	return c.ec2.RunInstances(input)
}

func (c Client) waitForInstanceRunning(instanceId string) error {
	input := &ec2.DescribeInstanceStatusInput{
		InstanceIds:         aws.StringSlice([]string{instanceId}),
		IncludeAllInstances: aws.Bool(true),
	}

	ticker := time.NewTicker(15 * time.Second)
	timeout := time.After(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			return errors.New("timed out waiting for instance to be in running state and pass status checks")
		case <-ticker.C:
			statusRes, err := c.ec2.DescribeInstanceStatus(input)
			if err != nil {
				return fmt.Errorf("error describing instance status: %w", err)
			}

			if len(statusRes.InstanceStatuses) == 0 {
				continue
			}

			status := statusRes.InstanceStatuses[0]
			if *status.InstanceStatus.Status == "ok" && *status.SystemStatus.Status == "ok" {
				shared.LogLevel("info", fmt.Sprintf("Instance %s is running "+
					"and passed status checks", instanceId))

				return nil
			}
		}
	}
}

func (c Client) waitInstanceStop(instanceID string) error {
	input := &ec2.DescribeInstanceStatusInput{
		InstanceIds:         aws.StringSlice([]string{instanceID}),
		IncludeAllInstances: aws.Bool(true),
	}

	ticker := time.NewTicker(10 * time.Second)
	timeout := time.After(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			return shared.ReturnLogError("timed out waiting for instance to stop")
		case <-ticker.C:
			statusRes, err := c.ec2.DescribeInstanceStatus(input)
			if err != nil {
				return shared.ReturnLogError("error describing instance status: %w\n", err)
			}

			status := statusRes.InstanceStatuses[0]
			if *status.InstanceState.Name == "stopped" {
				shared.LogLevel("info", fmt.Sprintf("Instance %s is stopped", instanceID))

				return nil
			}
		}
	}
}

func (c Client) fetchIP(nodeID string) (publicIP, privateIP string, err error) {
	waitErr := c.waitForInstanceRunning(nodeID)
	if waitErr != nil {
		return "", "", shared.ReturnLogError("error waiting for instance to be running: %w\n", waitErr)
	}

	id := &ec2.DescribeInstancesInput{
		InstanceIds: aws.StringSlice([]string{nodeID}),
	}
	result, err := c.ec2.DescribeInstances(id)
	if err != nil {
		return "", "", shared.ReturnLogError("error describing instances: %w\n", err)
	}

	for _, r := range result.Reservations {
		for _, i := range r.Instances {
			if i.PublicIpAddress != nil && i.PrivateIpAddress != nil {
				return *i.PublicIpAddress, *i.PrivateIpAddress, nil
			}
		}
	}

	return "", "", shared.ReturnLogError("no ip found for instance: %s\n", nodeID)
}

func extractID(reservation *ec2.Reservation) (string, error) {
	if len(reservation.Instances) == 0 || reservation.Instances[0].InstanceId == nil {
		return "", errors.New("no instance ID found")
	}

	return *reservation.Instances[0].InstanceId, nil
}
