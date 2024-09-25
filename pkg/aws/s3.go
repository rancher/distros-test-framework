package aws

import (
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/rancher/distros-test-framework/shared"
)

func (c Client) GetObjects(s3Config shared.AwsS3Config) {
	input := &s3.ListObjectsInput{
		Bucket: aws.String(s3Config.Bucket),
	}

	output, err := c.s3.ListObjects(input)
	if err != nil {
		fmt.Println(err)
	}

	for _, obj := range output.Contents {
		if strings.Contains(*obj.Key, s3Config.Folder) {
			fmt.Println("obj-name: ", *obj.Key)
		}
	}
}

// TODO: Create bucket

// TODO: Delete bucket

// TODO: Create object

// TODO: Delete object
