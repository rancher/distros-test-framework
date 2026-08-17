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
