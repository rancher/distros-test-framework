package aws

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/s3"

	"github.com/rancher/distros-test-framework/internal/provisioning/driver"
	"github.com/rancher/distros-test-framework/internal/resources"
)

type Client struct {
	infra *driver.Cluster
	ec2   *ec2.EC2
	s3    *s3.S3
}

func AddClient(c *driver.Cluster) (*Client, error) {
	sess, err := session.NewSession(&aws.Config{
		Region: aws.String(c.Aws.Region),
	})
	if err != nil {
		return nil, resources.ReturnLogError("error creating AWS session: %v", err)
	}

	return &Client{
		infra: &driver.Cluster{Aws: c.Aws},
		ec2:   ec2.New(sess),
		s3:    s3.New(sess),
	}, nil
}
