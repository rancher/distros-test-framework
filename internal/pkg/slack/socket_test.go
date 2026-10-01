package slack

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/net/websocket"
)

func noLog(string, string, ...any) {}

func msgFrame(id, ts string) map[string]any {
	return map[string]any{"envelope_id": id, "type": "events_api", "payload": map[string]any{"event": map[string]any{
		"type": "message", "channel": "C1", "user": "U1", "ts": ts, "text": "hi",
	}}}
}

// socketServer serves apps.connections.open and a websocket; ws runs once per connection.
func socketServer(t *testing.T, ws func(conn *websocket.Conn, n int32)) (c *Client, opens *atomic.Int32) {
	t.Helper()
	opens = &atomic.Int32{}
	var srv *httptest.Server
	mux := http.NewServeMux()
	mux.HandleFunc("/api/apps.connections.open", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer xapp-test" {
			t.Errorf("connections.open auth = %q", r.Header.Get("Authorization"))
		}
		opens.Add(1)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":  true,
			"url": "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws",
		})
	})
	mux.Handle("/ws", websocket.Handler(func(conn *websocket.Conn) { ws(conn, opens.Load()) }))
	srv = httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	return &Client{BaseURL: srv.URL + "/api", AppToken: "xapp-test", HTTP: srv.Client()}, opens
}

func drain(conn *websocket.Conn, acks chan<- string) {
	for {
		var ack map[string]string
		if websocket.JSON.Receive(conn, &ack) != nil {
			return
		}
		if acks != nil {
			acks <- ack["envelope_id"]
		}
	}
}

func expectAcks(t *testing.T, acks <-chan string, timeout <-chan struct{}, want ...string) {
	t.Helper()
	for _, w := range want {
		select {
		case id := <-acks:
			if id != w {
				t.Fatalf("ack %q, want %q", id, w)
			}
		case <-timeout:
			t.Fatalf("envelope %s was not acknowledged", w)
		}
	}
}

func TestSocketModeAcksDispatchesAndReconnects(t *testing.T) {
	acks := make(chan string, 10)
	c, opens := socketServer(t, func(conn *websocket.Conn, n int32) {
		_ = websocket.JSON.Send(conn, map[string]any{"type": "hello"})
		if n > 1 {
			drain(conn, nil)
			return
		}
		for _, fr := range []map[string]any{
			msgFrame("e1", "1.1"),
			{"envelope_id": "e2", "type": "events_api", "payload": map[string]any{"event": map[string]any{
				"type": "reaction_added", "user": "U1",
			}}},
		} {
			_ = websocket.JSON.Send(conn, fr)
			var ack map[string]string
			if websocket.JSON.Receive(conn, &ack) == nil {
				acks <- ack["envelope_id"]
			}
		}
		_ = websocket.JSON.Send(conn, map[string]any{"type": "disconnect", "reason": "refresh_requested"})
	})

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var mu sync.Mutex
	var got []Event
	done := make(chan error, 1)
	go func() {
		done <- c.RunSocketMode(ctx, func(e *Event) {
			mu.Lock()
			got = append(got, *e)
			mu.Unlock()
		}, noLog)
	}()

	expectAcks(t, acks, ctx.Done(), "e1", "e2")
	for opens.Load() < 2 && ctx.Err() == nil {
		time.Sleep(10 * time.Millisecond) // disconnect leads to a new connection
	}
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("RunSocketMode returned %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if opens.Load() < 2 || len(got) != 1 || got[0].TS != "1.1" || got[0].Channel != "C1" {
		t.Fatalf("opens=%d events=%+v", opens.Load(), got)
	}
}

