package customflag

import (
	"flag"
	"testing"
	"time"
)

func TestSuiteTimeout(t *testing.T) {
	now := time.Date(2026, 11, 27, 12, 0, 0, 0, time.UTC)
	margin := 10 * time.Minute
	cases := []struct {
		name        string
		explicit    bool
		hasDeadline bool
		remaining   time.Duration
		want        time.Duration
	}{
		{"go test 4h deadline replaces ginkgo's 1h default", false, true, 4 * time.Hour, 4*time.Hour - margin},
		{"explicit --ginkgo.timeout wins", true, true, 4 * time.Hour, time.Hour},
		{"no go test deadline keeps the default", false, false, 0, time.Hour},
		{"short deadline still ends before go test", false, true, 15 * time.Minute, 7*time.Minute + 30*time.Second},
		{"past deadline keeps the default", false, true, -time.Minute, time.Hour},
	}
	for _, c := range cases {
		got := SuiteTimeout(time.Hour, c.explicit, now.Add(c.remaining), c.hasDeadline, now, margin)
		if got != c.want {
			t.Errorf("%s: SuiteTimeout = %s, want %s", c.name, got, c.want)
		}
	}
}

func TestFlagGiven(t *testing.T) {
	fs := flag.NewFlagSet("t", flag.ContinueOnError)
	fs.Duration("ginkgo.timeout", time.Hour, "")
	fs.Bool("destroy", false, "")
	if err := fs.Parse([]string{"-ginkgo.timeout=3h"}); err != nil {
		t.Fatal(err)
	}
	if !FlagGiven(fs, "ginkgo.timeout") || FlagGiven(fs, "destroy") || FlagGiven(fs, "missing") {
		t.Fatal("FlagGiven must report only flags set on the command line")
	}
}
