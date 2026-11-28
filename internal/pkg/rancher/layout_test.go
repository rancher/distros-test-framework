package rancher

import (
	"reflect"
	"testing"
)

func TestParseLayout(t *testing.T) {
	cases := []struct {
		spec string
		want [][]string
	}{
		{DefaultCustomLayout, [][]string{
			{"etcd"}, {"etcd"}, {"etcd"}, {"controlplane"}, {"controlplane"}, {"worker"},
		}},
		{"worker+CP+etcd:3", [][]string{
			{"etcd", "controlplane", "worker"}, {"etcd", "controlplane", "worker"}, {"etcd", "controlplane", "worker"},
		}},
		{DefaultImportLayout, [][]string{
			{"etcd", "controlplane"}, {"etcd", "controlplane"}, {"etcd", "controlplane"}, {"worker"},
		}},
	}
	for _, c := range cases {
		got, err := ParseLayout(c.spec)
		if err != nil || !reflect.DeepEqual(got, c.want) {
			t.Errorf("ParseLayout(%q) = %v, %v; want %v", c.spec, got, err, c.want)
		}
	}

	for _, bad := range []string{"", "etcd", "etcd:0", "etcd:-1", "etcd:x", "master:1", "etcd:3,,worker:1", "etcd:11"} {
		if _, err := ParseLayout(bad); err == nil {
			t.Errorf("ParseLayout(%q) succeeded, want error", bad)
		}
	}
}

func TestValidateLayouts(t *testing.T) {
	mustParse := func(s string) [][]string {
		n, err := ParseLayout(s)
		if err != nil {
			t.Fatal(err)
		}

		return n
	}

	for spec, ok := range map[string]bool{
		DefaultCustomLayout:            true,
		"etcd+controlplane+worker:3":   true,
		"etcd+controlplane:1":          false, // no worker
		"controlplane:1,worker:1":      false, // no etcd
		"etcd:1,worker+controlplane:1": true,
	} {
		if err := ValidateCustomLayout(mustParse(spec)); (err == nil) != ok {
			t.Errorf("ValidateCustomLayout(%q) err=%v, want ok=%v", spec, err, ok)
		}
	}

	for spec, ok := range map[string]bool{
		DefaultImportLayout:          true,
		"etcd+cp:1":                  true,
		"worker:1,etcd+cp:3":         false, // first node must be a server
		"etcd:3,worker:1":            false, // standalone can't make etcd-only nodes here
		"etcd+controlplane+worker:3": false,
	} {
		if err := ValidateImportLayout(mustParse(spec)); (err == nil) != ok {
			t.Errorf("ValidateImportLayout(%q) err=%v, want ok=%v", spec, err, ok)
		}
	}
}
