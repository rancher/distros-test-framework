// Package ui drives a headless Chrome against the Rancher dashboard to capture evidence screenshots.
package ui

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"
)

// DropdownMenuItems matches vue-select options, the widget behind Rancher's version dropdowns.
const DropdownMenuItems = ".vs__dropdown-menu > li"

var unsafeName = regexp.MustCompile(`[^a-zA-Z0-9_.-]+`)

// Browser is a single headless Chrome tab with an authenticated Rancher session.
type Browser struct {
	ctx     context.Context
	cancel  context.CancelFunc
	baseURL string
	dir     string

	welcomeWait time.Duration

	mu          sync.Mutex
	secrets     []string
	console     []string
	shots       int
	welcomeSeen bool // a later page load only checks once instead of waiting for the dialog
	welcomeShot bool
}

// Options configure New.
type Options struct {
	BaseURL      string
	SessionToken string // set as the R_SESS cookie, so no password is ever typed into the page
	TrustedSPKI  string // base64 sha256 of the only self-signed key Chrome may accept; empty trusts system CAs
	ArtifactsDir string
	WelcomeWait  time.Duration // how long the first page load waits for a welcome dialog; 0 means 10s
}

// New starts Chrome and opens an authenticated session.
func New(opts Options) (*Browser, error) {
	if err := os.MkdirAll(opts.ArtifactsDir, 0o700); err != nil {
		return nil, fmt.Errorf("create screenshot dir: %w", err)
	}

	allocOpts := append([]chromedp.ExecAllocatorOption{}, chromedp.DefaultExecAllocatorOptions[:]...)
	allocOpts = append(allocOpts, chromedp.WindowSize(1600, 1000))
	if opts.TrustedSPKI != "" {
		allocOpts = append(allocOpts, chromedp.Flag("ignore-certificate-errors-spki-list", opts.TrustedSPKI))
	}
	if os.Geteuid() == 0 {
		// Chrome refuses to start as root with its sandbox; the test containers run as root.
		allocOpts = append(allocOpts, chromedp.NoSandbox)
	}

	allocCtx, allocCancel := chromedp.NewExecAllocator(context.Background(), allocOpts...)
	ctx, ctxCancel := chromedp.NewContext(allocCtx)
	b := &Browser{
		ctx:         ctx,
		cancel:      func() { ctxCancel(); allocCancel() },
		baseURL:     strings.TrimRight(opts.BaseURL, "/"),
		dir:         opts.ArtifactsDir,
		welcomeWait: opts.WelcomeWait,
	}
	if b.welcomeWait == 0 {
		b.welcomeWait = defaultWelcomeWait
	}
	if opts.SessionToken != "" {
		b.AddSecrets(opts.SessionToken)
	}

	chromedp.ListenTarget(ctx, func(ev any) {
		if e, ok := ev.(*runtime.EventConsoleAPICalled); ok {
			b.recordConsole(e)
		}
	})

	// The first Run owns the browser process, so it must not use a cancelable child context.
	if err := chromedp.Run(ctx); err != nil {
		b.Close()
		return nil, fmt.Errorf("start chrome: %w", err)
	}

	start := []chromedp.Action{network.Enable()}
	if opts.SessionToken != "" {
		start = append(start, network.SetCookie("R_SESS", opts.SessionToken).
			WithURL(b.baseURL).WithPath("/").WithHTTPOnly(true).WithSecure(true))
	}
	if err := b.run(60*time.Second, start...); err != nil {
		b.Close()
		return nil, fmt.Errorf("set rancher session cookie: %w", err)
	}

	return b, nil
}

// AddSecrets registers values that must never appear in a screenshot.
func (b *Browser) AddSecrets(secrets ...string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, s := range secrets {
		if s != "" {
			b.secrets = append(b.secrets, s)
		}
	}
}

// Close stops Chrome.
func (b *Browser) Close() { b.cancel() }

// Open navigates to path (relative to the base URL) and waits until the page shows waitText.
func (b *Browser) Open(path, waitText string, timeout time.Duration) error {
	text, err := json.Marshal(waitText)
	if err != nil {
		return err
	}
	cond := fmt.Sprintf("document.body && document.body.innerText.includes(%s)", text)

	deadline := time.Now().Add(timeout)
	err = b.run(timeout, chromedp.Navigate(b.baseURL+path))
	for err == nil || navigatedAway(err) {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			err = fmt.Errorf("page kept navigating: %w", context.DeadlineExceeded)

			break
		}
		if err = b.run(remaining, chromedp.Poll(cond, nil, chromedp.WithPollingInterval(time.Second))); err == nil {
			break
		}
	}
	if err != nil {
		return fmt.Errorf("open %s waiting for %q: %w", path, waitText, err)
	}

	// Rancher Prime 2.16+ opens a "Welcome to Rancher Prime" dialog over the first page; it blocks clicks.
	b.mu.Lock()
	wait := b.welcomeWait
	if b.welcomeSeen {
		wait = 0
	}
	b.welcomeSeen = true
	b.mu.Unlock()
	if err = b.dismissWelcome(wait); err != nil {
		return fmt.Errorf("open %s: %w", path, err)
	}

	return nil
}

