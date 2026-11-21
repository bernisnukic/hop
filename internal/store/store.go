// Package store keeps short links. Postgres is the real backend; Memory is
// for tests and for running the service on a laptop without a database.
package store

import (
	"context"
	"crypto/rand"
	"errors"
	"time"
)

// ErrNotFound means no link has the requested code.
var ErrNotFound = errors.New("link not found")

// ErrCodeTaken means a generated code collided with an existing one.
var ErrCodeTaken = errors.New("code already in use")

// Link is one short link.
type Link struct {
	Code      string    `json:"code"`
	URL       string    `json:"url"`
	Hits      int64     `json:"hits"`
	CreatedAt time.Time `json:"created_at"`
}

// Store is what the HTTP server needs from a backend.
type Store interface {
	// Create saves a link under code. It returns ErrCodeTaken on a collision.
	Create(ctx context.Context, code, url string) (Link, error)
	// Resolve returns the link for code and counts one hit.
	Resolve(ctx context.Context, code string) (Link, error)
	// Get returns the link for code without counting a hit.
	Get(ctx context.Context, code string) (Link, error)
	// Ping reports whether the backend can serve requests.
	Ping(ctx context.Context) error
}

const alphabet = "abcdefghijkmnopqrstuvwxyzABCDEFGHJKLMNPQRSTUVWXYZ23456789"

// NewCode returns a random code of n characters. The alphabet leaves out
// characters that are easy to misread (0/O, 1/l/I).
func NewCode(n int) string {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		panic(err) // crypto/rand does not fail on supported platforms
	}
	for i, b := range buf {
		buf[i] = alphabet[int(b)%len(alphabet)]
	}
	return string(buf)
}
