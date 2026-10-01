package releasebot

import (
	"crypto/sha256"
	"encoding/hex"
	"regexp"
	"sort"
	"strings"
)

// A failed build's log gives triage a short excerpt and a signature; the same failure on other
// RCs or products has the same signature, so it is triaged once.
var (
	ansiCode    = regexp.MustCompile(`\x1b\[[0-9;]*m`)
	logStamp    = regexp.MustCompile(`^\[\d{4}-\d\d-\d\dT[0-9:.]+Z\] ?`)
	stageMarker = regexp.MustCompile(`^\[Pipeline\] \{ \((.+)\)$`)
	errorish    = regexp.MustCompile(`(?i)\b(error|fatal|panic|failed|failure|timed out|timeout)\b`)
	notError    = regexp.MustCompile(`(?i)\bfailed=0\b|\berrors?: 0\b|ignoring|\bno errors?\b`)
	// A timeout setting is not a timeout: Jenkins' "# timeout=10" on every git step, go test's
	// -timeout=100m. It is removed before a line is checked, so a real error on it still counts.
	timeoutSetting = regexp.MustCompile(`(?i)(?:#\s*|-{1,2}|\b)timeout\s*[=:]\s*\S+|-{1,2}timeout\s+\S+`)

	// Normalizing removes only what differs between runs of the same failure (ids, addresses,
	// dates, durations); other numbers (HTTP codes, exit statuses, expected values) stay.
	volatileID = regexp.MustCompile(
		`[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}|\b0x[0-9a-f]+\b|\b[0-9a-f]{7,}\b`)
	ipAddr    = regexp.MustCompile(`\b\d{1,3}(\.\d{1,3}){3}(:\d+)?\b`)
	timestamp = regexp.MustCompile(`\b\d{2,4}[-/]\d\d[-/]\d{2,4}\b|\b\d\d:\d\d(:\d\d)?(\.\d+)?\b`)
	duration  = regexp.MustCompile(
		`\b\d+(\.\d+)?\s?(ms|s|m|h|seconds?|minutes?|hours?)\b|\b\d+h\d+m\d+s\b|\b\d+m\d+(\.\d+)?s\b`)
	spaceRun = regexp.MustCompile(`\s+`)

	// Only Jenkins' known failure trailers (and stack frames) are generic: they say nothing about
	// the cause. Exceptions with their own message (hudson.AbortException: Could not find...) are causes.
	genericLine = regexp.MustCompile(`^Finished: |^\[Pipeline\]|^Also:\s|` +
		`^(ERROR: )?([\w.$]+Exception: )*script returned exit code|^at (hudson|org\.jenkinsci|java|jdk|groovy)\.`)
)

const (
	maxExcerpt     = 12000
	maxBlockLines  = 25
	contextLines   = 5
	tailLines      = 40
	maxErrorBlocks = 5
	reasonLines    = 12
)

type Failure struct {
	Excerpt    string
	Signature  string
	TestFailed bool
	Primary    string
}

var provisionError = regexp.MustCompile(`error provisioning infrastructure`)

func extractFailure(log string) (excerpt, signature string) {
	f := AnalyzeFailure(log)

	return f.Excerpt, f.Signature
}

func AnalyzeFailure(log string) Failure {
	lines := cleanLines(log)
	failed, summary := ginkgoFailures(lines)
	var primary []string

	var keys []string
	var blocks [][]string
	stage := ""
	if len(failed) > 0 {
		stage = stageBefore(lines, failed[0])
		primary = block(lines, failed[0], maxBlockLines)
		for _, i := range failed {
			blocks = append(blocks, block(lines, i, maxBlockLines))
			keys = append(keys, failureReason(lines, i))
		}
		for _, i := range summary {
			keys = append(keys, lines[i])
		}
	} else {
		errs := firstErrors(lines, maxErrorBlocks)
		if len(errs) > 0 {
			first := primaryError(lines, errs)
			stage = stageBefore(lines, first)
			primary = errorBlock(lines, first)
		}
		for i, l := range lines {
			if provisionError.MatchString(l) {
				primary = append(primary, lines[i])
				break
			}
		}
		for _, i := range errs {
			blocks = append(blocks, window(lines, i-contextLines, i+contextLines+1))
			if !genericLine.MatchString(strings.TrimSpace(lines[i])) {
				keys = append(keys, lines[i])
			}
		}
	}

	excerpt := excerptOf(stage, lines, summary, blocks)
	f := Failure{Excerpt: excerpt, TestFailed: len(failed) > 0, Primary: strings.Join(primary, "\n")}
	if len(keys) > 0 {
		f.Signature = signatureOf(stage, keys)
	}

	return f
}