// DropdownOptions opens the select matching selector, scrolls highlight into view and returns every option label.
func (b *Browser) DropdownOptions(selector, highlight string, timeout time.Duration) ([]string, error) {
	h, err := json.Marshal(highlight)
	if err != nil {
		return nil, err
	}
	items, err := json.Marshal(DropdownMenuItems)
	if err != nil {
		return nil, err
	}
	js := fmt.Sprintf(`(() => {
		const items = [...document.querySelectorAll(%s)];
		const hit = items.find(li => li.innerText.includes(%s));
		if (hit) { hit.scrollIntoView({block: 'center'}); hit.style.outline = '3px solid red'; }
		return items.map(li => li.innerText.trim());
	})()`, items, h)

	if welcomeErr := b.dismissWelcome(0); welcomeErr != nil {
		return nil, welcomeErr
	}
	var options []string
	err = b.run(timeout,
		chromedp.WaitVisible(selector, chromedp.ByQuery),
		chromedp.Click(selector, chromedp.ByQuery),
		chromedp.WaitVisible(DropdownMenuItems, chromedp.ByQuery),
		chromedp.Evaluate(js, &options),
	)
	if err != nil {
		return nil, fmt.Errorf("read dropdown %s: %w", selector, err)
	}

	return options, nil
}

// Screenshot saves the visible page as NN-name.png unless the page shows a registered secret.
func (b *Browser) Screenshot(name string) (string, error) {
	var page string
	err := b.run(30*time.Second, chromedp.Evaluate(`document.documentElement.innerText + "\n" +
		[...document.querySelectorAll('input, textarea')].map(e => e.value).join("\n") + "\n" + location.href`, &page))
	if err != nil {
		return "", fmt.Errorf("inspect page before screenshot: %w", err)
	}
	if b.showsSecret(page) {
		return "", errors.New("refusing screenshot " + name + ": page shows a credential or registration token")
	}

	var png []byte
	if err = b.run(30*time.Second, chromedp.CaptureScreenshot(&png)); err != nil {
		return "", fmt.Errorf("capture screenshot %s: %w", name, err)
	}

	path := filepath.Join(b.dir, fmt.Sprintf("%02d-%s.png", b.nextShot(), unsafeName.ReplaceAllString(name, "_")))
	if err = os.WriteFile(path, png, 0o600); err != nil {
		return "", fmt.Errorf("write screenshot: %w", err)
	}

	return path, nil
}

// SaveConsole writes the browser console messages seen so far, for failure debugging.
func (b *Browser) SaveConsole(name string) (string, error) {
	b.mu.Lock()
	lines := strings.Join(b.console, "\n")
	b.mu.Unlock()

	path := filepath.Join(b.dir, unsafeName.ReplaceAllString(name, "_")+"-console.log")

	return path, os.WriteFile(path, []byte(lines), 0o600)
}

// navigatedAway reports Chrome losing the page's JS context to a redirect or reload during a wait;
// the dashboard does this on load, so the wait just restarts on the new page.
func navigatedAway(err error) bool {
	msg := err.Error()

	return strings.Contains(msg, "Cannot find context with specified id") ||
		strings.Contains(msg, "Execution context was destroyed") ||
		strings.Contains(msg, "Inspected target navigated or closed")
}

func (b *Browser) run(timeout time.Duration, actions ...chromedp.Action) error {
	ctx, cancel := context.WithTimeout(b.ctx, timeout)
	defer cancel()

	return chromedp.Run(ctx, actions...)
}

func (b *Browser) showsSecret(page string) bool {
	b.mu.Lock()
	defer b.mu.Unlock()

	return b.showsSecretLocked(page)
}

func (b *Browser) nextShot() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.shots++

	return b.shots
}

func (b *Browser) recordConsole(e *runtime.EventConsoleAPICalled) {
	parts := make([]string, 0, len(e.Args))
	for _, a := range e.Args {
		parts = append(parts, strings.Trim(string(a.Value), `"`))
	}
	line := fmt.Sprintf("[%s] %s", e.Type, strings.Join(parts, " "))

	b.mu.Lock()
	defer b.mu.Unlock()
	if !b.showsSecretLocked(line) {
		b.console = append(b.console, line)
	}
}

func (b *Browser) showsSecretLocked(s string) bool {
	for _, secret := range b.secrets {
		if strings.Contains(s, secret) {
			return true
		}
	}

	return false
}
