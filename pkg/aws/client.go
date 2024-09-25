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

type ec2Response struct {
	nodeId     string
	externalIP string
	privateIP  string
}

func AddEC2Client(c *shared.Cluster) (*Client, error) {
	sess, err := session.NewSession(&aws.Config{
		Region: aws.String(c.AwsEC2.Region)})
	if err != nil {
		return nil, shared.ReturnLogError("error creating AWS EC2 client session: %v", err)
	}

	return &Client{
		infra: &shared.Cluster{AwsEC2: c.AwsEC2},
		ec2:   ec2.New(sess),
	}, nil
}

func AddS3Client(s3Config shared.AwsS3Config) (*Client, error) {
	sess, err := session.NewSession(&aws.Config{
		Region: aws.String(s3Config.Region)})
	if err != nil {
		return nil, shared.ReturnLogError("error creating AWS S3 client session: %v", err)
	}

	return &Client{
		s3: s3.New(sess),
	}, nil
}
