package main

import (
	"errors"
	"os"

	"github.com/rancher/distros-test-framework/internal/pkg/releasebot"
	"github.com/rancher/distros-test-framework/internal/resources"
)

// main picks the mode from the flags (see options.go) and runs it.
func main() {
	o := parseFlags()

	runFn := run
	switch {
	case o.validate: // wins over -listen: a deploy check must never connect to Slack
		runFn = validateMatrix
	case o.triageBroker:
		runFn = runBroker
	case o.triageBuild != "":
		runFn = runTriageBuild
	case o.listen:
		runFn = runListen
	}
	if err := runFn(o); err != nil {
		resources.LogLevel("error", "releasebot: %v", err)
		if errors.Is(err, releasebot.ErrModelAccess) {
			os.Exit(77) // releasebot-triage@.service does not restart on it: a person signs in first
		}
		os.Exit(1)
	}
}
