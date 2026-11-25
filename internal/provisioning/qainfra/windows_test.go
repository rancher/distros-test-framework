package qainfra

import (
	"strings"
	"testing"
)

func TestInjectWindowsModuleArgs(t *testing.T) {
	const tpl = "module \"x\" {\n  " + windowsMarker + "\n}\n"
	if got := injectWindowsModuleArgs(tpl, false); strings.Contains(got, "windows") {
		t.Errorf("non-Windows run must not get Windows args:\n%s", got)
	}
	if got := injectWindowsModuleArgs(tpl, true); !strings.Contains(got, "windows_enable_rdp") {
		t.Errorf("Windows run missing args:\n%s", got)
	}
}

func TestWindowsRequestedIgnoresCommentsAndStrings(t *testing.T) {
	const etcd = `nodes = [{ count = 1, role = ["etcd"] }]` + "\n"
	const win = `os = "windows"`
	for name, tc := range map[string]struct {
		content string
		want    bool
	}{
		"hash comment":  {"# example: " + win + "\n" + etcd, false},
		"block comment": {"/* " + win + " */\n" + etcd, false},
		"description":   {`description = "os = \"windows\""` + "\n" + etcd, false},
		"zero count":    {`nodes = [{ count = 0, role = ["worker"], ` + win + ` }]` + "\n", false},
		"zero then real": {
			`nodes = [{ count = 0, role = ["worker"], ` + win + ` }, { count = 2, role = ["worker"], ` + win + ` }]` + "\n",
			true,
		},
		"computed count":  {`nodes = [{ count = 0 + 1, role = ["worker"], ` + win + ` }]` + "\n", true},
		"variable count":  {`nodes = [{ count = var.n, role = ["worker"], ` + win + ` }]` + "\n", true},
		"quoted zero":     {`nodes = [{ count = "0", role = ["worker"], ` + win + ` }]` + "\n", false},
		"after nodes var": {etcd + "foo = { " + win + " }\n", false},
		"real node": {
			"nodes = [\n  { count = 1, role = [\"etcd\"] },\n" +
				"  { count = 1, role = [\"worker\"], " + win + " }, # win\n]\n",
			true,
		},
	} {
		t.Run(name, func(t *testing.T) {
			setNodeEnvs(t, nil)
			got, err := windowsRequested(writeTFVars(t, tc.content))
			if err != nil || got != tc.want {
				t.Fatalf("windowsRequested = %v, %v; want %v", got, err, tc.want)
			}
		})
	}
}
