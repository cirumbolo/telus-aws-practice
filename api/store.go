package main

import (
	"context"
	"errors"
)

// ErrNotFound is returned by Store.Get when a note does not exist. The handler
// translates it into a 200 response with empty text, so a brand-new pad is a
// normal case rather than an error. Every backend must report a missing note
// this way so the API behaves identically regardless of the store.
var ErrNotFound = errors.New("note not found")

// Store is the note-persistence seam. Swapping backends (filesystem now, S3 or
// DynamoDB later) means adding an implementation of this interface and wiring
// it in newStore — handler code never changes.
//
// The context is threaded through even though the filesystem backend ignores
// it, so the S3 backend (whose SDK calls take a context) slots in without a
// signature change.
type Store interface {
	Get(ctx context.Context, slug string) (string, error)
	Put(ctx context.Context, slug, text string) error
}
