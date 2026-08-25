# logo

**logo**（**log** + g**o**）是一个可复用的 Go 日志框架：强制注册 scope/module、分级日志级别，并支持按天 / 按大小归档；可接管 `os.Stdout` / `os.Stderr`。

模块路径：`github.com/wifi504/logo`  
当前版本：见 `logo.Version`（与 Git tag 一致，如 `v1.0.0`）

## 安装

```bash
go get github.com/wifi504/logo@v1.0.0
```

## 日志行格式

```text
[yyyy-MM-dd hh:mm:ss SSS][scope][LEVEL][module] 文件名:行号 : 日志内容
```

- 中括号之间无空格
- 级别全大写：`DEBUG` / `INFO` / `WARN` / `ERROR`

示例：

```text
[2026-08-25 16:24:01 123][app][INFO][server] main.go:42 : HTTP server listening on :8080
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
        // CaptureStd 默认 true；测试可 logo.Bool(false)
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

scope / module **必须在代码里先注册**。YAML 只为已注册名称提供级别覆盖。

`Init` 会打印 ASCII banner（含 `Version`），并**同时写入** `latest.log` 与控制台（当 `stdout: true`）。

## 配置段

本库**不负责**读取配置文件。请提供可反序列化为 `logo.Config` 的 `logs` 段：

```yaml
logs:
  level: debug
  dir: logs
  max_size_mb: 10
  compress: true
  max_backups: 30
  max_age_days: 30
  stdout: true
  capture_std: true   # 接管 os.Stdout / os.Stderr；省略时默认为 true
  scopes:
    app:
      level: debug
      modules:
        server:
          level: debug
```

级别优先级：`module` > `scope` > `logs.level`。

## 接管标准输出

类似 Java 的 `System.setOut`：`Init` 后替换 `os.Stdout` / `os.Stderr`。未走 `Logger.*` 的输出会被封装为：

```text
[...][未配置Logo域和模块][WARN][unknown] stdout : ...
[...][未配置Logo域和模块][ERROR][unknown] stderr : ...
```

出处固定为 `stdout` / `stderr`（不展示文件行号）。框架自身写日志使用原始控制台副本，避免递归。

能兜住：`fmt.Print*`、`println`、多数写 `os.Stdout` 的代码、标准库 `log`（stderr）。  
兜不住：Init 前已缓存旧 `*File` 的库、直接写 fd 的 CGO。

## 归档规则

| 项目 | 规则 |
|------|------|
| 当前写入 | `{dir}/latest.log` |
| 归档名 | `log-yyyy-MM-dd-001` 同日递增 |
| 触发 | 满 `max_size_mb` 或本地每天 0 点 |
| 压缩 | `compress: true` → `.gz` |

## 许可证

MIT © WIFI连接超时
