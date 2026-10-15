package releasebot

import (
	"strings"
	"testing"
)

// provisioningLog is a build that failed in TestMain while provisioning (no Ginkgo spec ran):
// cause is the tofu/pipeline error; cleanup lines follow it.
func provisioningLog(cause string, cleanup ...string) string {
	lines := append([]string{
		"[Pipeline] { (Run TestCombination)",
		"Apply started",
		cause,
		"  with module.cluster.aws_instance.node[1]",
		`time="2026-09-29T00:50:07Z" level=error msg="error provisioning infrastructure: tofu apply: exit status 1"`,
	}, cleanup...)

	return strings.Join(append(lines, "Finished: FAILURE"), "\n")
}

// Each rule grants on its own primary failure, and never for the same text found only in
// cleanup after another primary failure, nor when a test failed.
func TestTransientRules(t *testing.T) {
	persistent := "Error: creating EC2 Instance: InvalidAMIID.NotFound: The image id 'ami-1' does not exist"
	causes := map[string]string{
		"aws-insufficient-capacity": "Error: creating EC2 Instance: InsufficientInstanceCapacity: no capacity",
		"aws-throttled":             "Error: describing instances: RequestLimitExceeded: Request limit exceeded.",
		"aws-server-error":          "Error: creating VPC: InternalError: internal error, status code: 500",
		"jenkins-agent-lost":        "ERROR: hudson.remoting.ChannelClosedException: Remote call failed",
		"docker-pull-rate-limit":    "Error: toomanyrequests: You have reached your pull rate limit.",
	}
	for id, cause := range causes {
		f := AnalyzeFailure(provisioningLog(cause))
		if r := MatchTransient(&f); r == nil || r.ID != id {
			t.Fatalf("%s: matched %v on its own primary failure", id, r)
		}

		collateral := AnalyzeFailure(provisioningLog(persistent, "cleanup: "+cause))
		if r := MatchTransient(&collateral); r != nil {
			t.Fatalf("%s: granted from cleanup text after a persistent failure (%s)", id, r.ID)
		}

		inTest := AnalyzeFailure(strings.Replace(ginkgoLog("x", "v", 1), "local-path volume cleanup failed", cause, 1))
		if r := MatchTransient(&inTest); r != nil {
			t.Fatalf("%s: granted for a test failure", id)
		}
	}

	for name, cause := range map[string]string{
		"missing AMI":       persistent,
		"bad ssh key":       "fatal: [node-0]: UNREACHABLE! => {\"msg\": \"Permission denied (publickey)\"}",
		"quota":             "Error: creating EC2 Instance: VcpuLimitExceeded: You have requested more vCPU capacity",
		"aws client error":  "Error: creating VPC: InvalidParameterValue: bad CIDR, status code: 400",
		"unrelated timeout": "Error: timeout while waiting for state to become 'running'",
	} {
		f := AnalyzeFailure(provisioningLog(cause))
		if r := MatchTransient(&f); r != nil {
			t.Fatalf("%s: granted by %s", name, r.ID)
		}
	}
}

// The verdict agrees to a rerun only as a confident, evidenced INFRA diagnosis.
func TestVerdictRerunVeto(t *testing.T) {
	ok := Verdict{Action: "rerun", Bucket: "INFRA", Confidence: 90, Summary: "s", Evidence: []string{"e"}}
	if d := ok.Decision(); !d.Rerun {
		t.Fatalf("agreeing verdict vetoed: %+v", d)
	}
	for name, v := range map[string]Verdict{
		"product bucket": {Action: "rerun", Bucket: "PRODUCT", Confidence: 95, Summary: "s", Evidence: []string{"e"}},
		"low confidence": {Action: "rerun", Bucket: "INFRA", Confidence: 79, Summary: "s", Evidence: []string{"e"}},
		"no evidence":    {Action: "rerun", Bucket: "INFRA", Confidence: 95, Summary: "s"},
		"blank evidence": {Action: "rerun", Bucket: "INFRA", Confidence: 95, Summary: "s", Evidence: []string{"   ", ""}},
		"infra variant": {
			Action: "rerun", Bucket: "INFRA-UNVERIFIED", Confidence: 95, Summary: "s", Evidence: []string{"e"},
		},
		"lower case": {Action: "rerun", Bucket: "infra", Confidence: 95, Summary: "s", Evidence: []string{"e"}},
	} {
		if d := v.Decision(); d.Rerun || !strings.Contains(d.Summary, "no automatic rerun") {
			t.Fatalf("%s: %+v", name, d)
		}
	}
}

