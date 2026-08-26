# logo

面向 Go 的轻量日志框架：显式注册 scope / module、分级日志级别，以及按体积或按日滚动归档。

| 项目 | 说明 |
|------|------|
| Module | [`github.com/wifi504/logo`](https://github.com/wifi504/logo) |
| License | [MIT](LICENSE) |
| Version | [`logo.Version`](version.go)（与 Git tag 对齐，例如 `v1.0.0`） |

## 特性

- 通过 `RegisterScope` / `RegisterModule` 注册后，使用 `*Logger` 的 `Debug` / `Info` / `Warn` / `Error` 输出
- 级别继承：module 覆盖 scope，scope 覆盖全局
- 固定行格式；级别以大写输出（`DEBUG` / `INFO` / `WARN` / `ERROR`）
- scope / module 列宽按已出现名称动态对齐（上限 15）；来源列固定 16
- 当前文件 `latest.log`；归档名为 `log-yyyy-MM-dd-NNN`（可选 gzip）
- 按 `max_size_mb` 与本地零点触发滚动
- 默认接管 `os.Stdout` / `os.Stderr`（包加载时尽量提前；并重绑标准库 `log`）
- 启动时输出 ASCII banner，并写入日志文件（`stdout` 开启时同步到控制台）

## 安装

```bash
go get github.com/wifi504/logo@latest
```

需要锁定某一发布版时，将 `@latest` 换成对应 Git tag（例如 `@v1.0.4`）。当前版本以仓库 tag 与 [`logo.Version`](version.go) 为准。

## 日志格式

```text
[yyyy-MM-dd hh:mm:ss SSS][scope][LEVEL][module] file:line…      : message
```

示例（列按当前已出现名称的最大宽度对齐）：

```text
[2026-08-25 16:24:01 123][app ][INFO ][server] main.go:42       : listening on :8080
[2026-08-25 16:24:01 200][gin ][DEBUG][engine] logging.go:46    : GET /healthz
```

约定：

| 字段 | 规则 |
|------|------|
| scope / module | 注册名 ≤ 15 个 rune；列宽取**已出现名称的最大长度**（不超过 15），左对齐补空格 |
| LEVEL | 大写；左对齐，固定宽 5 |
| 来源 | 左对齐固定宽 **16**；超过则**截断左侧**保留末尾；不足补空格 |
| 冒号 | 以上列对齐后，` : ` 前竖直对齐 |

## 快速开始

scope 与 module 须在代码中先行注册。配置仅对已注册名称提供级别覆盖。

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

`Init` 会输出包含 `logo.Version` 的 banner，并写入 `latest.log`；在 `stdout: true` 时同时写入真实控制台。

完整示例见 [`examples/basic`](examples/basic)。

## 配置

本库不解析业务侧的配置文件路径。将 `logs` 段（或等价结构）反序列化为 `logo.Config` 后传入 `Init`：

```go
var root struct {
	Logs logo.Config `yaml:"logs"`
}
// yaml.Unmarshal(data, &root)
logo.Init(root.Logs)
```

YAML 示例：

```yaml
logs:
  level: debug
  dir: logs
  max_size_mb: 10
  compress: true
  max_backups: 30
  max_age_days: 30
  stdout: true
  scopes:
    app:
      level: debug
      modules:
        server:
          level: debug
```

| 字段 | 说明 |
|------|------|
| `level` | 全局默认级别：`debug` \| `info` \| `warn` \| `error` |
| `dir` | `latest.log` 与归档目录 |
| `max_size_mb` | `latest.log` 达到该大小时滚动 |
| `compress` | 归档是否 gzip |
| `max_backups` | 归档文件保留个数上限 |
| `max_age_days` | 按天龄删除归档；`0` 表示不启用 |
| `stdout` | 是否同时写入真实控制台 |
| `scopes` | 按 scope / module 覆盖级别 |

**级别优先级（由高到低）：**  
`scopes.<scope>.modules.<module>.level` → `scopes.<scope>.level` → `logs.level`。

## 标准输出接管

劫持为框架默认行为，不可关闭。包 `init` 阶段会尽可能早地替换 `os.Stdout` / `os.Stderr`（`go test` 下会跳过以避免干扰测试框架）；`Init` 时再次确保劫持，并调用 `log.SetOutput(os.Stderr)`，使已缓存默认 logger 的库（如 GORM 默认日志）一并进入管道。

未经过 `Logger` 方法的写入将被改写为：

```text
[…][未配置Logo域和模块][WARN ][unknown] stdout           : …
[…][未配置Logo域和模块][ERROR][unknown] stderr           : …
```

出处字段固定为 `stdout` 或 `stderr`。框架内部输出使用已保存的控制台句柄，以避免递归捕获。`Init` 之前的捕获行会先打到真实控制台，并在 `Init` 后写入日志文件。

| 范围 | 说明 |
|------|------|
| 可捕获 | `fmt.Print*`、`println`、标准库 `log`（经 `SetOutput` 重绑）、多数写入当前 `os.Stdout` / `os.Stderr` 的代码 |
| 不可捕获 | 自行长期持有旧 `*os.File` 且不走 `log.SetOutput` 的依赖；直接写入 fd 1/2 的本地代码 |

## 归档

| 项目 | 行为 |
|------|------|
| 当前文件 | `{dir}/latest.log`（固定文件名） |
| 归档命名 | `log-yyyy-MM-dd-001`，同日递增为 `002`、`003`… |
| 体积触发 | `latest.log` ≥ `max_size_mb` |
| 时间触发 | 本地时区每日 0 点（空文件跳过） |
| 压缩 | `compress: true` 时生成 `log-yyyy-MM-dd-NNN.gz` |

## API

| 符号 | 作用 |
|------|------|
| `RegisterScope` / `(*Scope).RegisterModule` | 注册 |
| `Init` / `Close` | 生命周期 |
| `(*Logger).Debug` / `Info` / `Warn` / `Error` | 日志输出 |
| `(*Logger).Writer` | 供第三方库使用的 `io.Writer` |
| `Config` | 配置结构 |
| `Version` | 版本号常量 |

## 参与贡献

欢迎在 [GitHub](https://github.com/wifi504/logo) 提交 Issue 与 Pull Request。提交前请运行 `go test ./...`。

## 许可证

[MIT](LICENSE) © WIFI连接超时
