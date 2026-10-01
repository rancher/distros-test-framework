package slack

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func testClient(srv *httptest.Server) *Client {
	return &Client{
		BaseURL: srv.URL, Token: "xoxb-test", AppToken: "xapp-test", HTTP: srv.Client(),
		PostInterval: time.Millisecond,
	}
}

func TestPostMessageSendsJSON(t *testing.T) {
	var got Message
	var auth, contentType string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth, contentType = r.Header.Get("Authorization"), r.Header.Get("Content-Type")
		_ = json.NewDecoder(r.Body).Decode(&got)
		_, _ = w.Write([]byte(`{"ok":true,"ts":"2.2"}`))
	}))
	defer srv.Close()

	ts, err := testClient(srv).PostMessage(context.Background(), &Message{
		Channel: "C1", ThreadTS: "1.1",
		Text: "hello", Blocks: []Block{{Type: "section", Text: &BlockText{Type: "mrkdwn", Text: "*x*"}}},
	})
	if err != nil || ts != "2.2" {
		t.Fatalf("ts=%q err=%v", ts, err)
	}
	if got.Channel != "C1" || got.ThreadTS != "1.1" || got.Text != "hello" || len(got.Blocks) != 1 ||
		auth != "Bearer xoxb-test" || !strings.HasPrefix(contentType, "application/json") {
		t.Fatalf("request: %+v auth=%q type=%q", got, auth, contentType)
	}
}

func TestErrorsAndPermanentCodes(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"ok":false,"error":"invalid_auth"}`))
	}))
	defer srv.Close()

	_, err := testClient(srv).AuthTest(context.Background())
	var se *Error
	if !errors.As(err, &se) || se.Code != "invalid_auth" || !se.Permanent() {
		t.Fatalf("AuthTest error = %v", err)
	}
	for _, code := range []string{"ratelimited", "internal_error", "service_unavailable", "request_timeout"} {
		if (&Error{Code: code}).Permanent() {
			t.Fatalf("%s is transient", code)
		}
	}
}

func TestRetriesRateLimitedPosts(t *testing.T) {
	var calls atomic.Int32
	var limitAlways atomic.Bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if calls.Add(1) == 1 || limitAlways.Load() {
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()
	c := testClient(srv)

	start := time.Now()
	if _, err := c.PostMessage(context.Background(), &Message{Channel: "C1", Text: "summary"}); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 2 || time.Since(start) < time.Second {
		t.Fatalf("calls=%d after %s: Retry-After not honored", calls.Load(), time.Since(start))
	}

	limitAlways.Store(true)
	c.MaxRetries = 1
	if _, err := c.PostMessage(context.Background(), &Message{Channel: "C1", Text: "x"}); err == nil ||
		!strings.Contains(err.Error(), "rate limited") {
		t.Fatalf("persistent 429: %v", err)
	}
}

func TestRetryAfterIsNotShortened(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Retry-After", "120")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()

	_, wait, err := testClient(srv).do(context.Background(), "xoxb", "chat.postMessage", nil)
	if err != nil || wait != 120*time.Second {
		t.Fatalf("Retry-After 120 gave wait=%s err=%v", wait, err)
	}
}

// A wait beyond the budget fails at once instead of retrying before Slack allows it.
func TestRetryAfterBeyondBudgetFailsWithoutRetry(t *testing.T) {
	var calls atomic.Int32
	var retryAfter atomic.Value
	retryAfter.Store("120")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if calls.Add(1) == 1 {
			w.Header().Set("Retry-After", retryAfter.Load().(string))
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()
	c := testClient(srv)
	c.RetryBudget = 30 * time.Second

	start := time.Now()
	_, err := c.PostMessage(context.Background(), &Message{Channel: "C1", Text: "m"})
	if err == nil || !strings.Contains(err.Error(), "over the remaining retry budget") ||
		calls.Load() != 1 || time.Since(start) > time.Second {
		t.Fatalf("err=%v calls=%d after %s", err, calls.Load(), time.Since(start))
	}

	calls.Store(0)
	retryAfter.Store("2")
	c = testClient(srv) // a fresh client: the first one is still inside the 120s window
	c.RetryBudget = 30 * time.Second
	start = time.Now()
	if _, err = c.PostMessage(context.Background(), &Message{Channel: "C1", Text: "m"}); err != nil ||
		calls.Load() != 2 || time.Since(start) < 2*time.Second {
		t.Fatalf("Retry-After 2: err=%v calls=%d after %s", err, calls.Load(), time.Since(start))
	}
}

func TestPostsArePaced(t *testing.T) {
	var mu sync.Mutex
	var at []time.Time
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		mu.Lock()
		at = append(at, time.Now())
		mu.Unlock()
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()
	c := testClient(srv)
	c.PostInterval = 150 * time.Millisecond

	for range 3 {
		if _, err := c.PostMessage(context.Background(), &Message{Channel: "C1", Text: "m"}); err != nil {
			t.Fatal(err)
		}
	}
	for i := 1; i < len(at); i++ {
		if gap := at[i].Sub(at[i-1]); gap < 140*time.Millisecond {
			t.Fatalf("posts %d and %d only %s apart", i-1, i, gap)
		}
	}
}

// The Retry-After deadline outlives the call that gave up: later messages wait for it too.
func TestRateLimitDeadlineIsKeptBetweenCalls(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.Header().Set("Retry-After", "120")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()
	c := testClient(srv)
	c.RetryBudget = 30 * time.Second

	if _, err := c.PostMessage(context.Background(), &Message{Channel: "C1", Text: "first"}); err == nil {
		t.Fatal("first post should give up")
	}
	start := time.Now()
	_, err := c.PostMessage(context.Background(), &Message{Channel: "C1", Text: "second"})
	if err == nil || !strings.Contains(err.Error(), "rate limited for another") || calls.Load() != 1 ||
		time.Since(start) > time.Second {
		t.Fatalf("second post: err=%v calls=%d after %s", err, calls.Load(), time.Since(start))
	}
	if _, err = c.AuthTest(context.Background()); err == nil && calls.Load() != 2 {
		t.Fatalf("other methods have their own limit (calls=%d)", calls.Load())
	}
}

// A post waiting behind another one (held by a long Retry-After) returns as soon as its ctx ends.
func TestWaitingPostHonorsContext(t *testing.T) {
	release, arrived := make(chan struct{}), make(chan struct{}, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		arrived <- struct{}{}
		<-release
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()
	defer close(release)
	c := testClient(srv)

	go func() { _, _ = c.PostMessage(context.Background(), &Message{Channel: "C1", Text: "first"}) }()
	<-arrived
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	start := time.Now()
	if _, err := c.PostMessage(ctx, &Message{Channel: "C1", Text: "second"}); !errors.Is(err, context.DeadlineExceeded) ||
		time.Since(start) > time.Second {
		t.Fatalf("err %v after %s", err, time.Since(start))
	}
}
