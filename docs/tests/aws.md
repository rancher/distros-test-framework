#### How to test aws sdk wrapper:

#### Example:

```go
package main

import (
"fmt"

"github.com/rancher/distros-test-framework/pkg/aws"
)

type Whatever interface {
CreateInstances(names ...string) (ids, ips []string, err error)
DeleteInstance(ip string) error
WaitForInstanceRunning(instanceId string) error
}

func testCase(w Whatever) error {
ids, ips, err := w.CreateInstances("fmoral-test-instance-12")
if err != nil {
return err
}
fmt.Println(ids, ips)

return nil
}

func main() {
dependencies, err := aws.AddAwsNode()
if err != nil {
fmt.Println(err)
}

a := testCase(dependencies)
// err = e.DeleteInstance("1.111.11.1")

fmt.Println(a)
}
 ```

## Please note that today this is tied up with terraform, so if you want to test it separately you need to in the pass the values instead of the struct implementation.

```go
func AddAwsNode() (*Client, error) {
{{/*c := factory.ClusterConfig(GinkgoT())*/}} // remove this and add the values directly


sess, err := session.NewSession(&aws.Config{
Region: aws.String(c.AwsEc2.Region)})
if err != nil {
return nil, shared.ReturnLogError("error creating AWS session: %v", err)
}

return &Client{
infra: &factory.Cluster{AwsEc2: c.AwsEc2}, // and here add your values for each field removing the factory.Cluster
ec2:   ec2.New(sess),
}, nil
}

```



