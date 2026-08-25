package logo

import (
	"fmt"
	"io"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

var (
	writeMu   sync.Mutex
	rotator   *fileRotator
	stopCh    chan struct{}
	closeOnce sync.Once
)

// Init applies configuration, opens latest.log, ensures stdio capture, and prints the banner.
func Init(cfg Config) error {
	cfg = cfg.withDefaults()
	if _, err := parseLevel(cfg.Level); err != nil {
		return err
	}
	for sn, sc := range cfg.Scopes {
		if sc.Level != "" {
			if _, err := parseLevel(sc.Level); err != nil {
				return fmt.Errorf("logo: scope %q: %w", sn, err)
			}
		}
		for mn, mc := range sc.Modules {
			if mc.Level != "" {
				if _, err := parseLevel(mc.Level); err != nil {
					return fmt.Errorf("logo: scope %q module %q: %w", sn, mn, err)
				}
			}
		}
	}

	Close()

	r, err := newFileRotator(cfg)
	if err != nil {
		return err
	}

	writeMu.Lock()
	rotator = r
	cfgMu.Lock()
	runtimeCfg = cfg
	rebuildLevelCache()
	inited = true
	cfgMu.Unlock()
	stopCh = make(chan struct{})
	writeMu.Unlock()

	// 劫持为默认行为：包 init 已尽量提前；此处确保开启并重绑 log.Default
	if err := ensureCapture(); err != nil {
		Close()
		return err
	}
	flushEarlyBuffer()

	go midnightLoop(stopCh)

	banner := renderBanner()
	if !strings.HasSuffix(banner, "\n") {
		banner += "\n"
	}
	// 版本行后再空一行，与后续日志隔开
	banner += "\n"
	emitRaw(banner)

	warnUnknownConfigKeys(cfg)
	return nil
}

func warnUnknownConfigKeys(cfg Config) {
	regMu.RLock()
	defer regMu.RUnlock()
	for sn, sc := range cfg.Scopes {
		if _, ok := scopes[sn]; !ok {
			fmt.Fprintf(realStderr, "logo: config has unregistered scope %q (ignored for logging, levels unused)\n", sn)
			continue
		}
		for mn := range sc.Modules {
			if _, ok := modules[moduleKey(sn, mn)]; !ok {
				fmt.Fprintf(realStderr, "logo: config has unregistered module %s/%s\n", sn, mn)
			}
		}
	}
}

// Close stops capture, midnight rotation, and closes the log file.
func Close() {
	closeOnce.Do(func() {
		stopCapture()

		writeMu.Lock()
		ch := stopCh
		stopCh = nil
		writeMu.Unlock()
		if ch != nil {
			close(ch)
		}

		writeMu.Lock()
		if rotator != nil {
			_ = rotator.Close()
			rotator = nil
		}
		cfgMu.Lock()
		inited = false
		cfgMu.Unlock()
		writeMu.Unlock()
	})
	closeOnce = sync.Once{}
}

func midnightLoop(stop <-chan struct{}) {
	for {
		now := time.Now()
		next := time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, now.Location())
		timer := time.NewTimer(time.Until(next))
		select {
		case <-stop:
			timer.Stop()
			return
		case <-timer.C:
			writeMu.Lock()
			if rotator != nil {
				_ = rotator.Rotate()
			}
			writeMu.Unlock()
		}
	}
}

func formatTimestamp(t time.Time) string {
	return fmt.Sprintf("%s %03d", t.Format("2006-01-02 15:04:05"), t.Nanosecond()/1e6)
}

func formatLogLine(scope, level, module, where, msg string) string {
	return fmt.Sprintf("[%s][%s][%s][%s] %s : %s\n",
		formatTimestamp(time.Now()),
		scope,
		level,
		module,
		where,
		msg,
	)
}

// emitRaw writes bytes to the log file and optionally the real console (never the hijacked os.Stdout).
func emitRaw(s string) {
	b := []byte(s)
	writeMu.Lock()
	defer writeMu.Unlock()
	cfgMu.RLock()
	toStdout := inited && runtimeCfg.Stdout
	cfgMu.RUnlock()
	if rotator != nil {
		if _, err := rotator.WriteCheckRotate(b); err != nil {
			fmt.Fprintf(realStderr, "logo: write: %v\n", err)
		}
	}
	if toStdout {
		_, _ = realStdout.Write(b)
	} else if rotator == nil {
		_, _ = realStderr.Write(b)
	}
}

func (l *Logger) logf(level Level, format string, args ...any) {
	if l == nil {
		return
	}
	if !isRegistered(l.scope, l.module) {
		fmt.Fprintf(realStderr, "logo: logger %s/%s is not registered\n", l.scope, l.module)
		return
	}
	if level < effectiveLevel(l.scope, l.module) {
		return
	}

	_, file, line, ok := runtime.Caller(2)
	if !ok {
		file = "???"
		line = 0
	} else {
		file = filepath.Base(file)
	}

	msg := format
	if len(args) > 0 {
		msg = fmt.Sprintf(format, args...)
	}
	msg = strings.TrimRight(msg, "\r\n")
	where := fmt.Sprintf("%s:%d", file, line)
	emitRaw(formatLogLine(l.scope, level.String(), l.module, where, msg))
}

// Debug logs at debug level.
func (l *Logger) Debug(format string, args ...any) { l.logf(LevelDebug, format, args...) }

// Info logs at info level.
func (l *Logger) Info(format string, args ...any) { l.logf(LevelInfo, format, args...) }

// Warn logs at warn level.
func (l *Logger) Warn(format string, args ...any) { l.logf(LevelWarn, format, args...) }

// Error logs at error level.
func (l *Logger) Error(format string, args ...any) { l.logf(LevelError, format, args...) }

// Writer returns an io.Writer that logs each complete line at the given level.
func (l *Logger) Writer(level Level) io.Writer {
	return &lineWriter{logger: l, level: level}
}

type lineWriter struct {
	logger *Logger
	level  Level
	buf    []byte
	mu     sync.Mutex
}

func (w *lineWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.buf = append(w.buf, p...)
	for {
		i := -1
		for j, b := range w.buf {
			if b == '\n' {
				i = j
				break
			}
		}
		if i < 0 {
			break
		}
		line := string(w.buf[:i])
		w.buf = w.buf[i+1:]
		line = strings.TrimRight(line, "\r")
		if line != "" {
			w.logger.logfLine(w.level, line)
		}
	}
	return len(p), nil
}

func (l *Logger) logfLine(level Level, msg string) {
	if l == nil {
		return
	}
	if !isRegistered(l.scope, l.module) {
		return
	}
	if level < effectiveLevel(l.scope, l.module) {
		return
	}
	_, file, line, ok := runtime.Caller(3)
	if !ok {
		file = "???"
		line = 0
	} else {
		file = filepath.Base(file)
	}
	where := fmt.Sprintf("%s:%d", file, line)
	emitRaw(formatLogLine(l.scope, level.String(), l.module, where, msg))
}
