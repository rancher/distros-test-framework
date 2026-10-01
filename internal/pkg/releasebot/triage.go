package releasebot

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/rancher/distros-test-framework/internal/pkg/slack"
)

// Triage modes: quick decides rerun-or-ask for every failure (smaller model, no verifier, from
// the log excerpt); full is the complete skill with its verifier, run when a person asks.
const (
	TriageQuick = "quick"
	TriageFull  = "full"
)

// TriageRequest is one failed build to triage.
type TriageRequest struct {
	ID       string `json:"id"`
	Mode     string `json:"mode"`
	JobName  string `json:"job_name"`
	JobPath  string `json:"job_path"`
	Product  string `json:"product"`
	Version  string `json:"version"`
	Result   string `json:"result"`
	BuildURL string `json:"build_url"`
	Attempt  int    `json:"attempt"`
	// Excerpt is the failing part of the console log (see extractFailure), so the model starts
	// from it instead of paging through the whole log.
	Excerpt string `json:"excerpt,omitempty"`
}

// Verdict is the broker's answer. Error means triage could not run; the bot then asks a person.
type Verdict struct {
	Action     string   `json:"action"` // "rerun" or "ask"
	Bucket     string   `json:"bucket"`
	Confidence int      `json:"confidence"`
	Summary    string   `json:"summary"`
	Evidence   []string `json:"evidence,omitempty"`
	Error      string   `json:"error,omitempty"`
	CostUSD    float64  `json:"cost_usd,omitempty"`
	Seconds    int      `json:"seconds,omitempty"`
}

// Decision turns a verdict into the scheduler's decision; anything but a clean "rerun" asks.
func (v *Verdict) Decision() Decision {
	if v.Error != "" {
		return Decision{Summary: "automatic triage failed: " + slack.Escape(redactSecrets(v.Error))}
	}
	// The summary is the model's text, posted in Slack: redacted, and escaped so it cannot mention
	// people, channels or @here, nor hide a link.
	summary := fmt.Sprintf("%s, %d%% confidence: %s", v.Bucket, v.Confidence, v.Summary)
	d := Decision{Rerun: v.Action == "rerun", Summary: slack.Escape(redactSecrets(summary))}
	// The verdict only agrees to a rerun as an evidenced, confident INFRA diagnosis.
	if why := v.rerunVeto(); d.Rerun && why != "" {
		d.Rerun = false
		d.Summary += " (no automatic rerun: " + why + ")"
	}

	return d
}

// The skill's buckets; only BucketInfra can agree to a rerun.
const BucketInfra = "INFRA"

var buckets = []string{BucketInfra, "JOB", "TEST-CODE", "PRODUCT", "UNKNOWN"}

// MinRerunConfidence is the confidence (percent) below which a rerun verdict is not agreement.
const MinRerunConfidence = 80

// rerunVeto says why a rerun verdict does not count as agreement, or "".
func (v *Verdict) rerunVeto() string {
	switch {
	case v.Bucket != BucketInfra:
		return fmt.Sprintf("the diagnosis is %s, not %s", v.Bucket, BucketInfra)
	case v.Confidence < MinRerunConfidence:
		return fmt.Sprintf("confidence %d%% is below %d%%", v.Confidence, MinRerunConfidence)
	case !slices.ContainsFunc(v.Evidence, func(e string) bool { return strings.TrimSpace(e) != "" }):
		return "the verdict cites no evidence"
	}

	return ""
}

// GateRerun keeps a rerun only when the rule grants it: the primary failure (not a test failure)
// matches a transient-infrastructure rule. The verdict can only veto, never grant.
func GateRerun(d Decision, fail *Failure) Decision {
	if !d.Rerun {
		return d
	}
	if rule := MatchTransient(fail); rule != nil {
		d.Summary += fmt.Sprintf(" [rerun rule %s, rules %s]", rule.ID, TransientRulesVersion)
		return d
	}
	d.Rerun = false
	d.Summary += " (no automatic rerun: the failure matches no transient-infrastructure rule)"

	return d
}

