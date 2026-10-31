package aws

import (
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/ec2/ec2iface"

	"github.com/rancher/distros-test-framework/internal/provisioning/driver"
)

type fakeEC2 struct {
	ec2iface.EC2API
	byFilter     map[string][]string
	describeErr  error
	terminateErr error
	waitErr      error
	gotFilters   [][]*ec2.Filter
	terminated   []string
	waited       int
}

func (f *fakeEC2) DescribeInstances(in *ec2.DescribeInstancesInput) (*ec2.DescribeInstancesOutput, error) {
	f.gotFilters = append(f.gotFilters, in.Filters)
	if f.describeErr != nil {
		return nil, f.describeErr
	}
	out := &ec2.DescribeInstancesOutput{}
	for _, id := range f.byFilter[aws.StringValue(in.Filters[0].Name)] {
		out.Reservations = append(out.Reservations, &ec2.Reservation{
			Instances: []*ec2.Instance{{InstanceId: aws.String(id)}},
		})
	}

	return out, nil
}

func (f *fakeEC2) TerminateInstances(in *ec2.TerminateInstancesInput) (*ec2.TerminateInstancesOutput, error) {
	if f.terminateErr != nil {
		return nil, f.terminateErr
	}
	f.terminated = append(f.terminated, aws.StringValue(in.InstanceIds[0]))

	return &ec2.TerminateInstancesOutput{}, nil
}

func (f *fakeEC2) WaitUntilInstanceTerminated(*ec2.DescribeInstancesInput) error {
	f.waited++

	return f.waitErr
}

func TestInstanceIDByIP(t *testing.T) {
	tests := []struct {
		name    string
		ip      string
		byF     map[string][]string
		want    string
		wantErr bool
	}{
		{"public ip", "1.2.3.4", map[string][]string{"ip-address": {"i-1"}}, "i-1", false},
		{"private ip", "10.0.0.4", map[string][]string{"private-ip-address": {"i-2"}}, "i-2", false},
		{"ipv6", "2600::1", map[string][]string{"ipv6-address": {"i-3"}}, "i-3", false},
		{"same instance via both filters", "1.2.3.4",
			map[string][]string{"ip-address": {"i-1"}, "private-ip-address": {"i-1"}}, "i-1", false},
		{"no live instance means already gone", "1.2.3.4", nil, "", false},
		{"ambiguous match fails", "1.2.3.4",
			map[string][]string{"ip-address": {"i-1", "i-9"}}, "", true},
		{"empty ip", "", nil, "", true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c := Client{ec2: &fakeEC2{byFilter: tc.byF}}
			got, err := c.InstanceIDByIP(tc.ip)
			if (err != nil) != tc.wantErr || got != tc.want {
				t.Fatalf("got (%q, %v), want (%q, err=%v)", got, err, tc.want, tc.wantErr)
			}
		})
	}
}

func TestInstanceIDByIPScopesStateAndSubnet(t *testing.T) {
	f := &fakeEC2{}
	c := Client{ec2: f, infra: &driver.Cluster{Aws: driver.AwsConfig{Subnets: "subnet-1"}}}
	if _, err := c.InstanceIDByIP("1.2.3.4"); err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, f := range f.gotFilters[0] {
		seen[aws.StringValue(f.Name)] = true
	}
	for _, want := range []string{"instance-state-name", "subnet-id"} {
		if !seen[want] {
			t.Errorf("missing %s filter", want)
		}
	}
}

func TestInstanceIDByIPDescribeError(t *testing.T) {
	c := Client{ec2: &fakeEC2{describeErr: errors.New("boom")}}
	if _, err := c.InstanceIDByIP("1.2.3.4"); err == nil {
		t.Fatal("expected error")
	}
}

func TestTerminateInstanceByIDAndWait(t *testing.T) {
	f := &fakeEC2{}
	if err := (Client{ec2: f}).TerminateInstanceByIDAndWait("i-1"); err != nil {
		t.Fatal(err)
	}
	if len(f.terminated) != 1 || f.waited != 1 {
		t.Fatalf("terminated=%v waited=%d", f.terminated, f.waited)
	}
}

func TestTerminateInstanceByIDAndWaitNotFoundIsIdempotent(t *testing.T) {
	f := &fakeEC2{terminateErr: awserr.New("InvalidInstanceID.NotFound", "gone", nil)}
	if err := (Client{ec2: f}).TerminateInstanceByIDAndWait("i-1"); err != nil {
		t.Fatalf("not-found must succeed, got %v", err)
	}
	if f.waited != 0 {
		t.Error("should not wait when instance is unknown")
	}
}

func TestTerminateInstanceByIDAndWaitErrors(t *testing.T) {
	c := Client{ec2: &fakeEC2{terminateErr: errors.New("denied")}}
	if err := c.TerminateInstanceByIDAndWait("i-1"); err == nil {
		t.Error("expected terminate error")
	}
	c = Client{ec2: &fakeEC2{waitErr: errors.New("timeout")}}
	if err := c.TerminateInstanceByIDAndWait("i-1"); err == nil {
		t.Error("expected wait error")
	}
	if err := (Client{ec2: &fakeEC2{}}).TerminateInstanceByIDAndWait(""); err == nil {
		t.Error("expected empty id error")
	}
}
