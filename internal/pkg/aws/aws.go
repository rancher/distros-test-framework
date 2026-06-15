package aws

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
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
	if c.Aws.Region == "" {
		return nil, resources.ReturnLogError("cluster.Aws.Region is empty — qainfra vars.tfvars parsing may have failed")
	}

	cfg := &aws.Config{
		Region: aws.String(c.Aws.Region),
	}
	// If we have explicit credentials in cluster config, use them. Otherwise
	// fall back to the SDK's default credential chain (env, ~/.aws, IAM
	// role). Explicit creds make the session deterministic — no surprise from
	// stale ambient env state, especially after the test thread runs for a
	// while.
	if c.Aws.AccessKeyID != "" && c.Aws.SecretAccessKey != "" {
		cfg.Credentials = credentials.NewStaticCredentials(
			c.Aws.AccessKeyID, c.Aws.SecretAccessKey, "",
		)
	}

	sess, err := session.NewSession(cfg)
	if err != nil {
		return nil, resources.ReturnLogError("error creating AWS session: %v", err)
	}

	return &Client{
		infra: &driver.Cluster{Aws: c.Aws, SSH: c.SSH},
		ec2:   ec2.New(sess),
		s3:    s3.New(sess),
	}, nil
}
