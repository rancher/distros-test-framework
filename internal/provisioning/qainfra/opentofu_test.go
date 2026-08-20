package qainfra

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rancher/distros-test-framework/internal/provisioning/driver"
)

func writeTFVars(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "vars.tfvars")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	return path
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	return string(data)
}

// clearRuntimeEnvs blanks every env threadRuntimeEnvIntoTFVars and the
// external-DB gate read, so host leftovers never leak into a case.
func clearRuntimeEnvs(t *testing.T) {
	t.Helper()
	for _, k := range []string{
		"AWS_AMI", "SSH_USER", "INSTANCE_TYPE", "EC2_INSTANCE_CLASS",
		"VOLUME_SIZE", "VOLUME_TYPE", "AWS_REGION",
		"DATASTORE_TYPE", "datastore_type", "EXTERNAL_DB_ENDPOINT", "rendered_template",
		"SERVER_FLAGS", "server_flags", "EXTERNAL_DB", "external_db",
		"EXTERNAL_DB_VERSION", "external_db_version", "DB_GROUP_NAME", "db_group_name",
		"EXTERNAL_DB_NODE_TYPE", "instance_class", "DB_USERNAME", "db_username",
		"DB_PASSWORD", "db_password", "ENGINE_MODE", "engine_mode",
		"EXTERNAL_DB_SUBNET_IDS", "external_db_subnet_ids",
	} {
		t.Setenv(k, "")
	}
}

func TestSetOrAppendTFVarReplacesWithoutDuplicating(t *testing.T) {
	path := writeTFVars(t, "aws_ami = \"ami-old\"\nregion = \"us-east-2\"\n")

	if err := setOrAppendTFVar(path, "aws_ami", "ami-new"); err != nil {
		t.Fatal(err)
	}

	got := readFile(t, path)
	if n := strings.Count(got, "aws_ami"); n != 1 {
		t.Fatalf("aws_ami appears %d times, want 1:\n%s", n, got)
	}
	if !strings.Contains(got, `aws_ami = "ami-new"`) {
		t.Errorf("value not replaced:\n%s", got)
	}
	if !strings.Contains(got, `region = "us-east-2"`) {
		t.Errorf("unrelated line touched:\n%s", got)
	}
}

func TestSetOrAppendTFVarReplacesLineWithInlineComment(t *testing.T) {
	path := writeTFVars(t, "aws_ami         = \"ami-old\" # SLES 16\n")

	if err := setOrAppendTFVar(path, "aws_ami", "ami-new"); err != nil {
		t.Fatal(err)
	}

	got := readFile(t, path)
	if n := strings.Count(got, "aws_ami"); n != 1 {
		t.Fatalf("commented line was not replaced — %d aws_ami entries:\n%s", n, got)
	}
	if !strings.Contains(got, `aws_ami = "ami-new"`) {
		t.Errorf("value not replaced:\n%s", got)
	}
}

func TestSetOrAppendTFVarAppendsMissingKey(t *testing.T) {
	for name, content := range map[string]string{
		"with trailing newline":    "region = \"us-east-2\"\n",
		"without trailing newline": "region = \"us-east-2\"",
		"empty file":               "",
	} {
		t.Run(name, func(t *testing.T) {
			path := writeTFVars(t, content)
			if err := setOrAppendTFVar(path, "aws_ami", "ami-new"); err != nil {
				t.Fatal(err)
			}

			got := readFile(t, path)
			if !strings.Contains(got, "\naws_ami = \"ami-new\"\n") && got != "aws_ami = \"ami-new\"\n" {
				t.Errorf("append broke line separation:\n%q", got)
			}
		})
	}
}

func TestThreadRuntimeEnvAMIOverridesCommentedLine(t *testing.T) {
	clearRuntimeEnvs(t)
	t.Setenv("AWS_AMI", "ami-override")
	path := writeTFVars(t, "aws_ami = \"ami-seeded\" # RHEL 9.8\n")

	if err := threadRuntimeEnvIntoTFVars(path); err != nil {
		t.Fatal(err)
	}

	got := readFile(t, path)
	if n := strings.Count(got, "aws_ami"); n != 1 {
		t.Fatalf("override duplicated aws_ami (%d entries):\n%s", n, got)
	}
	if !strings.Contains(got, `aws_ami = "ami-override"`) {
		t.Errorf("AWS_AMI override not applied:\n%s", got)
	}
}

func TestThreadRuntimeEnvInstanceTypeFallback(t *testing.T) {
	clearRuntimeEnvs(t)
	t.Setenv("EC2_INSTANCE_CLASS", "t3.large")
	path := writeTFVars(t, "")

	if err := threadRuntimeEnvIntoTFVars(path); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(readFile(t, path), `instance_type = "t3.large"`) {
		t.Fatal("EC2_INSTANCE_CLASS fallback not written")
	}

	// primary wins over fallback
	t.Setenv("INSTANCE_TYPE", "t3.xlarge")
	if err := threadRuntimeEnvIntoTFVars(path); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, path)
	if !strings.Contains(got, `instance_type = "t3.xlarge"`) || strings.Count(got, "instance_type") != 1 {
		t.Fatalf("INSTANCE_TYPE should win and replace in place:\n%s", got)
	}
}