func excerptOf(stage string, lines []string, summary []int, blocks [][]string) string {
	var b strings.Builder
	if stage != "" {
		b.WriteString("Stage: " + stage + "\n")
	}
	for _, i := range summary {
		b.WriteString("Failed: " + strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(lines[i]), "[FAIL]")) + "\n")
	}
	for _, bl := range blocks {
		b.WriteString(strings.Join(bl, "\n") + "\n---\n")
	}
	b.WriteString("Last lines:\n" + strings.Join(window(lines, len(lines)-tailLines, len(lines)), "\n"))

	// Redacted here, once, so the spool file, the prompt and -triage-build carry the same text.
	excerpt := redactSecrets(b.String())
	if len(excerpt) > maxExcerpt {
		excerpt = excerpt[:maxExcerpt] + "\n[excerpt truncated]"
	}

	return excerpt
}

func cleanLines(log string) []string {
	raw := strings.Split(strings.ReplaceAll(log, "\r", ""), "\n")
	out := make([]string, 0, len(raw))
	for _, l := range raw {
		out = append(out, strings.TrimRight(logStamp.ReplaceAllString(ansiCode.ReplaceAllString(l, ""), ""), " "))
	}

	return out
}

func ginkgoFailures(lines []string) (failed, summary []int) {
	inSummary := false
	for i, l := range lines {
		t := strings.TrimSpace(l)
		switch {
		case strings.HasPrefix(t, "Summarizing "):
			inSummary = true
		case inSummary && strings.HasPrefix(t, "[FAIL] "):
			summary = append(summary, i)
		case inSummary && t == "":
			inSummary = false
		case strings.HasPrefix(t, "[FAILED] "):
			failed = append(failed, i)
		}
	}

	return failed, summary
}

func failureReason(lines []string, i int) string {
	parts := []string{strings.TrimPrefix(strings.TrimSpace(lines[i]), "[FAILED] ")}
	for _, l := range lines[i+1 : min(i+reasonLines, len(lines))] {
		t := strings.TrimSpace(l)
		if strings.HasPrefix(t, "In [") || strings.HasPrefix(t, "-----") {
			break
		}
		if t != "" {
			parts = append(parts, t)
		}
	}

	return strings.Join(parts, " ")
}

func errorBlock(lines []string, i int) []string {
	end := i + 1
	for end < len(lines) && end < i+maxBlockLines && strings.HasPrefix(lines[end], " ") {
		end++
	}

	return lines[i:end]
}

// firstErrors returns the first n error-like or rule-matching lines ("Agent went offline" has no
// error word), skipping lines the log marks as not an error (ignoring, failed=0).
func firstErrors(lines []string, n int) []int {
	var out []int
	for i, l := range lines {
		checked := timeoutSetting.ReplaceAllString(l, "")
		signal := matchesTransientLine(l) || errorish.MatchString(checked)
		if signal && !notError.MatchString(checked) && !strings.HasPrefix(l, "[Pipeline]") {
			out = append(out, i)
			if len(out) == n {
				break
			}
		}
	}

	return out
}

// primaryError is the first meaningful failure line in the stage that failed first; past a generic
// trailer only a lost agent is promoted, and never from a later stage (cleanup, post actions).
func primaryError(lines []string, errs []int) int {
	stage := stageBefore(lines, errs[0])
	for n, i := range errs {
		l := strings.TrimSpace(lines[i])
		transient := matchesTransientLine(l)
		switch {
		case stageBefore(lines, i) != stage:
			return errs[0]
		case genericLine.MatchString(l) && !transient:
			continue
		case n > 0 && transient && !matchesRule(agentLostRule, l):
			return errs[0]
		}

		return i
	}

	return errs[0]
}

func stageBefore(lines []string, i int) string {
	for j := i; j >= 0; j-- {
		if m := stageMarker.FindStringSubmatch(strings.TrimSpace(lines[j])); m != nil {
			return m[1]
		}
	}

	return ""
}

// block is the failure starting at i, up to the separator Ginkgo prints after it.
func block(lines []string, i, maxLines int) []string {
	end := min(i+maxLines, len(lines))
	for j := i + 1; j < end; j++ {
		if strings.HasPrefix(strings.TrimSpace(lines[j]), "-----") {
			end = j
			break
		}
	}

	return lines[i:end]
}

func window(lines []string, from, to int) []string {
	return lines[max(from, 0):min(to, len(lines))]
}

func signatureOf(stage string, keys []string) string {
	norm := make([]string, 0, len(keys))
	for _, k := range keys {
		norm = append(norm, normalize(k))
	}
	sort.Strings(norm)
	sum := sha256.Sum256([]byte(normalize(stage) + "\n" + strings.Join(norm, "\n")))

	return hex.EncodeToString(sum[:8])
}

func normalize(s string) string {
	s = strings.ToLower(s)
	s = timestamp.ReplaceAllString(s, "<time>")
	s = volatileID.ReplaceAllString(s, "<id>")
	s = ipAddr.ReplaceAllString(s, "<ip>")
	s = duration.ReplaceAllString(s, "<duration>")
	s = strings.TrimSpace(spaceRun.ReplaceAllString(s, " "))
	if len(s) > 200 {
		s = s[:200]
	}

	return s
}
