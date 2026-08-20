package customflag

import "testing"

func TestValidateChartsValues(t *testing.T) {
	cases := []struct {
		name, repo, url, args string
		wantErr               bool
	}{
		{"all empty", "", "", "", false},
		{
			"valid", "rancher-latest", "https://releases.rancher.com/server-charts/latest",
			"bootstrapPassword=admin,replicas=1", false,
		},
		{"name with shell meta", "r;rm -rf /", "", "", true},
		{"name leading dash", "-oProxy", "", "", true},
		{"url no scheme", "", "releases.rancher.com/charts", "", true},
		{"url ftp", "", "ftp://x/charts", "", true},
		{"url no host", "", "http://", "", true},
		{"url shell meta in host", "", "http://;id", "", true},
		{"args bare word", "", "", "abc", true},
		{"args only commas", "", "", ",,,", true},
		{"args url value is legit", "", "", "systemUrl=https://x/y", false},
		{"args password with specials is legit", "", "", "bootstrapPassword=p@ss!w0rd#x=y", false},
		{"args value with comma stays invalid", "", "", "key=a,b", true},
		{"args key with shell meta", "", "", "k;ey=x", true},
		{"args nested key", "", "", "global.cattle.psp.enabled=false", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateChartsValues(tc.repo, tc.url, tc.args)
			if (err != nil) != tc.wantErr {
				t.Errorf("validateChartsValues(%q,%q,%q) err=%v, wantErr=%v", tc.repo, tc.url, tc.args, err, tc.wantErr)
			}
		})
	}
}

func TestValidateNvidiaVersion(t *testing.T) {
	for v, wantErr := range map[string]bool{
		"": false, "580.159.03": false, "570.133": false,
		"580.159.03; rm -rf /": true, "v580.1": true, "$(id)": true,
	} {
		if err := validateNvidiaVersion(v); (err != nil) != wantErr {
			t.Errorf("validateNvidiaVersion(%q) err=%v, wantErr=%v", v, err, wantErr)
		}
	}
}
