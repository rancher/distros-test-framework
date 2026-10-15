package releasebot

import (
	"reflect"
	"testing"
)

func TestParseRequest(t *testing.T) {
	msg := "please test `v1.37.1-rc1+k3s1` v1.36.5-rc1%2Bk3s1 and " +
		"<https://github.com/rancher/rke2/releases/tag/v1.37.1-rc2+rke2r1|v1.37.1-rc2+rke2r1>" +
		" v1.37.1-rc1+k3s1"

	got := ParseRequest(msg)
	want := Request{
		K3s:  []string{"v1.36.5-rc1+k3s1", "v1.37.1-rc1+k3s1"},
		RKE2: []string{"v1.37.1-rc2+rke2r1"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v\nwant %+v", got, want)
	}

	if !ParseRequest("hello bot").Empty() {
		t.Fatal("expected empty request")
	}
}

func TestBaseVersion(t *testing.T) {
	if got := baseRC("v1.37.1-rc2+rke2r1"); got != "v1.37.1-rc2" {
		t.Fatal(got)
	}
	if got := baseVersion("v1.37.1-rc2+rke2r1"); got != "v1.37.1" {
		t.Fatal(got)
	}
}