// A slow handler must not delay acknowledgements; events still reach it in order.
func TestSocketModeAcksWhileHandlerIsSlow(t *testing.T) {
	acks := make(chan string, 16)
	c, _ := socketServer(t, func(conn *websocket.Conn, _ int32) {
		for _, fr := range []map[string]any{msgFrame("e1", "1"), msgFrame("e2", "2"), msgFrame("e3", "3")} {
			_ = websocket.JSON.Send(conn, fr)
		}
		drain(conn, acks)
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	release := make(chan struct{})
	var mu sync.Mutex
	var order []string
	done := make(chan error, 1)
	go func() {
		done <- c.RunSocketMode(ctx, func(e *Event) {
			if e.TS == "1" {
				<-release
			}
			mu.Lock()
			order = append(order, e.TS)
			mu.Unlock()
		}, noLog)
	}()

	deadline := time.After(time.Second)
	for _, want := range []string{"e1", "e2", "e3"} {
		select {
		case id := <-acks:
			if id != want {
				t.Fatalf("ack %q, want %q", id, want)
			}
		case <-deadline:
			t.Fatalf("envelope %s not acknowledged while the handler was busy", want)
		}
	}
	close(release)
	for {
		mu.Lock()
		n := len(order)
		mu.Unlock()
		if n == 3 {
			break
		}
		time.Sleep(time.Millisecond)
	}
	cancel()
	<-done
	if strings.Join(order, ",") != "1,2,3" {
		t.Fatalf("handler order %v", order)
	}
}

// A server that accepts TCP but never completes the handshake must not block reconnect or shutdown.
func TestSocketModeDialHonorsTimeoutAndCancel(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		for {
			conn, acceptErr := ln.Accept()
			if acceptErr != nil {
				return
			}
			go func() { time.Sleep(5 * time.Second); _ = conn.Close() }()
		}
	}()

	var opens atomic.Int32
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		opens.Add(1)
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "url": "ws://" + ln.Addr().String() + "/ws"})
	}))
	t.Cleanup(api.Close)
	c := &Client{BaseURL: api.URL, AppToken: "xapp", HTTP: api.Client(), DialTimeout: 200 * time.Millisecond}

	ctx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
	defer cancel()
	start := time.Now()
	err = c.RunSocketMode(ctx, func(*Event) {}, noLog)
	if !errors.Is(err, context.DeadlineExceeded) || time.Since(start) > 3*time.Second {
		t.Fatalf("RunSocketMode = %v after %s", err, time.Since(start))
	}
	if opens.Load() < 2 {
		t.Fatalf("a stuck handshake prevented reconnection (opens=%d)", opens.Load())
	}
}

// A silent (half-open) connection is dropped and re-opened.
func TestSocketModeReconnectsWhenSilent(t *testing.T) {
	c, opens := socketServer(t, func(conn *websocket.Conn, _ int32) {
		_ = websocket.JSON.Send(conn, map[string]any{"type": "hello"})
		time.Sleep(3 * time.Second)
	})
	c.IdleTimeout = 200 * time.Millisecond

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = c.RunSocketMode(ctx, func(*Event) {}, noLog)
	if opens.Load() < 2 {
		t.Fatalf("silent connection was never dropped (opens=%d)", opens.Load())
	}
}

// Errors that retrying cannot fix (token, workspace, request) stop the loop, so systemd sees the
// failure; Slack's transient codes keep reconnecting.
func TestSocketModeStopsOnPermanentErrors(t *testing.T) {
	for _, code := range []string{
		"token_revoked", "forbidden_team", "enterprise_is_restricted",
		"team_access_not_granted", "deprecated_endpoint", "invalid_arguments", "some_new_code",
	} {
		t.Run(code, func(t *testing.T) {
			c, opens := failingOpen(t, code)
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			err := c.RunSocketMode(ctx, func(*Event) {}, noLog)
			var se *Error
			if ctx.Err() != nil || !errors.As(err, &se) || se.Code != code || opens.Load() != 1 {
				t.Fatalf("RunSocketMode = %v (ctx %v, opens %d)", err, ctx.Err(), opens.Load())
			}
		})
	}

	c, opens := failingOpen(t, "service_unavailable")
	ctx, cancel := context.WithTimeout(context.Background(), 2500*time.Millisecond)
	defer cancel()
	if err := c.RunSocketMode(ctx, func(*Event) {}, noLog); !errors.Is(err, context.DeadlineExceeded) || opens.Load() < 2 {
		t.Fatalf("transient error: RunSocketMode = %v, opens = %d", err, opens.Load())
	}
}

