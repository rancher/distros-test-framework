package aws

import (
	"fmt"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/s3"

	"github.com/rancher/distros-test-framework/internal/resources"
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

func (c Client) DeleteS3Object(bucket, folder, objectName string) error {
	var key string
	if folder != "" {
		key = fmt.Sprintf("%s/%s", folder, objectName)
	} else {
		key = objectName
	}

	headInput := &s3.HeadObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	}

	_, headErr := c.s3.HeadObject(headInput)
	if headErr != nil {
		resources.LogLevel("info", "object %s doesn't exist in bucket %s (already deleted)", key, bucket)
		return nil
	}

	delInput := &s3.DeleteObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	}

	output, err := c.s3.DeleteObject(delInput)
	if err != nil {
		return fmt.Errorf("failed to delete object %s: %w", key, err)
	}

	resources.LogLevel("info", "deleted specific object %s with key %s from bucket %s", output, key, bucket)

	return nil
}
