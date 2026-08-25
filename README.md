# logo

**logo**（**log** + g**o**）是一个可复用的 Go 日志框架：强制注册 scope/module、分级日志级别，并支持按天 / 按大小归档。

模块路径：`github.com/wifi504/logo`

## 安装

```bash
go get github.com/wifi504/logo
```

## 日志行格式

```text
[yyyy-MM-dd hh:mm:ss SSS] [scope] [level] [module] 文件名:行号 : 日志内容
```

示例：

```text
[2026-08-25 16:24:01 123] [app] [info] [server] main.go:42 : HTTP server listening on :8080
```

## 快速开始

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

scope / module **必须在代码里先注册**。YAML（或其它配置源）只为已注册名称提供级别覆盖，不能代替注册。

## 配置段

本库**不负责**读取配置文件。请提供可反序列化为 `logo.Config` 的 `logs` 段：

```yaml
logs:
  level: debug          # 全局默认级别：debug | info | warn | error
  dir: logs             # latest.log 与归档所在目录
  max_size_mb: 10       # latest.log 达到该大小（MB）时滚动
  compress: true        # 归档是否再打成 .gz
  max_backups: 30       # 最多保留的归档文件个数
  max_age_days: 30      # 归档保留天数；0 表示不按天龄删除
  stdout: true          # 是否同时输出到控制台
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

级别优先级（越具体越高）：

`scopes.<scope>.modules.<module>.level` > `scopes.<scope>.level` > `logs.level`

## 归档规则

| 项目 | 规则 |
|------|------|
| 当前写入文件 | `{dir}/latest.log`（写死，不可配置） |
| 归档文件名 | `log-yyyy-MM-dd-001`，同日多次则为 `002`、`003`… |
| 按量触发 | `latest.log` ≥ `max_size_mb` |
| 按天触发 | 本地时区每天 0 点（未满大小也会归档；空文件跳过） |
| 压缩 | `compress: true` 时生成 `log-yyyy-MM-dd-NNN.gz` |

## 对接其它库

```go
engineLog := ginScope.RegisterModule("engine")
// 例如：gin.DefaultWriter = engineLog.Writer(logo.LevelInfo)
_ = engineLog.Writer(logo.LevelInfo)
```

## 许可证

MIT © WIFI连接超时
