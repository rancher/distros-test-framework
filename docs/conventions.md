## Best Practices, Standards, and Conventions

### TestSuite naming convention:
- All testsuites should be placed under `./entrypoint/<TESTSUITE_NAME>`.
- All testsuite functions should be named: `<TESTSUITE_NAME>_suite_test.go`.

### Testcase naming convention:
- All tests should be placed under `./testcase/<TESTNAME>`.
- All test functions should be named: `Test<TESTNAME>`.

### Contributing:
- All contributions should be made via pull requests.
- Before pushing please run `make pre-commit` to format your code and run linters.
- All pull requests should be made against the `main` branch.
- All pull requests should be reviewed by at least two other person before being merged.
- All pull requests should be squashed into a single commit before being merged.
- All pull requests should be rebased/merged against the `main` branch before being merged.
- All pull requests should be tested locally before being merged.