func failingOpen(t *testing.T, code string) (*Client, *atomic.Int32) {
	t.Helper()
	opens := &atomic.Int32{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		opens.Add(1)
		_, _ = w.Write([]byte(`{"ok":false,"error":"` + code + `"}`))
	}))
	t.Cleanup(srv.Close)

	return &Client{BaseURL: srv.URL, AppToken: "xapp", HTTP: srv.Client()}, opens
}

// An ack to a peer that stopped reading fails after IdleTimeout instead of blocking the reader.
func TestIdleConnBoundsWrites(t *testing.T) {
	local, peer := net.Pipe() // writes block until the peer reads, and this peer never does
	defer peer.Close()
	c := &idleConn{Conn: local, idle: 100 * time.Millisecond}
	defer c.Close()

	done := make(chan error, 1)
	go func() {
		_, err := c.Write([]byte(`{"envelope_id":"e1"}`))
		done <- err
	}()
	select {
	case err := <-done:
		var ne net.Error
		if !errors.As(err, &ne) || !ne.Timeout() {
			t.Fatalf("Write = %v, want a timeout", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("write blocked past its deadline")
	}
}

// The Socket Mode URL carries a connection ticket that must never reach the logs.
func TestSocketModeRedactsTicket(t *testing.T) {
	const ticket = "SECRET-TICKET-1234"
	// url.Parse quotes the whole URL in its error; an invalid host exercises that path.
	badURL := "ws://bad host/link/?ticket=" + ticket + "&app_id=A1"
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "url": badURL})
	}))
	defer api.Close()
	c := &Client{BaseURL: api.URL, AppToken: "xapp", HTTP: api.Client(), DialTimeout: 200 * time.Millisecond}

	var mu sync.Mutex
	var logs []string
	ctx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
	defer cancel()
	_ = c.RunSocketMode(ctx, func(*Event) {}, func(_, format string, args ...any) {
		mu.Lock()
		logs = append(logs, fmt.Sprintf(format, args...))
		mu.Unlock()
	})
	mu.Lock()
	all := strings.Join(logs, "\n")
	mu.Unlock()
	if all == "" || strings.Contains(all, ticket) || strings.Contains(all, "app_id=A1") {
		t.Fatalf("ticket leaked or nothing logged:\n%s", all)
	}

	// The cause stays inspectable.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	closed := "ws://" + ln.Addr().String() + "/link/?ticket=" + ticket
	_ = ln.Close()
	_, dialErr := c.dial(context.Background(), closed)
	var opErr *net.OpError
	if red := redactURL(dialErr, closed); strings.Contains(red.Error(), ticket) || !errors.As(red, &opErr) {
		t.Fatalf("redacted error %q lost the redaction or its cause", red)
	}
}

// Disconnects right after connecting (too_many_websockets) back off instead of looping; a
// connection that stayed up is refreshed at once.
func TestSocketModeBacksOffQuickDisconnects(t *testing.T) {
	c, opens := socketServer(t, func(conn *websocket.Conn, _ int32) {
		_ = websocket.JSON.Send(conn, map[string]any{"type": "disconnect", "reason": "too_many_websockets"})
	})
	run := func(d time.Duration) int32 {
		opens.Store(0)
		ctx, cancel := context.WithTimeout(context.Background(), d)
		defer cancel()
		_ = c.RunSocketMode(ctx, func(*Event) {}, noLog)

		return opens.Load()
	}
	if n := run(2500 * time.Millisecond); n > 3 {
		t.Fatalf("%d connections in 2.5s: quick disconnects are not backed off", n)
	}
	healthyConnection = 0
	defer func() { healthyConnection = time.Minute }()
	if n := run(500 * time.Millisecond); n < 5 {
		t.Fatalf("%d connections in 0.5s: a healthy connection's refresh waited", n)
	}
}
