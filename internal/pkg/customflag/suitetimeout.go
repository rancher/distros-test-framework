package customflag

import (
	"flag"
	"time"
)

// FlagGiven reports whether name was set on the command line.
func FlagGiven(fs *flag.FlagSet, name string) bool {
	given := false
	fs.Visit(func(f *flag.Flag) { given = given || f.Name == name })

	return given
}

// SuiteTimeout keeps an explicit --ginkgo.timeout; otherwise the Ginkgo suite ends margin before the
// go test deadline, so Ginkgo interrupts and AfterSuite cleanup runs before go test kills the process.
func SuiteTimeout(current time.Duration, explicit bool, deadline time.Time, hasDeadline bool,
	now time.Time, margin time.Duration,
) time.Duration {
	remaining := deadline.Sub(now)
	if explicit || !hasDeadline || remaining <= 0 {
		return current
	}
	if remaining > 2*margin {
		return remaining - margin
	}

	return remaining / 2
}
