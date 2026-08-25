package logo

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLevelFiltering(t *testing.T) {
	dir := t.TempDir()
	app := RegisterScope("testapp")
	mod := app.RegisterModule("svc")

	if err := Init(Config{
		Level:      "warn",
		Dir:        dir,
		MaxSizeMB:  10,
		Stdout:     false,
		CaptureStd: Bool(false),
		Scopes: map[string]ScopeConfig{
			"testapp": {
				Modules: map[string]ModuleConfig{
					"svc": {Level: "info"},
				},
			},
		},
	}); err != nil {
		t.Fatal(err)
	}
	defer Close()

	mod.Debug("hidden")
	mod.Info("visible-info")
	mod.Warn("visible-warn")

	data, err := os.ReadFile(filepath.Join(dir, "latest.log"))
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)
	if strings.Contains(s, "hidden") {
		t.Fatalf("debug should be filtered: %s", s)
	}
	if !strings.Contains(s, "visible-info") || !strings.Contains(s, "visible-warn") {
		t.Fatalf("expected info/warn lines: %s", s)
	}
	if !strings.Contains(s, "[testapp][INFO][svc]") {
		t.Fatalf("bad format: %s", s)
	}
	if !strings.Contains(s, "Logo Framework "+Version) {
		t.Fatalf("banner missing version: %s", s)
	}
}

func TestArchiveSeq(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "log-2099-01-02-001"), []byte("a"), 0o644)
	_ = os.WriteFile(filepath.Join(dir, "log-2099-01-02-002.gz"), []byte("b"), 0o644)
	n, err := nextArchiveSeq(dir, "2099-01-02")
	if err != nil {
		t.Fatal(err)
	}
	if n != 3 {
		t.Fatalf("got %d want 3", n)
	}
}

func TestRotateCreatesArchive(t *testing.T) {
	dir := t.TempDir()
	app := RegisterScope("rotapp")
	mod := app.RegisterModule("rotmod")

	if err := Init(Config{
		Level:      "debug",
		Dir:        dir,
		MaxSizeMB:  1,
		Compress:   false,
		MaxBackups: 10,
		Stdout:     false,
		CaptureStd: Bool(false),
	}); err != nil {
		t.Fatal(err)
	}
	defer Close()

	mod.Info("before-rotate")
	writeMu.Lock()
	err := rotator.Rotate()
	writeMu.Unlock()
	if err != nil {
		t.Fatal(err)
	}

	entries, _ := os.ReadDir(dir)
	found := false
	for _, e := range entries {
		if archiveNameRe.MatchString(e.Name()) {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("no archive created, entries=%v", entries)
	}
	if _, err := os.Stat(filepath.Join(dir, "latest.log")); err != nil {
		t.Fatal(err)
	}
}

func TestCaptureStdout(t *testing.T) {
	dir := t.TempDir()
	_ = RegisterScope("capapp").RegisterModule("capmod")

	if err := Init(Config{
		Level:      "debug",
		Dir:        dir,
		MaxSizeMB:  10,
		Stdout:     false,
		CaptureStd: Bool(true),
	}); err != nil {
		t.Fatal(err)
	}
	defer Close()

	fmt.Println("bare-stdout-line")
	time.Sleep(50 * time.Millisecond)

	data, err := os.ReadFile(filepath.Join(dir, "latest.log"))
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)
	if !strings.Contains(s, "[未配置Logo域和模块][WARN][unknown] stdout : bare-stdout-line") {
		t.Fatalf("captured stdout missing: %s", s)
	}
}
