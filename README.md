# logo

**logo** (**log** + g**o**) is a small, reusable Go logging framework with explicit scope/module registration, hierarchical levels, and daily/size-based archives.

Module: `github.com/wifi504/logo`

## Install

```bash
go get github.com/wifi504/logo
```

## Log line format

```text
[yyyy-MM-dd hh:mm:ss SSS] [scope] [level] [module] file:line : message
```

Example:

```text
[2026-08-25 16:24:01 123] [app] [info] [server] main.go:42 : HTTP server listening on :8080
```

## Quick start

```go
package main

import "github.com/wifi504/logo"

func main() {
    app := logo.RegisterScope("app")
    serverLog := app.RegisterModule("server")

    if err := logo.Init(logo.Config{
        Level:     "debug",
        Dir:       "logs",
        MaxSizeMB: 10,
        Compress:  true,
        Stdout:    true,
        Scopes: map[string]logo.ScopeConfig{
            "app": {
                Level: "debug",
                Modules: map[string]logo.ModuleConfig{
                    "server": {Level: "info"},
                },
            },
        },
    }); err != nil {
        panic(err)
    }
    defer logo.Close()

    serverLog.Info("listening on %s", ":8080")
}
```

Scopes and modules **must be registered** in code. YAML (or any config source) only supplies level overrides for already-registered names.

## Configuration section

The library does **not** load config files. Define a `logs` section that unmarshals into `logo.Config`:

```yaml
logs:
  level: debug          # global default: debug | info | warn | error
  dir: logs             # directory for latest.log and archives
  max_size_mb: 10       # rotate when latest.log reaches this size
  compress: true        # gzip archives
  max_backups: 30       # keep at most N archives
  max_age_days: 30      # delete archives older than N days (0 = disable)
  stdout: true          # also print to console
  scopes:
    app:
      level: debug
      modules:
        server:
          level: debug
    gin:
      level: info
      modules:
        engine:
          level: debug
        access:
          level: info
```

```go
var root struct {
    Logs logo.Config `yaml:"logs"`
}
// yaml.Unmarshal(..., &root)
logo.Init(root.Logs)
```

Level precedence (most specific wins):

`scopes.<scope>.modules.<module>.level` > `scopes.<scope>.level` > `logs.level`

## Archives

| Item | Rule |
|------|------|
| Active file | `{dir}/latest.log` (fixed name) |
| Archive name | `log-yyyy-MM-dd-001`, then `002`… same day |
| Size trigger | when `latest.log` ≥ `max_size_mb` |
| Time trigger | local midnight (even if under size; empty file skipped) |
| Compress | if `compress: true` → `log-yyyy-MM-dd-NNN.gz` |

## Bridging other libraries

```go
engineLog := ginScope.RegisterModule("engine")
// e.g. gin.DefaultWriter = engineLog.Writer(logo.LevelInfo)
_ = engineLog.Writer(logo.LevelInfo)
```

## License

MIT © WIFI连接超时
