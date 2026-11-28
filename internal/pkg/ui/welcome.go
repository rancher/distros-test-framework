package ui

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/chromedp/chromedp"
)

const (
	defaultWelcomeWait = 10 * time.Second
	// dialogsJS lists the elements Rancher's dashboard renders modals with.
	dialogsJS = `[...document.querySelectorAll('[role="dialog"],[aria-modal="true"],` +
		`.modal-container,.v--modal-box,.modal-dialog,.modal')]`
)

// welcomeJS finds a visible "Welcome to ..." dialog (Rancher Prime 2.16+ shows one on first load) and
// clicks its close control. It returns "" when there is no such dialog, "NOCLOSE" when it has no
// recognizable close control, or a short description of what it clicked.
const welcomeJS = `(() => {
	const visible = el => { const r = el.getBoundingClientRect(); return r.width > 0 && r.height > 0; };
	const dialog = ` + dialogsJS + `
		.filter(visible).find(d => /welcome to/i.test(d.innerText || ''));
	if (!dialog) return '';
	const attrs = el => [el.getAttribute('aria-label'), el.getAttribute('data-testid'), el.getAttribute('title'),
		String((el.className && el.className.baseVal) || el.className || '')].join(' ');
	const closeText = /^(close|×|✕|x|got it|get started|dismiss|skip|continue|ok)$/i;
	const control = [...dialog.querySelectorAll('button,[role="button"],a,i,span')].filter(visible)
		.find(el => /close|dismiss/i.test(attrs(el)) || closeText.test((el.innerText || '').trim()));
	if (!control) return 'NOCLOSE';
	control.click();
	return (control.getAttribute('aria-label') || control.getAttribute('data-testid') ||
		(control.innerText || '').trim() || control.tagName).slice(0, 60);
})()`

const welcomeOpenJS = dialogsJS + `.some(d => { const r = d.getBoundingClientRect();
	return r.width > 0 && r.height > 0 && /welcome to/i.test(d.innerText || ''); })`

// dismissWelcome closes the welcome dialog if it shows up within wait, then confirms it is gone.
func (b *Browser) dismissWelcome(wait time.Duration) error {
	deadline := time.Now().Add(wait)
	for {
		var open bool
		if err := b.run(15*time.Second, chromedp.Evaluate(welcomeOpenJS, &open)); err != nil {
			return fmt.Errorf("look for welcome dialog: %w", err)
		}
		if !open {
			if time.Now().After(deadline) {
				return nil
			}
			time.Sleep(time.Second)

			continue
		}

		b.shootWelcomeOnce()
		var clicked string
		if err := b.run(15*time.Second, chromedp.Evaluate(welcomeJS, &clicked)); err != nil {
			return fmt.Errorf("close welcome dialog: %w", err)
		}
		if clicked == "NOCLOSE" {
			return b.welcomeStuck("it has no recognizable close control")
		}
		if err := b.run(15*time.Second, chromedp.Poll(`!(`+welcomeOpenJS+`)`, nil,
			chromedp.WithPollingInterval(500*time.Millisecond))); err != nil {
			return b.welcomeStuck("clicking " + clicked + " did not close it")
		}

		return nil
	}
}

// shootWelcomeOnce keeps evidence of the popup, before closing it, the first time it appears.
func (b *Browser) shootWelcomeOnce() {
	b.mu.Lock()
	first := !b.welcomeShot
	b.welcomeShot = true
	b.mu.Unlock()
	if first {
		_, _ = b.Screenshot("rancher-welcome-dialog")
	}
}

// welcomeStuck saves the dialog markup so its close control can be matched, and returns an actionable error.
func (b *Browser) welcomeStuck(reason string) error {
	var html string
	_ = b.run(10*time.Second, chromedp.Evaluate(`(() => { const d = `+dialogsJS+
		`.find(d => /welcome to/i.test(d.innerText || '')); return d ? d.outerHTML : ''; })()`, &html))
	msg := "rancher welcome dialog blocks the page: " + reason
	if html == "" || b.showsSecret(html) {
		return errors.New(msg)
	}
	path := filepath.Join(b.dir, "welcome-dialog.html")
	if err := os.WriteFile(path, []byte(html), 0o600); err != nil {
		return fmt.Errorf("%s (saving its markup failed: %w)", msg, err)
	}

	return fmt.Errorf("%s; its markup is in %s", msg, path)
}
