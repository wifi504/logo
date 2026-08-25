package logo

import (
	"fmt"
	"strings"
	"sync"
)

// Level is a log severity.
type Level int

const (
	LevelDebug Level = iota
	LevelInfo
	LevelWarn
	LevelError
)

func parseLevel(s string) (Level, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "debug":
		return LevelDebug, nil
	case "info", "":
		return LevelInfo, nil
	case "warn", "warning":
		return LevelWarn, nil
	case "error":
		return LevelError, nil
	default:
		return LevelInfo, fmt.Errorf("logo: unknown level %q", s)
	}
}

func (l Level) String() string {
	switch l {
	case LevelDebug:
		return "debug"
	case LevelInfo:
		return "info"
	case LevelWarn:
		return "warn"
	case LevelError:
		return "error"
	default:
		return "info"
	}
}

// Scope is a registered logging domain (e.g. "app", "gin").
type Scope struct {
	name string
}

// Logger is a registered module logger under a scope.
type Logger struct {
	scope  string
	module string
}

var (
	regMu      sync.RWMutex
	scopes     = map[string]*Scope{}
	modules    = map[string]*Logger{} // key: scope/module
	inited     bool
	cfgMu      sync.RWMutex
	runtimeCfg Config
	levelCache sync.Map // key scope/module -> Level threshold
)

func moduleKey(scope, module string) string {
	return scope + "/" + module
}

// RegisterScope registers a scope. Must be called before Init (or at least before logging).
func RegisterScope(name string) *Scope {
	name = strings.TrimSpace(name)
	if name == "" {
		panic("logo: scope name must not be empty")
	}
	regMu.Lock()
	defer regMu.Unlock()
	if s, ok := scopes[name]; ok {
		return s
	}
	s := &Scope{name: name}
	scopes[name] = s
	return s
}

// RegisterModule registers a module under the scope and returns its Logger.
func (s *Scope) RegisterModule(name string) *Logger {
	if s == nil || s.name == "" {
		panic("logo: nil scope")
	}
	name = strings.TrimSpace(name)
	if name == "" {
		panic("logo: module name must not be empty")
	}
	key := moduleKey(s.name, name)
	regMu.Lock()
	defer regMu.Unlock()
	if l, ok := modules[key]; ok {
		return l
	}
	l := &Logger{scope: s.name, module: name}
	modules[key] = l
	return l
}

// ScopeName returns the scope name.
func (l *Logger) ScopeName() string { return l.scope }

// ModuleName returns the module name.
func (l *Logger) ModuleName() string { return l.module }

func effectiveLevel(scope, module string) Level {
	cfgMu.RLock()
	cfg := runtimeCfg
	cfgMu.RUnlock()

	if key := moduleKey(scope, module); true {
		if v, ok := levelCache.Load(key); ok {
			return v.(Level)
		}
	}

	global, _ := parseLevel(cfg.Level)
	threshold := global

	if sc, ok := cfg.Scopes[scope]; ok {
		if sc.Level != "" {
			if lv, err := parseLevel(sc.Level); err == nil {
				threshold = lv
			}
		}
		if mc, ok := sc.Modules[module]; ok && mc.Level != "" {
			if lv, err := parseLevel(mc.Level); err == nil {
				threshold = lv
			}
		}
	}

	levelCache.Store(moduleKey(scope, module), threshold)
	return threshold
}

func rebuildLevelCache() {
	levelCache = sync.Map{}
}

func isRegistered(scope, module string) bool {
	regMu.RLock()
	defer regMu.RUnlock()
	_, ok := modules[moduleKey(scope, module)]
	return ok
}
