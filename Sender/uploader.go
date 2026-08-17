package main

import (
	"context"
	"fmt"
	"os"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

func uploadFile(bucketName, filepath, s3key string) error {
	ctx := context.Background()

	cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion("ap-south-1"))
	if err != nil {
		return fmt.Errorf("config load failed: %w", err)
	}

	client := s3.NewFromConfig(cfg)

	file, err := os.Open(filepath)
	if err != nil {
		return fmt.Errorf("file open failed: %w", err)
	}
	defer file.Close()

	_, err = client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: &bucketName,
		Key:    &s3key,
		Body:   file,
	})

	if err != nil {
		return fmt.Errorf("upload failed: %w", err)
	}

	fmt.Printf("Uploaded %s to s3://%s/%s\n", filepath, bucketName, s3key)
	return nil

}
