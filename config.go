package logo

// Config is the logs configuration section expected by the framework.
// Callers load YAML (or other sources) into this struct and pass it to Init.
type Config struct {
	// Level is the global default level: debug | info | warn | error.
	Level string `yaml:"level"`
	// Dir is the directory for latest.log and archives.
	Dir string `yaml:"dir"`
	// MaxSizeMB rotates when latest.log reaches this size (megabytes).
	MaxSizeMB int `yaml:"max_size_mb"`
	// Compress gzip-compresses archives after rotation.
	Compress bool `yaml:"compress"`
	// MaxBackups is the max number of archive files to keep.
	MaxBackups int `yaml:"max_backups"`
	// MaxAgeDays deletes archives older than this many days; 0 disables age-based deletion.
	MaxAgeDays int `yaml:"max_age_days"`
	// Stdout also writes log lines to standard output.
	Stdout bool `yaml:"stdout"`
	// Scopes holds per-scope level overrides.
	Scopes map[string]ScopeConfig `yaml:"scopes"`
}

// ScopeConfig configures one scope.
type ScopeConfig struct {
	Level   string                  `yaml:"level"`
	Modules map[string]ModuleConfig `yaml:"modules"`
}

// ModuleConfig configures one module under a scope.
type ModuleConfig struct {
	Level string `yaml:"level"`
}

func (c Config) withDefaults() Config {
	if c.Level == "" {
		c.Level = "info"
	}
	if c.Dir == "" {
		c.Dir = "logs"
	}
	if c.MaxSizeMB <= 0 {
		c.MaxSizeMB = 10
	}
	if c.MaxBackups <= 0 {
		c.MaxBackups = 30
	}
	if c.Scopes == nil {
		c.Scopes = map[string]ScopeConfig{}
	}
	return c
}