// Every alternative of every rule reaches the rule through extraction, followed by the trailers
// jenkins prints after a failure (which must not become the primary failure).
func TestTransientRulesThroughExtraction(t *testing.T) {
	trailers := []string{
		"ERROR: script returned exit code 1",
		"Also:   hudson.remoting.ProxyException: java.lang.IllegalStateException: step failed",
		"Finished: FAILURE",
	}
	for _, c := range []struct{ id, line string }{
		{"aws-insufficient-capacity", "Error: creating EC2 Instance: InsufficientInstanceCapacity: no capacity"},
		{"aws-throttled", "Error: describing instances: RequestLimitExceeded: Request limit exceeded."},
		{"aws-throttled", "Error: listing subnets: Throttling: Rate exceeded"},
		{"aws-server-error", "Error: creating VPC: InternalError: internal error, status code: 500"},
		{"aws-server-error", "Error: creating VPC: ServiceUnavailable: try again, status code: 503"},
		{"aws-server-error", "Error: reading EIP: Unavailable: backend, status code: 503"},
		{"jenkins-agent-lost", "hudson.remoting.ChannelClosedException: Channel \"hudson.remoting.Channel@5\""},
		{"jenkins-agent-lost", "Agent went offline during the build"},
		{"docker-pull-rate-limit", "Error: toomanyrequests: You have reached your pull rate limit."},
	} {
		head := []string{"[Pipeline] { (Run TestCombination)", "provisioning", c.line}
		log := strings.Join(append(head, trailers...), "\n")
		f := AnalyzeFailure(log)
		if r := MatchTransient(&f); r == nil || r.ID != c.id {
			t.Fatalf("%q: matched %v, want %s (primary %q)", c.line, r, c.id, f.Primary)
		}
	}

	// A trailer printed first (by a parallel branch) is not the primary failure either.
	f := AnalyzeFailure(strings.Join(append(trailers[:2:2], "Agent went offline during the build"), "\n"))
	if r := MatchTransient(&f); r == nil || r.ID != "jenkins-agent-lost" {
		t.Fatalf("trailer taken as the primary failure: %q", f.Primary)
	}
}

// With an agreeing INFRA verdict, a transient signal that is not the primary failure grants nothing:
// an ignored warning before a permanent error, or throttling in cleanup after a jenkins exception.
func TestTransientSignalsThatAreNotTheCause(t *testing.T) {
	agree := Decision{Rerun: true, Summary: "INFRA"}
	for name, log := range map[string][]string{
		"ignored capacity warning": {
			"[Pipeline] { (Run TestCombination)",
			"Warning: InsufficientInstanceCapacity in us-east-2a, ignoring and trying us-east-2b",
			"aws_instance.node[0]: Creation complete after 12s",
			"Error: creating EC2 Instance: InvalidAMIID.NotFound: The image id 'ami-1' does not exist",
			"Finished: FAILURE",
		},
		"cleanup throttling after a generic trailer": {
			"ERROR: script returned exit code 1",
			"[Pipeline] { (Cleanup)",
			"cleanup: Error: describing instances: RequestLimitExceeded: Request limit exceeded.",
			"Finished: FAILURE",
		},
		"throttling after a generic trailer, same stage": {
			"[Pipeline] { (Run TestCombination)",
			"ERROR: script returned exit code 1",
			"Error: describing instances: RequestLimitExceeded: Request limit exceeded.",
			"Finished: FAILURE",
		},
		"lost agent in cleanup after a generic trailer": {
			"[Pipeline] { (Run TestCombination)",
			"ERROR: script returned exit code 1",
			"[Pipeline] { (Cleanup)",
			"Agent went offline during the build",
			"Finished: FAILURE",
		},
		"jenkins exception before cleanup throttling": {
			"[Pipeline] { (Run TestCombination)",
			"ERROR: hudson.AbortException: Could not find credentials entry with ID 'aws-qa'",
			"[Pipeline] { (Cleanup)",
			"Error: describing instances: RequestLimitExceeded: Request limit exceeded.",
			"ERROR: script returned exit code 1",
			"Finished: FAILURE",
		},
	} {
		f := AnalyzeFailure(strings.Join(log, "\n"))
		if d := GateRerun(agree, &f); d.Rerun {
			t.Fatalf("%s: rerun granted on primary %q", name, f.Primary)
		}
	}
}
