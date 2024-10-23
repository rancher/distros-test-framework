#### Qase Reporter Package
- Required vars to export locally or add on .env file:
```
export REPORT_TO_QASE=true
export QASE_AUTOMATION_TOKEN=your_token
export QASE_PROJECT_ID=your_project_code
export QASE_RUN_ID=your_run_id
export QASE_TEST_CASE_ID=your_test_case_id
```


- NEXT STEPS
```
() Create a method to automatically create a patch validation run on qase
() Create a method to publicy and finish results on qase.
() Create a method to report final results on slack.
() Tag execution on qase so we can differ when it was ran locally or on jenkins.
```

