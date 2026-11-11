package support

import "testing"

func TestParseWindowsCaption(t *testing.T) {
	tests := []struct{ name, in, want string }{
		{
			"2025", "Microsoft Windows Server 2025 Datacenter\r\n",
			"Microsoft Windows Server 2025 Datacenter",
		},
		{
			"ssh warning first", "Warning: Permanently added host\nMicrosoft Windows Server 2022 Datacenter\n",
			"Microsoft Windows Server 2022 Datacenter",
		},
		{"empty", "", ""},
		{"injection", "Microsoft Windows $(id) \"; rm -rf /\n", ""},
		{"backtick", "Microsoft Windows `id`", ""},
		{"ssh error only", "ssh: connect to host 1.2.3.4 port 22: Connection timed out", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := parseWindowsCaption(tt.in); got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestWindowsInstallCmd(t *testing.T) {
	base := `powershell .\windows_install.ps1 "1.1.1.1" "` + token + `" "2.2.2.2" "tarball"`
	if got := windowsInstallCmd("1.1.1.1", "2.2.2.2", "tarball", ""); got != base {
		t.Fatalf("empty flags: got %q, want %q", got, base)
	}
	if got, want := windowsInstallCmd("1.1.1.1", "2.2.2.2", "tarball", "x: y"), base+` "x: y"`; got != want {
		t.Fatalf("with flags: got %q, want %q", got, want)
	}
}
