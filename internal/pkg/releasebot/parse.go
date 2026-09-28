package releasebot

import (
	"regexp"
	"sort"
	"strings"
)

// Request is the set of release candidates asked for in a Slack message.
type Request struct {
	K3s     []string
	RKE2    []string
	RKE2LTS []string
}

var (
	k3sRC  = regexp.MustCompile(`v\d+\.\d+\.\d+-rc\d+\+k3s\d+`)
	rke2RC = regexp.MustCompile(`v\d+\.\d+\.\d+-rc\d+\+rke2r\d+`)
	ltsKey = regexp.MustCompile(`(?i)\blts\s*=\s*(\S+)`)
)

// ParseRequest extracts RC tags from free text; tags after "lts=" are RKE2 LTS prime-registry versions.
func ParseRequest(text string) Request {
	text = normalizeSlack(text)

	var lts []string
	for _, m := range ltsKey.FindAllStringSubmatch(text, -1) {
		lts = append(lts, rke2RC.FindAllString(m[1], -1)...)
		text = strings.Replace(text, m[0], " ", 1)
	}

	return Request{
		K3s:     uniqueSorted(k3sRC.FindAllString(text, -1)),
		RKE2:    uniqueSorted(rke2RC.FindAllString(text, -1)),
		RKE2LTS: uniqueSorted(lts),
	}
}

// Empty reports whether no RC was found.
func (r Request) Empty() bool {
	return len(r.K3s) == 0 && len(r.RKE2) == 0 && len(r.RKE2LTS) == 0
}

// normalizeSlack undoes Slack's escaping so tags inside links and code spans still match.
func normalizeSlack(s string) string {
	r := strings.NewReplacer("&lt;", "<", "&gt;", ">", "&amp;", "&", "`", " ", "%2B", "+", "%2b", "+")

	return r.Replace(s)
}

func uniqueSorted(in []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(in))
	for _, v := range in {
		if !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	sort.Strings(out)

	return out
}

// BaseRC turns "v1.37.1-rc2+rke2r1" into "v1.37.1-rc2", the format the Qase workflow expects.
func BaseRC(tag string) string {
	base, _, _ := strings.Cut(tag, "+")

	return base
}

// BaseVersion turns "v1.37.1-rc2+rke2r1" into "v1.37.1".
func BaseVersion(tag string) string {
	version, _, _ := strings.Cut(BaseRC(tag), "-rc")

	return version
}
