##  Common SSH Exit Codes.

| Exit Code | Category | Common Error Messages |
|:---------:|:---------|:----------------------|
| **1** | Generic Failure | `exit status 1` |
| **2** | Shell Built-in Misuse | `exit status 2`, `usage:` |
| **124** | Timeout Exceeded | `command timed out`, `exit status 124` |
| **126** | Permission Error | `exit status 126`, `permission denied`, `cannot execute`, `exec format error` |
| **127** | Command Not Found | `exit status 127`, `command not found`, `no such file or directory` |
| **130** | User Interruption | `exit status 130`, `terminated by signal 2`, `interrupted` |
| **137** | Out of Memory | `exit status 137`, `killed`, `terminated by signal 9`, `out of memory` |
| **255** | Connection Issue | `exit status 255`, `connection lost`, `connection reset by peer`, `connection refused`, `no route to host` |


### Common Error Messages that could possible be retryable.
```
"exit status 1",
"command timed out",
"no such file or directory",
"exit status 124",
"connection reset by peer",
"connect: connection refused",
"connect: operation timed out",
"exit status 255",
"remote command exited without exit status",
```

### RunCommandOnNodeWithRetry() Usage:
```go
func YourFuncWhatever() {
    // Define the command to be executed
    command := "your_command_here"

    // Define the IP address of the node
    ip := "your_node_ip_here"

    // create the config
    cfg := shared.CmdNodeRetryCfg()
    cfg.Attempts = 20
    cfg.Delay = 10 * time.Second

    // If you dont wanna use the default config, just call
    cfg.<your_config_here>= <your_value_here>

    // Call the RunCommandOnNodeWithRetry function
    out, err := RunCommandOnNodeWithRetry(command, ip, cfg)
}
```
