package aws

import (
	"fmt"
	"sort"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/s3"

	"github.com/rancher/distros-test-framework/shared"
)

func (c Client) GetObjects(bucket string) ([]*s3.Object, error) {
	input := &s3.ListObjectsInput{
		Bucket: aws.String(bucket),
	}

	output, err := c.s3.ListObjects(input)
	if err != nil {
		return nil, err
	}

	return output.Contents, nil
}

func (c Client) DeleteS3Object(bucket, folder string) error {
	input := &s3.ListObjectsV2Input{
		Bucket: aws.String(bucket),
		Prefix: aws.String(folder),
	}

	objList, err := c.s3.ListObjectsV2(input)
	if err != nil {
		return fmt.Errorf("failed to list objects: %w", err)
	}

	if len(objList.Contents) == 0 {
		return fmt.Errorf("no objects found with prefix %s", *input.Prefix)
	}

	sort.Slice(objList.Contents, func(i, j int) bool {
		return objList.Contents[i].LastModified.After(*objList.Contents[j].LastModified)
	})

	key := aws.StringValue(objList.Contents[0].Key)
	delInput := &s3.DeleteObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	}

	_, delErr := c.s3.DeleteObject(delInput)
	if delErr != nil {
		return fmt.Errorf("failed to delete object %s: %w", key, delErr)
	}

	shared.LogLevel("info", "deleted object %s from bucket %s", key, bucket)

	return nil
}
