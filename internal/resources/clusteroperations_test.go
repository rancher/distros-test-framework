package resources

import "testing"

func TestServerHostFromKubeconfig(t *testing.T) {
	cases := []struct {
		name    string
		content string
		want    string
		wantErr bool
	}{
		{
			name:    "ipv4",
			content: "clusters:\n- cluster:\n    server: https://1.2.3.4:6443\n",
			want:    "1.2.3.4",
		},
		{
			name:    "nlb dns name",
			content: "    server: https://dsf-x-nlb.elb.us-east-2.amazonaws.com:6443\n",
			want:    "dsf-x-nlb.elb.us-east-2.amazonaws.com",
		},
		{
			name:    "ipv6",
			content: "    server: https://[2600:1f16::1]:6443\n",
			want:    "2600:1f16::1",
		},
		{
			name:    "no server entry",
			content: "clusters: []\n",
			wantErr: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ServerHostFromKubeconfig(tc.content)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got %q", got)
				}

				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got != tc.want {
				t.Errorf("host = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestSSHTargetIP(t *testing.T) {
	cases := map[string]struct {
		ips  []string
		want string
	}{
		"ipv6 first picks ipv4": {[]string{"2600::1", "54.1.2.3", "2600::2", "54.1.2.4"}, "54.1.2.3"},
		"ipv4 first":            {[]string{"54.1.2.3", "2600::1"}, "54.1.2.3"},
		"ipv6 only falls back":  {[]string{"2600::1", "2600::2"}, "2600::1"},
		"empty":                 {nil, ""},
	}
	for name, tc := range cases {
		if got := SSHTargetIP(tc.ips); got != tc.want {
			t.Errorf("%s: SSHTargetIP(%v) = %q, want %q", name, tc.ips, got, tc.want)
		}
	}
}
