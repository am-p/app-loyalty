package storage

import (
	"context"
	"crypto/sha256"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
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

func TestS3SignsWithPublicEndpointWithoutLeakingInternalHost(t *testing.T) {
	store, err := NewS3(context.Background(), config.Config{
		S3Endpoint: "http://minio.internal.test:9000", S3PublicEndpoint: "https://media.puntazo.test", S3Region: "us-east-1", S3Bucket: "private", S3AccessKeyID: "access", S3SecretAccessKey: "secret",
	})
	if err != nil {
		t.Fatal(err)
	}
	signedURL, err := store.SignedGet(context.Background(), "brands/7/id.jpg", 5*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := url.Parse(signedURL)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Scheme != "https" || parsed.Host != "media.puntazo.test" || parsed.Path != "/private/brands/7/id.jpg" {
		t.Fatalf("unexpected public signed URL %q", signedURL)
	}
	credential := parsed.Query().Get("X-Amz-Credential")
	if !strings.HasPrefix(credential, "access/") || !strings.Contains(credential, "/us-east-1/s3/aws4_request") || parsed.Query().Get("X-Amz-Signature") == "" {
		t.Fatalf("public URL does not preserve S3 signing configuration: %q", signedURL)
	}
	if strings.Contains(signedURL, "minio.internal.test") {
		t.Fatalf("signed URL leaked internal endpoint: %q", signedURL)
	}
}

func TestS3InternalOperationsUsePrivateEncryptedBucket(t *testing.T) {
	type request struct {
		method, path, sse string
	}
	var mu sync.Mutex
	requests := make([]request, 0, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		_ = r.Body.Close()
		mu.Lock()
		requests = append(requests, request{method: r.Method, path: r.URL.Path, sse: r.Header.Get("X-Amz-Server-Side-Encryption")})
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	store, err := NewS3(context.Background(), config.Config{
		S3Endpoint: server.URL, S3PublicEndpoint: "https://media.puntazo.test", S3Region: "us-east-1", S3Bucket: "private", S3AccessKeyID: "access", S3SecretAccessKey: "secret", S3ServerSideEncryption: "AES256",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err = store.Ready(context.Background()); err != nil {
		t.Fatal(err)
	}
	body := []byte("private image")
	digest := sha256.Sum256(body)
	if err = store.Put(context.Background(), "brands/7/id.png", "image/png", body, digest[:]); err != nil {
		t.Fatal(err)
	}
	if err = store.Delete(context.Background(), "brands/7/id.png"); err != nil {
		t.Fatal(err)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(requests) != 3 || requests[0].method != http.MethodHead || requests[0].path != "/private" {
		t.Fatalf("readiness requests=%+v", requests)
	}
	if requests[1].method != http.MethodPut || requests[1].path != "/private/brands/7/id.png" || requests[1].sse != "AES256" {
		t.Fatalf("encrypted put request=%+v", requests[1])
	}
	if requests[2].method != http.MethodDelete || requests[2].path != "/private/brands/7/id.png" {
		t.Fatalf("delete request=%+v", requests[2])
	}
}
