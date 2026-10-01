package slack

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Client calls the Web API with a bot token and opens Socket Mode connections with an app token.
type Client struct {
	BaseURL      string
	Token        string
	AppToken     string
	HTTP         *http.Client
	PostInterval time.Duration
	MaxRetries   int
	RetryBudget  time.Duration
	DialTimeout  time.Duration
	IdleTimeout  time.Duration

	// One post at a time; waiting for the slot honors ctx (a post can hold it through a Retry-After).
	postSlot     chan struct{}
	postSlotOnce sync.Once
	lastPost     time.Time
	limitMu      sync.Mutex
	limited      map[string]time.Time
}

func New(token, appToken string) *Client {
	return &Client{
		BaseURL: "https://slack.com/api", Token: token, AppToken: appToken,
		HTTP: &http.Client{Timeout: 60 * time.Second},
	}
}

type Message struct {
	Channel  string  `json:"channel"`
	Text     string  `json:"text,omitempty"`
	Blocks   []Block `json:"blocks,omitempty"`
	ThreadTS string  `json:"thread_ts,omitempty"`
}

type Block struct {
	Type     string      `json:"type"`
	Text     *BlockText  `json:"text,omitempty"`
	Fields   []BlockText `json:"fields,omitempty"`
	Elements []BlockText `json:"elements,omitempty"`
}

type BlockText struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type AuthInfo struct {
	UserID string `json:"user_id"`
	User   string `json:"user"`
	Team   string `json:"team"`
}

type Error struct {
	Method string
	Code   string
}

func (e *Error) Error() string { return "slack " + e.Method + ": " + e.Code }

// Permanent reports errors that retrying cannot fix. Only Slack's documented transient codes are
// retried; any other code (token, scope, workspace, request) ends the process so systemd sees it.
func (e *Error) Permanent() bool {
	switch e.Code {
	case "ratelimited", "internal_error", "fatal_error", "service_unavailable", "request_timeout",
		"org_login_required", "team_added_to_org":
		return false
	}

	return true
}

// AuthTest validates the bot token and returns who it belongs to.
func (c *Client) AuthTest(ctx context.Context) (AuthInfo, error) {
	var info AuthInfo
	err := c.call(ctx, c.Token, "auth.test", nil, &info)

	return info, err
}

// PostMessage posts msg and returns its ts. Posts are paced by PostInterval and 429s retried.
func (c *Client) PostMessage(ctx context.Context, msg *Message) (string, error) {
	c.postSlotOnce.Do(func() { c.postSlot = make(chan struct{}, 1) })
	select {
	case c.postSlot <- struct{}{}:
	case <-ctx.Done():
		return "", ctx.Err()
	}
	defer func() { <-c.postSlot }()

	if wait := time.Until(c.lastPost.Add(c.postInterval())); wait > 0 {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(wait):
		}
	}
	defer func() { c.lastPost = time.Now() }()

	var out struct {
		TS string `json:"ts"`
	}
	err := c.call(ctx, c.Token, "chat.postMessage", msg, &out)

	return out.TS, err
}

// call runs one method (JSON payload, or none), retrying HTTP 429 after the full Retry-After.
func (c *Client) call(ctx context.Context, token, method string, payload, out any) error {
	budget := c.retryBudget()
	for attempt := 0; ; attempt++ {
		// Respect a Retry-After still running from earlier calls, even ones that gave up.
		if wait := time.Until(c.limitedUntil(method)); wait > 0 {
			if wait > budget {
				return fmt.Errorf("slack %s: rate limited for another %s, over the remaining retry budget %s",
					method, wait.Round(time.Second), budget)
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(wait):
			}
			budget -= wait
		}

		raw, retryAfter, err := c.do(ctx, token, method, payload)
		if err != nil {
			return err
		}
		if retryAfter < 0 {
			return decode(method, raw, out)
		}

		c.limitUntil(method, time.Now().Add(retryAfter))
		if attempt >= c.maxRetries() {
			return fmt.Errorf("slack %s: rate limited, gave up after %d retries", method, attempt)
		}
		if retryAfter > budget {
			return fmt.Errorf("slack %s: rate limited for %s, over the remaining retry budget %s",
				method, retryAfter, budget)
		}
	}
}

func (c *Client) limitedUntil(method string) time.Time {
	c.limitMu.Lock()
	defer c.limitMu.Unlock()

	return c.limited[method]
}

func (c *Client) limitUntil(method string, until time.Time) {
	c.limitMu.Lock()
	defer c.limitMu.Unlock()
	if c.limited == nil {
		c.limited = map[string]time.Time{}
	}
	if until.After(c.limited[method]) {
		c.limited[method] = until
	}
}

// do sends one request; retryAfter >= 0 means HTTP 429 with that delay.
func (c *Client) do(ctx context.Context, token, method string, payload any) (raw []byte,
	retryAfter time.Duration, err error,
) {
	body, contentType := []byte{}, "application/x-www-form-urlencoded" // no arguments
	if payload != nil {
		if body, err = json.Marshal(payload); err != nil {
			return nil, -1, err
		}
		contentType = "application/json; charset=utf-8"
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/"+method, bytes.NewReader(body))
	if err != nil {
		return nil, -1, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", contentType)

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, -1, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusTooManyRequests {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<16))
		secs, convErr := strconv.Atoi(resp.Header.Get("Retry-After"))
		if convErr != nil || secs < 1 {
			secs = 1
		}

		return nil, time.Duration(secs) * time.Second, nil
	}

	raw, err = io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, -1, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, -1, fmt.Errorf("slack %s: %s", method, resp.Status)
	}

	return raw, -1, nil
}

func decode(method string, raw []byte, out any) error {
	var status struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
	}
	if err := json.Unmarshal(raw, &status); err != nil {
		return fmt.Errorf("slack %s: %w", method, err)
	}
	if !status.OK {
		return &Error{Method: method, Code: status.Error}
	}
	if out != nil {
		if err := json.Unmarshal(raw, out); err != nil {
			return fmt.Errorf("slack %s: decode response: %w", method, err)
		}
	}

	return nil
}

func (c *Client) postInterval() time.Duration {
	if c.PostInterval > 0 {
		return c.PostInterval
	}

	return time.Second
}

func (c *Client) maxRetries() int {
	if c.MaxRetries > 0 {
		return c.MaxRetries
	}

	return 3
}

func (c *Client) retryBudget() time.Duration {
	if c.RetryBudget > 0 {
		return c.RetryBudget
	}

	return 3 * time.Minute
}

// escaper turns the three characters Slack reads as markup into entities.
var escaper = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;")

// Escape makes untrusted text (a model's summary, a log line) plain in a Slack message: <!here>,
// <@user>, <#channel> and <url|label> are shown as text instead of mentioning or linking.
func Escape(s string) string { return escaper.Replace(s) }
