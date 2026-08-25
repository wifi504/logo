package logo

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

var (
	writeMu   sync.Mutex
	outWriter io.Writer = os.Stderr
	rotator   *fileRotator
	stopCh    chan struct{}
	closeOnce sync.Once
)

// Init applies configuration, opens latest.log, and starts midnight rotation.
// Scopes/modules should already be registered. Safe to call once; subsequent calls Close then re-init.
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
	if cfg.Stdout {
		outWriter = io.MultiWriter(r, os.Stdout)
	} else {
		outWriter = r
	}
	cfgMu.Lock()
	runtimeCfg = cfg
	rebuildLevelCache()
	inited = true
	cfgMu.Unlock()
	stopCh = make(chan struct{})
	writeMu.Unlock()

	go midnightLoop(stopCh)

	warnUnknownConfigKeys(cfg)
	return nil
}

func warnUnknownConfigKeys(cfg Config) {
	regMu.RLock()
	defer regMu.RUnlock()
	for sn, sc := range cfg.Scopes {
		if _, ok := scopes[sn]; !ok {
			fmt.Fprintf(os.Stderr, "logo: config has unregistered scope %q (ignored for logging, levels unused)\n", sn)
			continue
		}
		for mn := range sc.Modules {
			if _, ok := modules[moduleKey(sn, mn)]; !ok {
				fmt.Fprintf(os.Stderr, "logo: config has unregistered module %s/%s\n", sn, mn)
			}
		}
	}
}

// Close stops the midnight rotator and closes the log file.
func Close() {
	closeOnce.Do(func() {
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
		outWriter = os.Stderr
		inited = false
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

func (l *Logger) logf(level Level, format string, args ...any) {
	if l == nil {
		return
	}
	if !isRegistered(l.scope, l.module) {
		fmt.Fprintf(os.Stderr, "logo: logger %s/%s is not registered\n", l.scope, l.module)
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

	now := time.Now()
	lineOut := fmt.Sprintf("[%s %03d] [%s] [%s] [%s] %s:%d : %s\n",
		now.Format("2006-01-02 15:04:05"),
		now.Nanosecond()/1e6,
		l.scope,
		level.String(),
		l.module,
		file,
		line,
		msg,
	)

	writeMu.Lock()
	defer writeMu.Unlock()
	cfgMu.RLock()
	toStdout := inited && runtimeCfg.Stdout
	cfgMu.RUnlock()

	if rotator != nil {
		if _, err := rotator.WriteCheckRotate([]byte(lineOut)); err != nil {
			fmt.Fprintf(os.Stderr, "logo: write: %v\n", err)
		}
		if toStdout {
			_, _ = os.Stdout.Write([]byte(lineOut))
		}
		return
	}
	_, _ = io.WriteString(outWriter, lineOut)
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
// Useful for bridging third-party libraries (e.g. Gin DefaultWriter).
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
			// depth: Writer -> logf needs Caller(2) from Debug/Info; use direct logf with adjust
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
	now := time.Now()
	lineOut := fmt.Sprintf("[%s %03d] [%s] [%s] [%s] %s:%d : %s\n",
		now.Format("2006-01-02 15:04:05"),
		now.Nanosecond()/1e6,
		l.scope,
		level.String(),
		l.module,
		file,
		line,
		msg,
	)
	writeMu.Lock()
	defer writeMu.Unlock()
	cfgMu.RLock()
	toStdout := inited && runtimeCfg.Stdout
	cfgMu.RUnlock()
	if rotator != nil {
		_, _ = rotator.WriteCheckRotate([]byte(lineOut))
		if toStdout {
			_, _ = os.Stdout.Write([]byte(lineOut))
		}
		return
	}
	_, _ = io.WriteString(outWriter, lineOut)
}
