package main

import (
	"fmt"
	// "github.com/rancher/distros-test-framework/pkg/aws"
)

type Whatever interface {
	CreateInstances(names ...string) (ids, ips []string, err error)
	DeleteInstance(ip string) error
	WaitForInstanceRunning(instanceId string) error
}

func testCase(w Whatever) error {
	ids, ips, err := w.CreateInstances("fmoral-test-instance-11")
	if err != nil {
		return err
	}
	fmt.Println(ids, ips)

	return nil
}

func main() {
	// TestApplyContainerdQoSClassConfigFileIfPresent(t)

	// whatever, err := aws.AddAwsNode()
	// if err != nil {
	// 	fmt.Println(err)
	// }
	//
	// a := testCase(whatever)
	// fmt.Println(a)
	// c, err := testcase.AddCmd("k3s", "server", "18.222.49.36", "test", "v1.26")
	// if err != nil {
	// 	fmt.Println(err)
	// }
	// fmt.Println(c)
	//
	// a, err := shared.RunCommandHost(c)
	// if err != nil {
	// 	fmt.Println(err)
	// }
	// fmt.Println(a)
	// err = whatever.DeleteInstance("18.227.79.172")

}
