package aws

import (
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"

	"github.com/rancher/distros-test-framework/factory"
	"github.com/rancher/distros-test-framework/shared"
)

type Client struct {
	infra *factory.Cluster
	ec2   *ec2.EC2
}

type response struct {
	nodeId     string
	externalIp string
	privateIp  string
}

func AddNode(c *factory.Cluster) (*Client, error) {
	sess, err := session.NewSession(&aws.Config{
		Region: aws.String(c.AwsConfig.Region)})
	if err != nil {
		return nil, shared.ReturnLogError("error creating AWS session: %v", err)
	}

	return &Client{
		infra: &factory.Cluster{AwsConfig: c.AwsConfig},
		ec2:   ec2.New(sess),
	}, nil
}

func (c Client) CreateInstances(names ...string) (externalIPs, privateIPs, ids []string, err error) {
	if len(names) == 0 {
		return nil, nil, nil, shared.ReturnLogError("must sent name for the instance")
	}

	errChan := make(chan error, len(names))
	resChan := make(chan response, len(names))
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

			externalIp, privateIp, err := c.fetchIP(nodeID)
			if err != nil {
				errChan <- shared.ReturnLogError("error fetching ip: %w\n", err)
				return
			}

			resChan <- response{nodeId: nodeID, externalIp: externalIp, privateIp: privateIp}
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

	var externalIps, privateIps, nodeIds []string
	for i := range resChan {
		nodeIds = append(nodeIds, i.nodeId)
		externalIps = append(externalIps, i.externalIp)
		privateIps = append(privateIps, i.privateIp)
	}

	return externalIps, privateIps, nodeIds, nil
}

func (c Client) DeleteInstance(ip string) error {
	if ip == "" {
		return shared.ReturnLogError("must sent a ip: %s\n", ip)
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
			if *node.State.Name == "running" {
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
	}
	if !found {
		return shared.ReturnLogError("no running instances found for ip: %s\n", ip)
	}

	return nil
}

func (c Client) WaitForInstanceRunning(instanceId string) error {
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
			return fmt.Errorf("timed out waiting for instance to be in running state and pass status checks")
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

func (c Client) create(name string) (*ec2.Reservation, error) {
	volume, err := strconv.ParseInt(c.infra.AwsConfig.VolumeSize, 10, 64)
	if err != nil {
		return nil, shared.ReturnLogError("error converting volume size to int64: %w\n", err)
	}

	input := &ec2.RunInstancesInput{
		ImageId:      aws.String(c.infra.AwsConfig.Ami),
		InstanceType: aws.String(c.infra.AwsConfig.InstanceClass),
		MinCount:     aws.Int64(1),
		MaxCount:     aws.Int64(1),
		KeyName:      aws.String(c.infra.AwsConfig.KeyName),
		NetworkInterfaces: []*ec2.InstanceNetworkInterfaceSpecification{
			{
				AssociatePublicIpAddress: aws.Bool(true),
				DeviceIndex:              aws.Int64(0),
				SubnetId:                 aws.String(c.infra.AwsConfig.Subnets),
				Groups:                   aws.StringSlice([]string{c.infra.AwsConfig.SgId}),
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
			AvailabilityZone: aws.String(c.infra.AwsConfig.AvailabilityZone),
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

func (c Client) fetchIP(nodeID string) (publicIP string, privateIP string, err error) {
	waitErr := c.WaitForInstanceRunning(nodeID)
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
		return "", fmt.Errorf("no instance ID found")
	}

	return *reservation.Instances[0].InstanceId, nil
}
