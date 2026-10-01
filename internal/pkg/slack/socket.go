package slack

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
	"time"

	"golang.org/x/net/websocket"
)

// Event is the part of a message event the release bot uses.
type Event struct {
	Type     string `json:"type"`
	Subtype  string `json:"subtype"`
	Channel  string `json:"channel"`
	User     string `json:"user"`
	BotID    string `json:"bot_id"`
	Text     string `json:"text"`
	TS       string `json:"ts"`
	ThreadTS string `json:"thread_ts"`
}

// envelope is one Socket Mode frame; every frame with an envelope_id must be acknowledged.
type envelope struct {
	EnvelopeID string `json:"envelope_id"`
	Type       string `json:"type"`
	Reason     string `json:"reason"`
	Payload    struct {
		Event Event `json:"event"`
	} `json:"payload"`
}

var errDisconnect = errors.New("slack requested disconnect")

// maxPayloadBytes caps one Socket Mode frame, like the 1 MiB read limit on Web API responses.
const maxPayloadBytes = 1 << 20

// healthyConnection is how long a connection must last for the next reconnect to be immediate.
var healthyConnection = time.Minute

// eventQueueSize bounds events waiting for the handler; beyond it they are dropped and logged.
const eventQueueSize = 256

// RunSocketMode serves Socket Mode until ctx ends or a permanent error. The reader only acks and
// queues message events; one worker handles them in order, so a slow handler never delays acks.
func (c *Client) RunSocketMode(ctx context.Context, handle func(*Event), logf func(level, format string,
	args ...any),
) error {
	queue := make(chan *Event, eventQueueSize)
	workerCtx, stopWorker := context.WithCancel(ctx)
	workerDone := make(chan struct{})
	go func() {
		defer close(workerDone)
		for {
			select {
			case <-workerCtx.Done():
				return
			case e := <-queue:
				handle(e)
			}
		}
	}()
	defer func() { stopWorker(); <-workerDone }()

	backoff := time.Second
	for {
		started := time.Now()
		err := c.serveOnce(ctx, queue, logf)
		if ctx.Err() != nil {
			return ctx.Err()
		}
		var slackErr *Error
		if errors.As(err, &slackErr) && slackErr.Permanent() {
			return fmt.Errorf("socket mode cannot continue: %w", err)
		}
		// Only a connection that stayed up resets the backoff; quick disconnects (e.g.
		// too_many_websockets) back off like any other error instead of looping.
		if time.Since(started) >= healthyConnection {
			backoff = time.Second
			if errors.Is(err, errDisconnect) {
				continue // routine refresh: reconnect right away
			}
		}
		logf("warn", "socket mode connection ended: %v; reconnecting in %s", err, backoff)

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(backoff):
		}
		backoff = min(backoff*2, time.Minute)
	}
}

func (c *Client) serveOnce(ctx context.Context, queue chan<- *Event, logf func(level, format string,
	args ...any),
) error {
	var out struct {
		URL string `json:"url"`
	}
	if err := c.call(ctx, c.AppToken, "apps.connections.open", nil, &out); err != nil {
		return err
	}
	conn, err := c.dial(ctx, out.URL)
	if err != nil {
		return redactURL(err, out.URL)
	}
	defer conn.Close()
	conn.MaxPayloadBytes = maxPayloadBytes

	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stop()

	for {
		var env envelope
		if recvErr := websocket.JSON.Receive(conn, &env); recvErr != nil {
			return recvErr
		}
		if env.EnvelopeID != "" {
			ack := map[string]string{"envelope_id": env.EnvelopeID}
			if sendErr := websocket.JSON.Send(conn, ack); sendErr != nil {
				return sendErr
			}
		}

		switch env.Type {
		case "hello":
			logf("info", "socket mode connected")
		case "disconnect":
			logf("info", "socket mode disconnect requested (%s)", env.Reason)
			return errDisconnect
		case "events_api":
			if env.Payload.Event.Type != "message" {
				continue
			}

			e := env.Payload.Event
			select {
			case queue <- &e:
			default:
				logf("warn", "event queue full (%d); dropping message %s in %s", eventQueueSize, e.TS, e.Channel)
			}
		}
	}
}

