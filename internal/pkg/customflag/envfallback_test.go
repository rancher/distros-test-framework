package customflag

import (
	"flag"
	"testing"
)

func TestFillFromEnv(t *testing.T) {
	fs := flag.NewFlagSet("t", flag.ContinueOnError)
	version := fs.String("chartsVersion", "", "")
	certManager := fs.String("certManagerVersion", "v1.16.1", "")
	kdm := fs.String("kdmSource", "", "")
	unset := fs.String("customNodes", "etcd:3", "")
	var destroy destroyFlag
	fs.Var(&destroy, "destroy", "")
	if err := fs.Parse([]string{"-kdmSource", "https://github.com/o/r/pull/1"}); err != nil {
		t.Fatal(err)
	}

	t.Setenv("CHARTS_VERSION", "2.16.0-7c25ce9-head")
	t.Setenv("CERT_MANAGER_VERSION", "v1.19.4")
	t.Setenv("KDM_SOURCE", "https://example.com/ignored.json")
	t.Setenv("CUSTOM_NODES", "")
	t.Setenv("DESTROY", "false")

	err := FillFromEnv(fs, map[string]string{
		"chartsVersion": "CHARTS_VERSION", "certManagerVersion": "CERT_MANAGER_VERSION",
		"kdmSource": "KDM_SOURCE", "customNodes": "CUSTOM_NODES", "destroy": "DESTROY",
	})
	if err != nil {
		t.Fatal(err)
	}
	if *version != "2.16.0-7c25ce9-head" || *certManager != "v1.19.4" {
		t.Errorf("env did not fill unset flags: chartsVersion=%q certManagerVersion=%q", *version, *certManager)
	}
	if *kdm != "https://github.com/o/r/pull/1" {
		t.Errorf("env overrode a command-line flag: kdmSource=%q", *kdm)
	}
	if *unset != "etcd:3" {
		t.Errorf("empty env replaced the default: customNodes=%q", *unset)
	}
	if destroy {
		t.Error("DESTROY=false not applied")
	}

	fresh := flag.NewFlagSet("fresh", flag.ContinueOnError)
	var freshDestroy destroyFlag
	fresh.Var(&freshDestroy, "destroy", "")
	t.Setenv("DESTROY", "maybe")
	if err = FillFromEnv(fresh, map[string]string{"destroy": "DESTROY"}); err == nil {
		t.Error("invalid DESTROY value accepted")
	}
}
