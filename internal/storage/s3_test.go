package storage

import (
	"context"
	"strings"
	"testing"
	"time"

	"clientesFrecuentes/internal/config"
)

func TestS3SignsPathStylePrivateURL(t *testing.T) {
	store, err := NewS3(context.Background(), config.Config{S3Endpoint: "https://storage.example.test", S3Region: "us-east-1", S3Bucket: "private", S3AccessKeyID: "access", S3SecretAccessKey: "secret"})
	if err != nil {
		t.Fatal(err)
	}
	url, err := store.SignedGet(context.Background(), "brands/7/id.jpg", 5*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(url, "https://storage.example.test/private/brands/7/id.jpg?") || !strings.Contains(url, "X-Amz-Signature=") {
		t.Fatalf("unexpected signed URL %q", url)
	}
}