// dial connects and handshakes within DialTimeout and ctx; reads then fail after IdleTimeout of
// silence and writes (acks) after IdleTimeout blocked, so a stuck connection gets replaced.
func (c *Client) dial(ctx context.Context, wsURL string) (*websocket.Conn, error) {
	cfg, err := websocket.NewConfig(wsURL, "https://slack.com")
	if err != nil {
		return nil, err
	}
	u := cfg.Location
	port := u.Port()
	if port == "" {
		port = "80"
		if u.Scheme == "wss" {
			port = "443"
		}
	}
	addr := net.JoinHostPort(u.Hostname(), port)

	dialCtx, cancel := context.WithTimeout(ctx, c.dialTimeout())
	defer cancel()
	var raw net.Conn
	if u.Scheme == "wss" {
		td := &tls.Dialer{Config: &tls.Config{ServerName: u.Hostname(), MinVersion: tls.VersionTLS12}}
		raw, err = td.DialContext(dialCtx, "tcp", addr)
	} else {
		raw, err = (&net.Dialer{}).DialContext(dialCtx, "tcp", addr)
	}
	if err != nil {
		return nil, err
	}

	// The handshake has no context: bound it with a deadline and close the conn on cancel.
	_ = raw.SetDeadline(time.Now().Add(c.dialTimeout()))
	stopClose := context.AfterFunc(dialCtx, func() { _ = raw.Close() })
	ic := &idleConn{Conn: raw}
	conn, err := websocket.NewClient(cfg, ic)
	if !stopClose() || err != nil {
		_ = raw.Close()
		if err == nil {
			err = dialCtx.Err()
		}

		return nil, err
	}
	_ = raw.SetDeadline(time.Time{})
	ic.idle = c.idleTimeout()

	return conn, nil
}

// idleConn renews the read deadline on every read, so any traffic (pings included) keeps it alive,
// and bounds every write, so a peer that stops reading cannot hang the reader in an ack.
type idleConn struct {
	net.Conn
	idle time.Duration // zero during the handshake
}

func (c *idleConn) Read(p []byte) (int, error) {
	if c.idle > 0 {
		_ = c.Conn.SetReadDeadline(time.Now().Add(c.idle))
	}

	return c.Conn.Read(p)
}

func (c *idleConn) Write(p []byte) (int, error) {
	if c.idle > 0 {
		_ = c.Conn.SetWriteDeadline(time.Now().Add(c.idle))
	}

	return c.Conn.Write(p)
}

type redactedError struct {
	msg   string
	cause error
}

func (e *redactedError) Error() string { return e.msg }
func (e *redactedError) Unwrap() error { return e.cause }

func redactURL(err error, wsURL string) error {
	msg := err.Error()
	for _, secret := range []string{wsURL, queryOf(wsURL), ticketOf(wsURL)} {
		if secret != "" {
			msg = strings.ReplaceAll(msg, secret, "<redacted>")
		}
	}

	return &redactedError{msg: msg, cause: err}
}

// queryOf and ticketOf find the secret parts even when the URL does not parse.
func queryOf(wsURL string) string {
	if _, query, found := strings.Cut(wsURL, "?"); found {
		return query
	}

	return ""
}

func ticketOf(wsURL string) string {
	if q, err := url.ParseQuery(queryOf(wsURL)); err == nil {
		return q.Get("ticket")
	}

	return ""
}

func (c *Client) dialTimeout() time.Duration {
	if c.DialTimeout > 0 {
		return c.DialTimeout
	}

	return 30 * time.Second
}

func (c *Client) idleTimeout() time.Duration {
	if c.IdleTimeout > 0 {
		return c.IdleTimeout
	}

	return 3 * time.Minute
}
