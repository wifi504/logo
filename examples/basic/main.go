package main

import (
	"fmt"

	"github.com/wifi504/logo"
)

func main() {
	app := logo.RegisterScope("app")
	serverLog := app.RegisterModule("server")

	if err := logo.Init(logo.Config{
		Level:      "debug",
		Dir:        "logs",
		MaxSizeMB:  10,
		Compress:   true,
		Stdout:     true,
		CaptureStd: logo.Bool(true),
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
	fmt.Println("this goes through capture as WARN/unknown/stdout")
}
