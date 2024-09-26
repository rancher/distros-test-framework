package aws

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/s3"

	"github.com/rancher/distros-test-framework/shared"
)

type Client struct {
	infra *shared.Cluster
	ec2   *ec2.EC2
	s3    *s3.S3
}

type ec2response struct {
	nodeId     string
	externalIp string
	privateIp  string
}

type s3response struct {
	s3Bucket string
	s3Folder string
	s3Region string
}

func AddEc2Client(c *shared.Cluster) (*Client, error) {
	sess, err := session.NewSession(&aws.Config{
		Region: aws.String(c.AwsEc2.Region)})
	if err != nil {
		return nil, shared.ReturnLogError("error creating AWS session: %v", err)
	}

	return &Client{
		infra: &shared.Cluster{AwsEc2: c.AwsEc2},
		ec2:   ec2.New(sess),
	}, nil
}
