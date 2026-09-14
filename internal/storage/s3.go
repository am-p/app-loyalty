package storage

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"time"

	"clientesFrecuentes/internal/config"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

type S3 struct {
	client    *s3.Client
	presigner *s3.PresignClient
	bucket    string
}

const operationTimeout = 10 * time.Second

func NewS3(ctx context.Context, cfg config.Config) (*S3, error) {
	awsCfg, err := awsconfig.LoadDefaultConfig(ctx,
		awsconfig.WithRegion(cfg.S3Region),
		awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(cfg.S3AccessKeyID, cfg.S3SecretAccessKey, "")),
	)
	if err != nil {
		return nil, fmt.Errorf("load s3 configuration: %w", err)
	}
	client := s3.NewFromConfig(awsCfg, func(options *s3.Options) {
		options.BaseEndpoint = aws.String(cfg.S3Endpoint)
		options.UsePathStyle = true
	})
	return &S3{client: client, presigner: s3.NewPresignClient(client), bucket: cfg.S3Bucket}, nil
}

func (s *S3) Put(ctx context.Context, key, mime string, body, digest []byte) error {
	ctx, cancel := context.WithTimeout(ctx, operationTimeout)
	defer cancel()
	_, err := s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:         aws.String(s.bucket),
		Key:            aws.String(key),
		Body:           bytes.NewReader(body),
		ContentLength:  aws.Int64(int64(len(body))),
		ContentType:    aws.String(mime),
		ChecksumSHA256: aws.String(base64.StdEncoding.EncodeToString(digest)),
	})
	if err != nil {
		return fmt.Errorf("put private media object: %w", err)
	}
	return nil
}

func (s *S3) Delete(ctx context.Context, key string) error {
	ctx, cancel := context.WithTimeout(ctx, operationTimeout)
	defer cancel()
	_, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{Bucket: aws.String(s.bucket), Key: aws.String(key)})
	if err != nil {
		return fmt.Errorf("delete private media object: %w", err)
	}
	return nil
}

func (s *S3) SignedGet(ctx context.Context, key string, ttl time.Duration) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, operationTimeout)
	defer cancel()
	result, err := s.presigner.PresignGetObject(ctx, &s3.GetObjectInput{Bucket: aws.String(s.bucket), Key: aws.String(key)}, s3.WithPresignExpires(ttl))
	if err != nil {
		return "", fmt.Errorf("sign private media object: %w", err)
	}
	return result.URL, nil
}
