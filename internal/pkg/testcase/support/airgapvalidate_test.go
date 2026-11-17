package support

import (
	"strings"
	"testing"
)

func TestAssertEgressBlocked(t *testing.T) {
	cases := []struct {
		out     string
		blocked bool
	}{
		{"140.82.112.3 rc=28\n1.1.1.1 rc=28", true},
		{"140.82.112.3 rc=28 curl: (28) Connection timed out\n1.1.1.1 rc=28", true},
		{"140.82.112.3 rc=7 curl: (7) Network is unreachable\n1.1.1.1 rc=7 curl: (7) No route to host", true},
		// a host answered
		{"140.82.112.3 rc=7 curl: (7) Failed to connect: Connection refused\n1.1.1.1 rc=28", false},
		// reached github
		{"140.82.112.3 rc=0\n1.1.1.1 rc=28", false},
		// TLS handshake = connectivity
		{"140.82.112.3 rc=60\n1.1.1.1 rc=28", false},
		// curl missing: never a pass
		{"140.82.112.3 rc=127", false},
		{"", false},
	}
	for _, tc := range cases {
		err := assertEgressBlocked(tc.out)
		if tc.blocked && err != nil {
			t.Errorf("%q: unexpected error %v", tc.out, err)
		}
		if !tc.blocked && err == nil {
			t.Errorf("%q: expected an error", tc.out)
		}
	}
}

func TestK3sCrictlCommandsUseTheAbsoluteBinaryPath(t *testing.T) {
	cmds := map[string]string{
		"images": crictlImagesCmd("k3s"),
		"pull":   crictlPullCmd("k3s", "registry.example/rancher/pause:3.6"),
	}
	for name, cmd := range cmds {
		if !strings.HasPrefix(cmd, "sudo /usr/local/bin/k3s crictl ") {
			t.Errorf("%s: %q must call k3s by absolute path, SLES sudo secure_path omits /usr/local/bin", name, cmd)
		}
	}
}
