package resources

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestShellPathQuoting(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"plain path", "/etc/rancher/rke2/config.yaml", "'/etc/rancher/rke2/config.yaml'"},
		{"home relative keeps tilde expandable", "~/scripts/run.sh", `"$HOME"/'scripts/run.sh'`},
		{"spaces", "/tmp/my file.txt", "'/tmp/my file.txt'"},
		{"apostrophe", "/tmp/it's.txt", `'/tmp/it'"'"'s.txt'`},
		{"command substitution stays literal", "/tmp/$(rm -rf /x)", "'/tmp/$(rm -rf /x)'"},
		{"semicolon and ampersand", "/tmp/a;b&&c", "'/tmp/a;b&&c'"},
		{"backticks", "/tmp/`id`.txt", "'/tmp/`id`.txt'"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := shellPath(tc.in); got != tc.want {
				t.Errorf("shellPath(%q) = %s, want %s", tc.in, got, tc.want)
			}
		})
	}
}

// TestShellPathThroughShell round-trips each quoted path through a real shell:
// the output must be the literal path (with ~/ expanded to $HOME) and nothing
// may execute — this is the exact surface the remote `sudo tee` fallback uses.
func TestShellPathThroughShell(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh not available")
	}

	cases := []struct {
		in   string
		want string
	}{
		{"/etc/rancher/rke2/config.yaml", "/etc/rancher/rke2/config.yaml"},
		{"/tmp/my file.txt", "/tmp/my file.txt"},
		{"/tmp/it's.txt", "/tmp/it's.txt"},
		{"/tmp/$(echo pwned)", "/tmp/$(echo pwned)"},
		{"/tmp/`echo pwned`", "/tmp/`echo pwned`"},
		{"~/scripts/run.sh", os.Getenv("HOME") + "/scripts/run.sh"},
	}
	for _, tc := range cases {
		//nolint:gosec // quoting the untrusted input is exactly what is under test
		out, err := exec.Command("sh", "-c", "printf %s "+shellPath(tc.in)).Output()
		if err != nil {
			t.Fatalf("shell rejected quoting of %q: %v", tc.in, err)
		}
		if got := string(out); got != tc.want {
			t.Errorf("shell round-trip of %q = %q, want %q", tc.in, got, tc.want)
		}
		if strings.Contains(string(out), "pwned\n") {
			t.Fatalf("substitution executed for %q", tc.in)
		}
	}
}
