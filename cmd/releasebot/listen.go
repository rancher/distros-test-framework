package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/rancher/distros-test-framework/internal/pkg/releasebot"
	"github.com/rancher/distros-test-framework/internal/pkg/slack"
	"github.com/rancher/distros-test-framework/internal/resources"
)

// listenConfig is what listen mode reads from the environment.
type listenConfig struct {
	botToken, appToken string
	channels, open     map[string]bool
	allowed            map[string]bool
}

// envSet reads a comma-separated list of ids from the environment.
func envSet(name string) map[string]bool {
	set := map[string]bool{}
	for _, v := range strings.Split(os.Getenv(name), ",") {
		if v = strings.TrimSpace(v); v != "" {
			set[v] = true
		}
	}

	return set
}

// readListenConfig: RELEASEBOT_CHANNELS (or the older RELEASEBOT_CHANNEL) lists the channels
// served; RELEASEBOT_OPEN_CHANNELS are served too, and there anyone may start runs.
func readListenConfig() (*listenConfig, error) {
	c := &listenConfig{
		botToken: os.Getenv("SLACK_BOT_TOKEN"), appToken: os.Getenv("SLACK_APP_TOKEN"),
		channels: envSet("RELEASEBOT_CHANNELS"), open: envSet("RELEASEBOT_OPEN_CHANNELS"),
		allowed: envSet("RELEASEBOT_ALLOWED_USERS"),
	}
	for ch := range envSet("RELEASEBOT_CHANNEL") {
		c.channels[ch] = true
	}
	for ch := range c.open {
		c.channels[ch] = true
	}
	if c.botToken == "" || c.appToken == "" || len(c.channels) == 0 {
		return nil, errors.New("listen mode needs SLACK_BOT_TOKEN, SLACK_APP_TOKEN and RELEASEBOT_CHANNELS")
	}
	if len(c.allowed) == 0 && len(c.open) == 0 {
		resources.LogLevel("warn", "no RELEASEBOT_ALLOWED_USERS and no open channel: every request stays a dry-run")
	}

	return c, nil
}

// runListen serves the configured Slack channels over Socket Mode until SIGINT/SIGTERM.
func runListen(o *options) error {
	cfg, err := readListenConfig()
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	release, err := releasebot.AcquireInstanceLock(o.stateFile + ".lock")
	if err != nil {
		return err
	}
	defer release()

	sc := slack.New(cfg.botToken, cfg.appToken)
	auth, err := sc.AuthTest(ctx)
	if err != nil {
		return fmt.Errorf("slack auth: %w", err)
	}

	botID := auth.UserID
	resources.LogLevel("info", "release bot %s serving %d channels (%d open to anyone), %d allowed users",
		botID, len(cfg.channels), len(cfg.open), len(cfg.allowed))

	app := newApp(o)
	var failures *releasebot.FailureTriage
	if o.triageSpool != "" {
		failures = app.FailureTriage(o.triageSpool, o.triageWait)
	}

	// One capacity for all runs and the listener: `unblock` frees the slots a run's unknown builds hold.
	capacity := &releasebot.Capacity{}
	l, err := newListener(o, sc, cfg, botID, capacity, app.Planner(capacity, failures))
	if err != nil {
		return err
	}

	err = sc.RunSocketMode(ctx, func(e *slack.Event) { l.Handle(ctx, e) }, resources.LogLevel)
	l.Wait()
	if errors.Is(err, context.Canceled) {
		return nil
	}

	return err
}

// newListener wires the listener to Slack and restores its persisted block state.
func newListener(
	o *options, sc *slack.Client, cfg *listenConfig, botID string,
	capacity *releasebot.Capacity, plan releasebot.Planner,
) (*releasebot.Listener, error) {
	l := &releasebot.Listener{
		Channels:     cfg.channels,
		OpenChannels: cfg.open,
		BotUserID:    botID,
		Allowed:      cfg.allowed,
		DryRun:       o.dryRun,
		StatePath:    o.stateFile,
		Capacity:     capacity,
		Plan:         plan,
		Post: func(ctx context.Context, channel, threadTS, text string) error {
			msg := &slack.Message{Channel: channel, ThreadTS: threadTS, Text: text}
			if _, postErr := sc.PostMessage(ctx, msg); postErr != nil {
				resources.LogLevel("error", "slack post in %s thread %s: %v", channel, threadTS, postErr)
				return postErr
			}

			return nil
		},
	}

	if err := l.Restore(); err != nil {
		return nil, err
	}
	if reason := l.Blocked(); reason != "" {
		resources.LogLevel("warn", "starting blocked: %s (reply `unblock` in that thread)", reason)
	}
	if o.dryRun {
		resources.LogLevel("info", "dry-run mode: requests get the plan only; start with -dry-run=false to execute")
	}

	return l, nil
}

// defaultStateFile is <user config dir>/releasebot/state.json, or a local file if there is none.
func defaultStateFile() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "releasebot-state.json"
	}

	return filepath.Join(dir, "releasebot", "state.json")
}
