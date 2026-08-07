package main

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"
)

// fakeS3 implements s3API without touching the network. getErr, when set, is
// returned from GetObject so the error-mapping paths can be exercised.
type fakeS3 struct {
	objects map[string]string
	getErr  error

	// last PutObject call, for asserting key layout and content type.
	putKey         string
	putContentType string
	putBody        string
}

func newFakeS3() *fakeS3 { return &fakeS3{objects: map[string]string{}} }

func (f *fakeS3) GetObject(_ context.Context, in *s3.GetObjectInput, _ ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	body, ok := f.objects[*in.Key]
	if !ok {
		return nil, &types.NoSuchKey{}
	}
	return &s3.GetObjectOutput{Body: io.NopCloser(strings.NewReader(body))}, nil
}

func (f *fakeS3) PutObject(_ context.Context, in *s3.PutObjectInput, _ ...func(*s3.Options)) (*s3.PutObjectOutput, error) {
	b, err := io.ReadAll(in.Body)
	if err != nil {
		return nil, err
	}
	f.putKey = *in.Key
	f.putBody = string(b)
	if in.ContentType != nil {
		f.putContentType = *in.ContentType
	}
	f.objects[*in.Key] = string(b)
	return &s3.PutObjectOutput{}, nil
}

func newTestS3Store() (*S3Store, *fakeS3) {
	f := newFakeS3()
	return &S3Store{client: f, bucket: "test-bucket"}, f
}

// The key layout must stay notes/{slug}.txt — it mirrors FSStore and is what
// the IAM policy scopes permissions to (arn:...:bucket/notes/*).
func TestS3StoreKeyLayoutAndContentType(t *testing.T) {
	store, fake := newTestS3Store()

	if err := store.Put(context.Background(), "abc12", "hello s3"); err != nil {
		t.Fatalf("Put: %v", err)
	}
	if fake.putKey != "notes/abc12.txt" {
		t.Errorf("key = %q, want %q", fake.putKey, "notes/abc12.txt")
	}
	if fake.putContentType != "text/plain; charset=utf-8" {
		t.Errorf("content type = %q, want text/plain; charset=utf-8", fake.putContentType)
	}
	if fake.putBody != "hello s3" {
		t.Errorf("body = %q, want %q", fake.putBody, "hello s3")
	}
}

func TestS3StoreRoundtrip(t *testing.T) {
	store, _ := newTestS3Store()
	ctx := context.Background()

	if err := store.Put(ctx, "abc12", "hello s3"); err != nil {
		t.Fatalf("Put: %v", err)
	}
	got, err := store.Get(ctx, "abc12")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got != "hello s3" {
		t.Errorf("text = %q, want %q", got, "hello s3")
	}
}

// With s3:ListBucket granted, a missing object surfaces as a typed NoSuchKey.
func TestS3StoreGetMissingNoSuchKey(t *testing.T) {
	store, fake := newTestS3Store()
	fake.getErr = &types.NoSuchKey{}

	_, err := store.Get(context.Background(), "brandnew")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

// Without s3:ListBucket — which is our least-privilege deployment — S3 returns
// AccessDenied for a missing object instead of NoSuchKey. This must still read
// as an empty note, or every fresh pad 500s in production.
func TestS3StoreGetMissingAccessDenied(t *testing.T) {
	store, fake := newTestS3Store()
	fake.getErr = &smithy.GenericAPIError{Code: "AccessDenied", Message: "Access Denied"}

	_, err := store.Get(context.Background(), "brandnew")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

// A genuine failure must NOT be swallowed as an empty note — the handler needs
// it to produce a 500 rather than silently serving a blank pad.
func TestS3StoreGetRealErrorNotSwallowed(t *testing.T) {
	store, fake := newTestS3Store()
	fake.getErr = &smithy.GenericAPIError{Code: "InternalError", Message: "boom"}

	_, err := store.Get(context.Background(), "abc12")
	if err == nil {
		t.Fatal("err = nil, want a non-nil error")
	}
	if errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want a real error rather than ErrNotFound", err)
	}
}
