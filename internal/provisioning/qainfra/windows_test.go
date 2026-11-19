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
