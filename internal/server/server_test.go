package server

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"

	"github.com/bernisnukic/hop/internal/store"
)

func newTestServer(t *testing.T) (*Server, *httptest.Server, *Metrics) {
	t.Helper()
	m := NewMetrics(prometheus.NewRegistry())
	s := New(store.NewMemory(), m, slog.New(slog.NewTextHandler(io.Discard, nil)), "https://hop.test/")
	s.SetReady()
	ts := httptest.NewServer(s.Handler())
	t.Cleanup(ts.Close)
	return s, ts, m
}

func noRedirect(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }

func create(t *testing.T, ts *httptest.Server, body string) (*http.Response, linkResponse) {
	t.Helper()
	resp, err := http.Post(ts.URL+"/api/links", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var l linkResponse
	_ = json.NewDecoder(resp.Body).Decode(&l)
	return resp, l
}

func TestCreateAndRedirect(t *testing.T) {
	_, ts, m := newTestServer(t)

	resp, l := create(t, ts, `{"url":"https://example.com/a?b=c"}`)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create: got %d", resp.StatusCode)
	}
	if len(l.Code) != codeLength || l.ShortURL != "https://hop.test/"+l.Code {
		t.Fatalf("unexpected link %+v", l)
	}

	client := &http.Client{CheckRedirect: noRedirect}
	for range 3 {
		r, err := client.Get(ts.URL + "/" + l.Code)
		if err != nil {
			t.Fatal(err)
		}
		r.Body.Close()
		if r.StatusCode != http.StatusFound || r.Header.Get("Location") != "https://example.com/a?b=c" {
			t.Fatalf("redirect: got %d to %q", r.StatusCode, r.Header.Get("Location"))
		}
	}

	r, err := http.Get(ts.URL + "/api/links/" + l.Code)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Body.Close()
	var got linkResponse
	if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if got.Hits != 3 {
		t.Fatalf("hits: got %d, want 3", got.Hits)
	}

	if n := testutil.ToFloat64(m.requests.WithLabelValues("GET /{code}", "302")); n != 3 {
		t.Fatalf("redirect metric: got %v, want 3", n)
	}
	if n := testutil.ToFloat64(m.created); n != 1 {
		t.Fatalf("created metric: got %v, want 1", n)
	}
}

func TestCreateRejectsBadURLs(t *testing.T) {
	_, ts, _ := newTestServer(t)
	for _, body := range []string{
		`not json`,
		`{"url":""}`,
		`{"url":"ftp://example.com"}`,
		`{"url":"javascript:alert(1)"}`,
		`{"url":"/relative"}`,
		`{"url":"https://example.com/` + strings.Repeat("a", maxURLLen) + `"}`,
	} {
		if resp, _ := create(t, ts, body); resp.StatusCode != http.StatusBadRequest {
			t.Errorf("%.40s: got %d, want 400", body, resp.StatusCode)
		}
	}
}

func TestUnknownCode(t *testing.T) {
	_, ts, m := newTestServer(t)
	for _, path := range []string{"/nope123", "/api/links/nope123"} {
		r, err := http.Get(ts.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		r.Body.Close()
		if r.StatusCode != http.StatusNotFound {
			t.Errorf("%s: got %d, want 404", path, r.StatusCode)
		}
	}
	// Unmatched paths use a fixed label, not the raw path.
	r, _ := http.Post(ts.URL+"/x/y/z", "text/plain", nil)
	r.Body.Close()
	if n := testutil.ToFloat64(m.requests.WithLabelValues("unmatched", "404")); n != 1 {
		t.Fatalf("unmatched metric: got %v, want 1", n)
	}
}

func TestReadiness(t *testing.T) {
	m := NewMetrics(prometheus.NewRegistry())
	s := New(store.NewMemory(), m, slog.New(slog.NewTextHandler(io.Discard, nil)), "")
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()

	status := func(path string) int {
		r, err := http.Get(ts.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		r.Body.Close()
		return r.StatusCode
	}

	if got := status("/readyz"); got != http.StatusServiceUnavailable {
		t.Fatalf("before ready: got %d", got)
	}
	if got := status("/healthz"); got != http.StatusOK {
		t.Fatalf("liveness before ready: got %d", got)
	}
	s.SetReady()
	if got := status("/readyz"); got != http.StatusOK {
		t.Fatalf("ready: got %d", got)
	}
	s.Drain()
	if got := status("/readyz"); got != http.StatusServiceUnavailable {
		t.Fatalf("draining: got %d", got)
	}
	if got := status("/healthz"); got != http.StatusOK {
		t.Fatalf("liveness while draining: got %d", got)
	}
}
