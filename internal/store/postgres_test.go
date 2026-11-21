package store

import (
	"context"
	"errors"
	"os"
	"sync"
	"testing"
)

// These tests need a real database: HOP_TEST_DATABASE_URL=postgres://...
// CI runs them against a Postgres service container.
func testPostgres(t *testing.T) *Postgres {
	t.Helper()
	url := os.Getenv("HOP_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("HOP_TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	p, err := OpenPostgres(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(p.Close)
	if _, err := p.pool.Exec(ctx, `DROP TABLE IF EXISTS links, schema_migrations`); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestPostgresConcurrentMigrate(t *testing.T) {
	p := testPostgres(t)
	var wg sync.WaitGroup
	errs := make(chan error, 5)
	for range 5 { // like five replicas starting at once
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs <- p.Migrate(context.Background())
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	var n int
	if err := p.pool.QueryRow(context.Background(), `SELECT count(*) FROM schema_migrations`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("schema_migrations rows: got %d, want 1", n)
	}
}

func TestPostgresLinks(t *testing.T) {
	p := testPostgres(t)
	ctx := context.Background()
	if err := p.Migrate(ctx); err != nil {
		t.Fatal(err)
	}

	if _, err := p.Create(ctx, "abc1234", "https://example.com"); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Create(ctx, "abc1234", "https://example.org"); !errors.Is(err, ErrCodeTaken) {
		t.Fatalf("duplicate code: got %v, want ErrCodeTaken", err)
	}
	for range 2 {
		if _, err := p.Resolve(ctx, "abc1234"); err != nil {
			t.Fatal(err)
		}
	}
	l, err := p.Get(ctx, "abc1234")
	if err != nil {
		t.Fatal(err)
	}
	if l.Hits != 2 || l.URL != "https://example.com" {
		t.Fatalf("got %+v", l)
	}
	if _, err := p.Resolve(ctx, "missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing: got %v, want ErrNotFound", err)
	}
}
