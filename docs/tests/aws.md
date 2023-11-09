#### How to use aws sdk wrapper:

#### Example:

```go
    package main


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