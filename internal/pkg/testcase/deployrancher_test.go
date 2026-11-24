package testcase

import (
	"reflect"
	"testing"

	"github.com/rancher/distros-test-framework/internal/pkg/customflag"
)

func TestChartsArgsList(t *testing.T) {
	mk := func(args string) *customflag.FlagConfig {
		f := &customflag.FlagConfig{}
		f.Charts.Args = args
		return f
	}
	cases := []struct {
		name string
		in   string
		want []string
	}{
		{"empty", "", nil},
		{"single", "a=1", []string{"--set", "a=1"}},
		{"multi with dedupe and trim", "a=1, b=2,a=1,,", []string{"--set", "a=1", "--set", "b=2"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := chartsArgsList(mk(tc.in)); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("chartsArgsList(%q) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}
