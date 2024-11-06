package aws

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/s3"

	"github.com/rancher/distros-test-framework/pkg/customflag"
)

func (c Client) GetObjects(flags *customflag.FlagConfig) ([]*s3.Object, error) {
	input := &s3.ListObjectsInput{
		Bucket: aws.String(flags.S3Flags.Bucket),
	}

	output, err := c.s3.ListObjects(input)
	if err != nil {
		return nil, err
	}

	// for _, obj := range output.Contents {
	// 	if strings.Contains(*obj.Key, flags.S3Flags.Folder) {
	// 		fmt.Println("obj-name: ", *obj.Key)
	// 	}
	// }
	return output.Contents, nil
}

// TODO: Create bucket

// TODO: Delete bucket

// TODO: Create object

// TODO: Delete object
