package releasebot

import (
	"regexp"
	"strings"
)

// A log could make triage repeat a credential. From a credential label on, the rest of its line
// goes; only lines known to be safe stay. Long tokens go too, except readable names (paths, URLs).
var (
	secretLike   = regexp.MustCompile(`[A-Za-z0-9+/_\-=.]{32,}`)
	readableName = regexp.MustCompile(`^[/._-]*[a-z0-9]{1,24}([/._-]+[a-z0-9]{1,24})*[/._-]*$`)
	// A name ending a word or _/- part with a credential word (dbpassword, password_hash, SLACK_TOKEN,
	// "api key"), plus a spaced descriptor ("key ID"); a word going on after it (SecretsEncryption) is not.
	credentialLabel = regexp.MustCompile(`(?i)\b[a-z0-9_-]*?(?:authorization|token|password|passwd|` +
		`secret|api[-_ ]?key|access[-_ ]?key|credential)s?(?:[-_]+[a-z0-9]+)*(?: (?:id|value|header|string|data))?\b`)
	// A camelCase name with a credential word followed by more words (clientSecretKey, apiKeyValue);
	// a Go test name (TestSecretsEncryption) is skipped only when prose follows it, not a value.
	camelLabel = regexp.MustCompile(`\b([A-Za-z0-9_]*?(?i:authorization|token|passw(?:or)?d|secret|apikey|` +
		`accesskey|credential)(?i:s)?(?:[A-Z0-9][A-Za-z0-9]*)+)\b`)
	goTestName     = regexp.MustCompile(`^Test[A-Z0-9_]`)
	labelSeparator = regexp.MustCompile(`^["']?\s*(?:=>|->|[:=])?\s*`)
	// Kubernetes names a Secret, not its content: `secret "rke2-serving" not found`.
	k8sSecretStatus = regexp.MustCompile(`(?i)\bsecrets?\s+"[^"\n]*"\s+(?:not found|created|configured|unchanged|` +
		`deleted|already exists|is forbidden|is invalid)\b`)
)

// proseAfter: a line whose text after the label is only these words stays ("the token expired").
// A trade-off for readability, not a guarantee: a credential that is exactly such a word would show.
var proseAfter = map[string]bool{}

func init() {
	for _, w := range strings.Fields(`a aftereach aftersuite an and are as at be beforeeach beforesuite been ` +
		`by can cannot could did does empty out panicked skipped spec specs suite timed ` +
		`encryption error expired failed for found from has had have if in invalid is may missing must not ` +
		`of ok on or passed provided refresh rejected required rotated rotation should succeeded test ` +
		`that the this to was were when while will with without`) {
		proseAfter[w] = true
	}
}

func redactSecrets(s string) string {
	lines := strings.Split(s, "\n")
	for i := range lines {
		lines[i] = redactLine(lines[i])
	}

	return secretLike.ReplaceAllStringFunc(strings.Join(lines, "\n"), func(m string) string {
		if readableName.MatchString(m) {
			return m
		}

		return "[redacted]"
	})
}

// redactLine keeps a line up to its first credential label (and separator) and redacts the rest,
// unless that rest is only prose; a Kubernetes Secret status is kept and the text around it checked.
func redactLine(l string) string {
	loc := firstLabel(l)
	if loc == nil {
		return l
	}
	// The status is kept only when no label comes before it (its own "secret" starts it); after a
	// label, everything goes, a status included.
	if k := k8sSecretStatus.FindStringIndex(l); len(k) == 2 && k[0] <= loc[0] {
		return redactLine(l[:k[0]]) + l[k[0]:k[1]] + redactLine(l[k[1]:])
	}
	rest := l[loc[1]:]
	if onlyProse(rest) {
		return l
	}

	return l[:loc[1]] + labelSeparator.FindString(rest) + "[redacted]"
}

// firstLabel is where the line's first credential label starts and ends, or nil.
func firstLabel(l string) []int {
	loc := credentialLabel.FindStringIndex(l)
	for _, m := range camelLabel.FindAllStringSubmatchIndex(l, -1) {
		if goTestName.MatchString(l[m[2]:m[3]]) && testNameInProse(l[m[3]:]) {
			continue
		}
		if len(loc) < 2 || m[2] < loc[0] {
			return m[2:4]
		}

		break
	}

	return loc
}

// testNameInProse: a test result follows a test name ("TestX: failed in BeforeSuite"), or nothing
// does. An assignment, or any other first word ("TestKey: invalid"), is a value.
func testNameInProse(rest string) bool {
	sep := labelSeparator.FindString(rest)
	if strings.ContainsAny(sep, `=>"'`) {
		return false
	}
	words := strings.Fields(rest[len(sep):])

	return len(words) == 0 || testResults[strings.ToLower(strings.Trim(words[0], ".,;!?()"))] &&
		onlyProse(rest[len(sep):])
}

// testResults are the words a test's result starts with in Go and Ginkgo output.
var testResults = map[string]bool{
	"failed": true, "passed": true, "succeeded": true, "skipped": true, "panicked": true, "timed": true,
}

// onlyProse: every word a prose word (a separator or quote is never one, so "token: x" never is).
func onlyProse(rest string) bool {
	for _, w := range strings.Fields(rest) {
		if !proseAfter[strings.ToLower(strings.Trim(w, ".,;!?()"))] {
			return false
		}
	}

	return true
}
