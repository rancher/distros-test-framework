package aws

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/request"
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
	blockWait    bool
	gotFilters   [][]*ec2.Filter
	terminated   []string
	waited       int
}

func (f *fakeEC2) DescribeInstancesWithContext(
	_ aws.Context, in *ec2.DescribeInstancesInput, _ ...request.Option,
) (*ec2.DescribeInstancesOutput, error) {
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

func (f *fakeEC2) TerminateInstancesWithContext(
	_ aws.Context, in *ec2.TerminateInstancesInput, _ ...request.Option,
) (*ec2.TerminateInstancesOutput, error) {
	if f.terminateErr != nil {
		return nil, f.terminateErr
	}
	f.terminated = append(f.terminated, aws.StringValue(in.InstanceIds[0]))

	return &ec2.TerminateInstancesOutput{}, nil
}

func (f *fakeEC2) WaitUntilInstanceTerminatedWithContext(
	ctx aws.Context, _ *ec2.DescribeInstancesInput, _ ...request.WaiterOption,
) error {
	f.waited++
	if f.blockWait {
		<-ctx.Done()

		return ctx.Err()
	}

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
	hasFilter := func(call []*ec2.Filter, name string) bool {
		for _, fl := range call {
			if aws.StringValue(fl.Name) == name {
				return true
			}
		}

		return false
	}
	// gotFilters[0] is the public ip-address lookup, [1] the private-ip-address one.
	if !hasFilter(f.gotFilters[0], "instance-state-name") || !hasFilter(f.gotFilters[1], "instance-state-name") {
		t.Error("every lookup must filter by live state")
	}
	if hasFilter(f.gotFilters[0], "subnet-id") {
		t.Error("public ip lookup must not be subnet scoped")
	}
	if !hasFilter(f.gotFilters[1], "subnet-id") {
		t.Error("private ip lookup must be subnet scoped")
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

func TestTerminateInstanceByIDAndWaitIsBoundedByDeadline(t *testing.T) {
	prev := terminateTimeout
	terminateTimeout = 50 * time.Millisecond
	t.Cleanup(func() { terminateTimeout = prev })

	f := &fakeEC2{blockWait: true}
	start := time.Now()
	err := (Client{ec2: f}).TerminateInstanceByIDAndWait("i-1")
	if err == nil || time.Since(start) > 2*time.Second {
		t.Fatalf("expected a prompt deadline error, got %v after %s", err, time.Since(start))
	}
	if !errors.Is(err, context.DeadlineExceeded) && f.waited != 1 {
		t.Errorf("waiter not reached or error not deadline related: %v", err)
	}
}