func TestUpdateVarsFilePrefixLimit(t *testing.T) {
	path := writeTFVars(t, "")

	// dsf-<resource>-rke2-xxxxx over 24 chars must be rejected before tofu runs.
	err := updateVarsFile(path, "abcde", "rke2", "waytoolongresourcename")
	if err == nil || !strings.Contains(err.Error(), "aws_hostname_prefix") {
		t.Fatalf("expected prefix-length error, got %v", err)
	}

	if err := updateVarsFile(path, "abcde", "rke2", "fmorx"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(readFile(t, path), `aws_hostname_prefix = "dsf-fmorx-rke2-abcde"`) {
		t.Fatalf("prefix not written:\n%s", readFile(t, path))
	}
}

func TestUsesExternalDBProvisioning(t *testing.T) {
	cases := []struct {
		name string
		envs map[string]string
		want bool
	}{
		{"etcd datastore", map[string]string{"DATASTORE_TYPE": "etcd"}, false},
		{"external clean", map[string]string{"DATASTORE_TYPE": "external"}, true},
		{"external lowercase env", map[string]string{"datastore_type": "External"}, true},
		{
			"external with endpoint",
			map[string]string{"DATASTORE_TYPE": "external", "EXTERNAL_DB_ENDPOINT": "mysql://x"},
			false,
		},
		{
			"external with endpoint in server flags",
			map[string]string{"DATASTORE_TYPE": "external", "SERVER_FLAGS": "datastore-endpoint: mysql://x"},
			false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			clearRuntimeEnvs(t)
			for k, v := range tc.envs {
				t.Setenv(k, v)
			}
			if got := usesExternalDBProvisioning(); got != tc.want {
				t.Errorf("usesExternalDBProvisioning() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestExternalDBIntoTFVarsWritesOnlyWhenProvisioning(t *testing.T) {
	clearRuntimeEnvs(t)
	t.Setenv("DATASTORE_TYPE", "external")
	t.Setenv("EXTERNAL_DB", "MySQL")
	t.Setenv("EXTERNAL_DB_VERSION", "8.4.8")
	path := writeTFVars(t, "")

	if err := externalDBIntoTFVars(path); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, path)
	for _, want := range []string{
		`datastore_type = "external"`,
		`external_db = "mysql"`, // engine selector is lowercased
		`external_db_version = "8.4.8"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}

	// endpoint provided -> Path A, nothing may be written
	t.Setenv("EXTERNAL_DB_ENDPOINT", "mysql://u:p@h/db")
	path2 := writeTFVars(t, "")
	if err := externalDBIntoTFVars(path2); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, path2); got != "" {
		t.Errorf("Path A must not write RDS vars, got:\n%s", got)
	}
}

func TestHclStringList(t *testing.T) {
	if got := hclStringList("subnet-a, subnet-b,subnet-c"); got != `["subnet-a", "subnet-b", "subnet-c"]` {
		t.Errorf("hclStringList = %s", got)
	}
}

func TestLoadVarsFromFileParsesClusterConfig(t *testing.T) {
	content := `
# comment line
aws_region       = "us-east-2"
subnets          = "subnet-1ed44d64"
aws_security_group = ["sg-08e8243a8cfbea8a0", "sg-other"]
vpc_id           = "vpc-bfccf4d7"
aws_ami          = "ami-0dcbeb6d585e578d0" # RHEL 10.2
volume_size      = "50"
ec2_instance_class = "t3.xlarge"
aws_ssh_user     = "ec2-user"
`
	path := writeTFVars(t, content)
	var cluster driver.Cluster
	if err := loadVarsFromFile(&cluster, false, false, path); err != nil {
		t.Fatal(err)
	}

	checks := map[string][2]string{
		"region":         {cluster.Aws.Region, "us-east-2"},
		"subnets":        {cluster.Aws.Subnets, "subnet-1ed44d64"},
		"sg first item":  {cluster.Aws.SgId, "sg-08e8243a8cfbea8a0"},
		"vpc":            {cluster.Aws.VPCID, "vpc-bfccf4d7"},
		"ami (comment)":  {cluster.Aws.EC2.Ami, "ami-0dcbeb6d585e578d0"},
		"volume":         {cluster.Aws.EC2.VolumeSize, "50"},
		"instance class": {cluster.Aws.EC2.InstanceClass, "t3.xlarge"},
		"ssh user":       {cluster.SSH.User, "ec2-user"},
	}
	for name, c := range checks {
		if c[0] != c[1] {
			t.Errorf("%s = %q, want %q", name, c[0], c[1])
		}
	}
}

// The provisioner rewrites infrastructure/qainfra/main.tf by literal string
// replacement, so the template must keep both anchors verbatim: the module
// source placeholder and the external_db injection marker (losing the marker
// silently breaks DATASTORE_TYPE=external — the RDS module is never injected
// and the run only fails much later, at datastore_endpoint lookup).
func TestInfraMainTfKeepsInjectionAnchors(t *testing.T) {
	path := filepath.Join("..", "..", "..", "infrastructure", "qainfra", "main.tf")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}

	for _, anchor := range []string{externalDBMarker, "placeholder-for-remote-module"} {
		if !strings.Contains(string(content), anchor) {
			t.Errorf("infrastructure/qainfra/main.tf lost required anchor %q", anchor)
		}
	}
}
