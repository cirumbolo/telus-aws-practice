package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"
)

// s3API is the slice of *s3.Client that S3Store actually uses. Depending on an
// interface rather than the concrete client keeps the store unit-testable
// without network access, and documents the backend's blast radius.
type s3API interface {
	GetObject(ctx context.Context, in *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error)
	PutObject(ctx context.Context, in *s3.PutObjectInput, optFns ...func(*s3.Options)) (*s3.PutObjectOutput, error)
}

// S3Store persists notes as objects at notes/{slug}.txt in a single private
// bucket, mirroring FSStore's {slug}.txt layout so the mental model carries
// across backends.
type S3Store struct {
	client s3API
	bucket string
}

// NewS3Store builds a store from the ambient AWS configuration: the default
// credential chain. That means AWS_PROFILE / env vars locally and the EC2
// instance role (via IMDSv2) in production, with no code difference between
// them. Region comes from AWS_REGION or the profile.
func NewS3Store(ctx context.Context, bucket string) (*S3Store, error) {
	if bucket == "" {
		return nil, errors.New("NOTES_BUCKET must be set for the s3 store")
	}
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("load aws config: %w", err)
	}
	if cfg.Region == "" {
		return nil, errors.New("no AWS region configured (set AWS_REGION)")
	}
	return &S3Store{client: s3.NewFromConfig(cfg), bucket: bucket}, nil
}

// key maps a slug to its object key. The caller validates the slug against
// ^[a-zA-Z0-9_-]{1,64}$ before reaching the store, so it cannot contain "/" and
// cannot escape the notes/ prefix.
func (s *S3Store) key(slug string) string { return "notes/" + slug + ".txt" }

func (s *S3Store) Get(ctx context.Context, slug string) (string, error) {
	out, err := s.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(s.key(slug)),
	})
	if err != nil {
		if isNotFound(err) {
			return "", ErrNotFound
		}
		return "", err
	}
	defer out.Body.Close()

	// Bound the read with the same cap the handler enforces on writes, so an
	// oversized object uploaded out-of-band can't balloon memory.
	b, err := io.ReadAll(io.LimitReader(out.Body, maxBody))
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func (s *S3Store) Put(ctx context.Context, slug, text string) error {
	_, err := s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(s.bucket),
		Key:         aws.String(s.key(slug)),
		Body:        strings.NewReader(text),
		ContentType: aws.String("text/plain; charset=utf-8"),
	})
	return err
}

// isNotFound reports whether err means "the object isn't there".
//
// Two cases, and missing the second is the classic S3 bug:
//
//  1. *types.NoSuchKey — what GetObject returns when the caller HAS
//     s3:ListBucket on the bucket.
//  2. AccessDenied — what S3 returns when the caller LACKS s3:ListBucket. S3
//     deliberately hides existence from callers who can't list, so a missing
//     key is indistinguishable from a forbidden one. Our least-privilege
//     instance role grants only GetObject/PutObject on notes/*, so THIS is the
//     branch that fires in the deployed setup. Matching only NoSuchKey would
//     turn every fresh pad into a 500.
//
// "NotFound" covers HeadObject-style paths, which surface that code rather than
// a typed NoSuchKey.
//
// Folding AccessDenied into "empty note" means a genuinely broken IAM policy
// shows up as every note being blank rather than as an error, so that branch
// logs a warning — the misconfiguration stays visible in journalctl without
// changing the HTTP contract.
func isNotFound(err error) bool {
	var nsk *types.NoSuchKey
	if errors.As(err, &nsk) {
		return true
	}
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		switch apiErr.ErrorCode() {
		case "NoSuchKey", "NotFound":
			return true
		case "AccessDenied":
			log.Printf("s3: AccessDenied treated as an empty note — if every note reads blank, "+
				"check the IAM policy grants s3:GetObject on the notes/ prefix (%v)", err)
			return true
		}
	}
	return false
}
