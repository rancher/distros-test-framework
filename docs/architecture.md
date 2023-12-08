## Architecture

For better maintenance, readability and productivity we encourage max of separation of concerns and loose coupling between packages so inner packages should not depend on outer packages. "External" and "Outer" layer or dependency here in this context is considered any other package within the framework.



### Packages:
```bash
./distros-test-framework
│
├── entrypoint
│   └───── Entry for tests execution, separated by test runs and test suites
│
├── internal
│   └── Core logic and internal services implementations including assert, aws sdk, customflag, logger, template, testcases.
│
├── modules
│   └───── Terraform modules and configurations
│
│── scripts
│    └───── Scripts needed for overall execution
│
├── shared
│    └───── auxiliary and reusable functions
│
│── workloads
│   └───── Place where resides workloads to use inside tests
```

### Explanation:

- `Internal`
```
    Testcase:
  
Act:                  Acts as an innermost layer where the main logic (test implementation) is handled.
Responsibility:       Encapsulates test logic and should not depend on any outer layer
```

- `Entrypoint`
````
Act:                  Acts as the one of the outer layer to receive the input to start test execution
Responsibility:       Should not implement any logic and only focus on orchestrating
````

- `Modules`
```
Act:                  Acts as the infra to provide the terraform modules and configurations
Responsibility:       Only provides indirectly for all, should not need the knowledge of any test logic or have dependencies from internal layers.
```

- `Scripts`
```
Act:                  Acts as a provider for scripts needed for overall execution
Responsibility:       Should not need knowledge of or "external" dependencies at all and provides for all layers.
```

- `Shared`
```
Act:                  Acts as an intermediate module providing shared, reusable and auxiliary functions
Responsibility:       Should not need knowledge of or "external" dependencies at all and provides for all layers.
```

- `Workloads`
````
Act:                  Acts as a provider for test workloads
Responsibility:       Totally independent of any other layer and should only provide
````