// verdictSchema is what --json-schema enforces on the model's answer.
const verdictSchema = `{"type":"object","additionalProperties":false,` +
	`"required":["action","bucket","confidence","summary"],"properties":{` +
	`"action":{"enum":["rerun","ask"]},"bucket":{"enum":["INFRA","JOB","TEST-CODE","PRODUCT","UNKNOWN"]},` +
	`"confidence":{"type":"integer","minimum":0,"maximum":100},` +
	`"summary":{"type":"string","maxLength":1200},` +
	`"evidence":{"type":"array","maxItems":5,"items":{"type":"string","maxLength":400}}}}`

// parseClaudeVerdict reads claude's --output-format json envelope; the verdict is in
// structured_output (from --json-schema) or, failing that, in result as JSON text.
func parseClaudeVerdict(raw []byte) (*Verdict, error) {
	var env struct {
		IsError          bool            `json:"is_error"`
		Result           string          `json:"result"`
		StructuredOutput json.RawMessage `json:"structured_output"`
		CostUSD          float64         `json:"total_cost_usd"`
		DurationMS       int             `json:"duration_ms"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		return nil, fmt.Errorf("claude output is not JSON: %w", err)
	}
	if env.IsError {
		return nil, fmt.Errorf("claude reported an error: %s", lastLine(env.Result))
	}
	body := env.StructuredOutput
	if len(body) == 0 || string(body) == "null" {
		body = json.RawMessage(env.Result)
	}

	v, err := decodeVerdict(body)
	if err != nil {
		return nil, err
	}
	if v.Error != "" {
		return nil, errors.New("the model may not set error")
	}
	v.CostUSD, v.Seconds = env.CostUSD, env.DurationMS/1000

	return v, nil
}

// decodeVerdict parses one verdict object and checks the whole contract (see verdictSchema):
// unknown fields, trailing data, missing or out-of-range fields are all errors, which ask.
func decodeVerdict(raw []byte) (*Verdict, error) {
	// Pointers tell a missing or null field from a zero value.
	var w struct {
		Action     *string  `json:"action"`
		Bucket     *string  `json:"bucket"`
		Confidence *int     `json:"confidence"`
		Summary    *string  `json:"summary"`
		Evidence   []string `json:"evidence"`
		Error      *string  `json:"error"`
		CostUSD    float64  `json:"cost_usd"`
		Seconds    int      `json:"seconds"`
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&w); err != nil {
		return nil, fmt.Errorf("verdict does not match the schema: %w", err)
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return nil, errors.New("verdict does not match the schema: data after the object")
	}

	v := &Verdict{Evidence: w.Evidence, CostUSD: w.CostUSD, Seconds: w.Seconds}
	if w.Error != nil && *w.Error != "" {
		v.Error = *w.Error
		return v, nil
	}
	if w.Action == nil || w.Bucket == nil || w.Confidence == nil || w.Summary == nil {
		return nil, errors.New("verdict does not match the schema: action, bucket, confidence and summary are required")
	}
	v.Action, v.Bucket, v.Confidence, v.Summary = *w.Action, *w.Bucket, *w.Confidence, *w.Summary
	if err := v.validate(); err != nil {
		return nil, fmt.Errorf("verdict does not match the schema: %w", err)
	}

	return v, nil
}

// validate checks the fields verdictSchema requires; a verdict that only reports an error needs
// nothing else (it asks a person anyway).
func (v *Verdict) validate() error {
	if v.Error != "" {
		return nil
	}
	switch {
	case v.Action != "rerun" && v.Action != "ask":
		return fmt.Errorf("action %q", v.Action)
	case !slices.Contains(buckets, v.Bucket):
		return fmt.Errorf("bucket %q is not one of %v", v.Bucket, buckets)
	case v.Confidence < 0 || v.Confidence > 100:
		return fmt.Errorf("confidence %d outside 0-100", v.Confidence)
	case strings.TrimSpace(v.Summary) == "" || len(v.Summary) > 1200:
		return errors.New("summary missing or longer than 1200")
	case len(v.Evidence) > 5:
		return errors.New("more than 5 evidence items")
	}
	for _, e := range v.Evidence {
		switch {
		case strings.TrimSpace(e) == "":
			return errors.New("empty evidence item")
		case len(e) > 400:
			return errors.New("evidence item longer than 400")
		}
	}

	return nil
}

func lastLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.LastIndex(s, "\n"); i >= 0 {
		s = s[i+1:]
	}
	if len(s) > 300 {
		s = s[:300]
	}

	return s
}
