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
	clearRuntimeEnvs(t)
	for name, tc := range map[string]struct {
		content string
		want    bool
	}{
		"hash comment":    {"# example: os = \"windows\"\nnodes = [{ count = 1, role = [\"etcd\"] }]\n", false},
		"block comment":   {"/* os = \"windows\" */\nnodes = [{ count = 1, role = [\"etcd\"] }]\n", false},
		"other variable":  {"description = \"os = \\\"windows\\\"\"\nnodes = [{ count = 1, role = [\"etcd\"] }]\n", false},
		"real node":       {"nodes = [\n  { count = 1, role = [\"etcd\"] },\n  { count = 1, role = [\"worker\"], os = \"windows\" }, # win\n]\n", true},
		"after nodes var": {"nodes = [{ count = 1, role = [\"etcd\"] }]\nfoo = { os = \"windows\" }\n", false},
	} {
		t.Run(name, func(t *testing.T) {
			path := writeTFVars(t, tc.content)
			got, err := windowsRequested(path)
			if err != nil || got != tc.want {
				t.Fatalf("windowsRequested = %v, %v; want %v", got, err, tc.want)
			}
		})
	}
}
