package releasebot

import (
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
)

// Request is the set of release candidates asked for in a Slack message.
type Request struct {
	K3s  []string
	RKE2 []string
}

var (
	k3sRC  = regexp.MustCompile(`v\d+\.\d+\.\d+-rc\d+\+k3s\d+`)
	rke2RC = regexp.MustCompile(`v\d+\.\d+\.\d+-rc\d+\+rke2r\d+`)
)

// ParseRequest extracts the k3s and rke2 RC tags from free text.
func ParseRequest(text string) Request {
	text = normalizeSlack(text)

	return Request{
		K3s:  uniqueSorted(k3sRC.FindAllString(text, -1)),
		RKE2: uniqueSorted(rke2RC.FindAllString(text, -1)),
	}
}

// Without returns the request minus the given tags.
func (r Request) Without(drop []string) Request {
	keep := func(tags []string) []string {
		out := []string{}
		for _, t := range tags {
			if !slices.Contains(drop, t) {
				out = append(out, t)
			}
		}

		return out
	}

	return Request{K3s: keep(r.K3s), RKE2: keep(r.RKE2)}
}

// Empty reports whether no RC was found.
func (r Request) Empty() bool {
	return len(r.K3s) == 0 && len(r.RKE2) == 0
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

// release is a parsed k3s/rke2 tag; rc is 0 for a GA release.
type release struct{ major, minor, patch, rc, rev int }

var releaseTag = regexp.MustCompile(`^v(\d+)\.(\d+)\.(\d+)(?:-rc(\d+))?\+(?:k3s|rke2r)(\d+)$`)

func parseRelease(tag string) (release, bool) {
	m := releaseTag.FindStringSubmatch(tag)
	if m == nil {
		return release{}, false
	}
	n := make([]int, 5)
	for i, s := range m[1:] {
		n[i], _ = strconv.Atoi(s)
	}

	return release{major: n[0], minor: n[1], patch: n[2], rc: n[3], rev: n[4]}, true
}

// less orders releases the way they ship: v1.37.1-rc1+rke2r1 < v1.37.1+rke2r1 < v1.37.1-rc1+rke2r2.
func (a release) less(b release) bool {
	ka := []int{a.major, a.minor, a.patch, a.rev, boolInt(a.rc == 0), a.rc}
	kb := []int{b.major, b.minor, b.patch, b.rev, boolInt(b.rc == 0), b.rc}
	for i := range ka {
		if ka[i] != kb[i] {
			return ka[i] < kb[i]
		}
	}

	return false
}

func boolInt(b bool) int {
	if b {
		return 1
	}

	return 0
}

// newestFirst sorts release tags from the newest down; unparsable tags keep their order at the end.
func newestFirst(tags []string) []string {
	out := append([]string{}, tags...)
	sort.SliceStable(out, func(i, j int) bool {
		a, okA := parseRelease(out[i])
		b, okB := parseRelease(out[j])
		if !okA || !okB {
			return okA && !okB
		}

		return b.less(a)
	})

	return out
}

// newerTag reports whether tag a is a newer release than b (v1.37 before v1.9, v1.36.10 before
// v1.36.9); tags that do not parse go after those that do, in reverse string order.
func newerTag(a, b string) bool {
	ra, okA := parseRelease(a)
	rb, okB := parseRelease(b)
	if !okA || !okB {
		return okA || (!okB && a > b)
	}

	return rb.less(ra)
}

// baseRC turns "v1.37.1-rc2+rke2r1" into "v1.37.1-rc2", the format the Qase workflow expects.
func baseRC(tag string) string {
	base, _, _ := strings.Cut(tag, "+")

	return base
}

// baseVersion turns "v1.37.1-rc2+rke2r1" into "v1.37.1".
func baseVersion(tag string) string {
	version, _, _ := strings.Cut(baseRC(tag), "-rc")

	return version
}
