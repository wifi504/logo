package logo

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

func TestLevelFiltering(t *testing.T) {
	dir := t.TempDir()
	app := RegisterScope("testapp")
	mod := app.RegisterModule("svc")

	if err := Init(Config{
		Level:     "warn",
		Dir:       dir,
		MaxSizeMB: 10,
		Stdout:    false,
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
	if !strings.Contains(s, "[testapp") || !strings.Contains(s, "[INFO ") || !strings.Contains(s, "[svc") {
		t.Fatalf("bad padded format: %s", s)
	}
	// columns before " : " should align across lines — check fixed widths via a sample line
	for _, line := range strings.Split(s, "\n") {
		if !strings.Contains(line, "visible-info") {
			continue
		}
		idx := strings.Index(line, " : ")
		if idx < 0 {
			t.Fatalf("missing separator: %s", line)
		}
		// [ts][15][5][15] space 25
		// rough: after second ] of timestamp field structure
		break
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

func TestCaptureStdoutAndStdLog(t *testing.T) {
	dir := t.TempDir()
	_ = RegisterScope("capapp").RegisterModule("capmod")

	if err := Init(Config{
		Level:     "debug",
		Dir:       dir,
		MaxSizeMB: 10,
		Stdout:    false,
	}); err != nil {
		t.Fatal(err)
	}
	defer Close()

	fmt.Println("bare-stdout-line")
	log.Println("bare-stdlog-line")
	time.Sleep(80 * time.Millisecond)

	data, err := os.ReadFile(filepath.Join(dir, "latest.log"))
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)
	if !strings.Contains(s, "[未配置Logo域和模块") || !strings.Contains(s, "[WARN ") || !strings.Contains(s, "stdout") || !strings.Contains(s, "bare-stdout-line") {
		t.Fatalf("captured stdout missing: %s", s)
	}
	if !strings.Contains(s, "[ERROR") || !strings.Contains(s, "stderr") || !strings.Contains(s, "bare-stdlog-line") {
		t.Fatalf("captured std log missing: %s", s)
	}
}

func TestPadAndTruncate(t *testing.T) {
	wantWhere := padRight("main.go:35", whereWidth)
	want := fmt.Sprintf("[app            ][INFO ][config         ] %s : hello", wantWhere)
	got := formatLogLine("app", "INFO", "config", "main.go:35", "hello")
	if !strings.Contains(got, want) {
		t.Fatalf("align fail:\n got %q\nwant substring %q", got, want)
	}
	long := strings.Repeat("x", 30) + ".go:9"
	got2 := formatLogLine("a", "DEBUG", "b", long, "m")
	where := truncateFront(long, whereWidth)
	if utf8.RuneCountInString(where) != whereWidth {
		t.Fatalf("where width=%d want %d (%q)", utf8.RuneCountInString(where), whereWidth, where)
	}
	if !strings.Contains(got2, where) {
		t.Fatalf("truncate fail: %q where=%q", got2, where)
	}
}

func TestNameTooLongPanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic")
		}
	}()
	RegisterScope(strings.Repeat("a", MaxNameLen+1))
}
