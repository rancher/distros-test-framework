package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSetEnvKeepsQuotesInsideFlags(t *testing.T) {
	const (
		cidr    = `cluster-cidr: 2001:cafe:42::/56\n`
		nodeIP  = `{{ ansible_host_ipv6 }},{{ ansible_facts.default_ipv4.address }}`
		want    = cidr + `node-ip: "` + nodeIP + `"`
		escaped = cidr + `node-ip: \"` + nodeIP + `\"`
	)
	cases := []struct {
		name, line, key, want string
	}{
		// Jenkinsfile flagFrom copies the HCL body (with \") into the env file unquoted.
		{"jenkins env line", `SERVER_FLAGS=` + escaped, "SERVER_FLAGS", want},
		{"tfvars line", `server_flags = "` + escaped + `"`, "server_flags", want},
		{"plain quoted value", `AWS_REGION="us-west-1"`, "AWS_REGION", "us-west-1"},
		{"unquoted value", `NODE_OS=ubuntu`, "NODE_OS", "ubuntu"},
		{"empty quoted value", `SSH_USER=""`, "SSH_USER", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "test.env")
			if err := os.WriteFile(path, []byte(c.line+"\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			t.Setenv(c.key, "")
			if err := setEnv(path); err != nil {
				t.Fatal(err)
			}
			if got := os.Getenv(c.key); got != c.want {
				t.Errorf("got  %s\nwant %s", got, c.want)
			}
		})
	}
}
