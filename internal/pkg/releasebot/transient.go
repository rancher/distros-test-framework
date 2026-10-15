package releasebot

import "regexp"

// Only these rules grant a rerun (the verdict can only veto): no spec failed and the primary failure
// matches. Keep the list narrow, one ID per rule, with positive and negative tests per change.

// TransientRulesVersion identifies the rule set in rerun messages.
const TransientRulesVersion = "2026-09-29.2"

// TransientRule is one known transient infrastructure failure.
type TransientRule struct {
	ID      string
	pattern *regexp.Regexp
}

// Ansible UNREACHABLE is not here: it also covers bad keys, inventory and security groups.
var transientRules = []TransientRule{
	// AWS had no capacity for the instance type in that zone at that moment.
	{"aws-insufficient-capacity", regexp.MustCompile(`InsufficientInstanceCapacity`)},
	// AWS API throttling.
	{"aws-throttled", regexp.MustCompile(`RequestLimitExceeded|Throttling: Rate exceeded`)},
	// AWS answered with a server-side error.
	{"aws-server-error", regexp.MustCompile(
		`\b(InternalError|ServiceUnavailable|Unavailable): .*status code: 5\d\d`)},
	// The jenkins agent running the build disconnected.
	{agentLostRule, regexp.MustCompile(
		`hudson\.remoting\.ChannelClosedException|Agent went offline during the build`)},
	// Docker Hub pull rate limit.
	{"docker-pull-rate-limit", regexp.MustCompile(`toomanyrequests: You have reached your pull rate limit`)},
}

const agentLostRule = "jenkins-agent-lost"

func matchesRule(id, l string) bool {
	for i := range transientRules {
		if transientRules[i].ID == id {
			return transientRules[i].pattern.MatchString(l)
		}
	}

	return false
}

func matchesTransientLine(l string) bool {
	for i := range transientRules {
		if transientRules[i].pattern.MatchString(l) {
			return true
		}
	}

	return false
}

// MatchTransient returns the rule that grants a rerun for this failure, or nil.
func MatchTransient(f *Failure) *TransientRule {
	if f == nil || f.TestFailed || f.Primary == "" {
		return nil
	}
	for i := range transientRules {
		if transientRules[i].pattern.MatchString(f.Primary) {
			return &transientRules[i]
		}
	}

	return nil
}
