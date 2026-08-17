package qainfra

import (
	"reflect"
	"testing"
)

func TestFormatServerFlagsToDict(t *testing.T) {
	cases := []struct {
		name  string
		flags string
		want  map[string]any
	}{
		{
			name:  "bare booleans become typed",
			flags: "selinux: true\nenable-servicelb: false",
			want:  map[string]any{"selinux": true, "enable-servicelb": false},
		},
		{
			name: "quoted values always stay strings",
			// quoted "true" and a leading-zero token must NOT be retyped
			flags: `selinux: "true"` + "\n" + `token: "00123"` + "\n" + `agent-token: '0xff'`,
			want:  map[string]any{"selinux": "true", "token": "00123", "agent-token": "0xff"},
		},
		{
			name:  "bare numbers stay strings",
			flags: "etcd-snapshot-retention: 5",
			want:  map[string]any{"etcd-snapshot-retention": "5"},
		},
		{
			name:  "plain strings and values with colons",
			flags: "profile: cis\ncni: multus,calico\ndatastore-endpoint: mysql://u:p@tcp(h:3306)/db",
			want: map[string]any{
				"profile": "cis", "cni": "multus,calico",
				"datastore-endpoint": "mysql://u:p@tcp(h:3306)/db",
			},
		},
		{
			name:  "comments, blanks and colon-less lines are skipped",
			flags: "# comment\n\nnot-a-pair\nselinux: true",
			want:  map[string]any{"selinux": true},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := formatServerFlagsToDict(tc.flags)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("formatServerFlagsToDict(%q) = %#v, want %#v", tc.flags, got, tc.want)
			}
		})
	}
}
