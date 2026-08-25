package logo

import (
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

const latestFileName = "latest.log"

var archiveNameRe = regexp.MustCompile(`^log-(\d{4}-\d{2}-\d{2})-(\d{3})(\.gz)?$`)

type fileRotator struct {
	dir        string
	maxSize    int64
	compress   bool
	maxBackups int
	maxAgeDays int
	file       *os.File
	size       int64
}

func newFileRotator(cfg Config) (*fileRotator, error) {
	if err := os.MkdirAll(cfg.Dir, 0o755); err != nil {
		return nil, fmt.Errorf("logo: mkdir %s: %w", cfg.Dir, err)
	}
	r := &fileRotator{
		dir:        cfg.Dir,
		maxSize:    int64(cfg.MaxSizeMB) * 1024 * 1024,
		compress:   cfg.Compress,
		maxBackups: cfg.MaxBackups,
		maxAgeDays: cfg.MaxAgeDays,
	}
	if err := r.openLatest(); err != nil {
		return nil, err
	}
	return r, nil
}

func (r *fileRotator) latestPath() string {
	return filepath.Join(r.dir, latestFileName)
}

func (r *fileRotator) openLatest() error {
	path := r.latestPath()
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return fmt.Errorf("logo: open %s: %w", path, err)
	}
	fi, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return err
	}
	r.file = f
	r.size = fi.Size()
	return nil
}

func (r *fileRotator) Write(p []byte) (int, error) {
	return r.WriteCheckRotate(p)
}

func (r *fileRotator) WriteCheckRotate(p []byte) (int, error) {
	if r.file == nil {
		if err := r.openLatest(); err != nil {
			return 0, err
		}
	}
	n, err := r.file.Write(p)
	r.size += int64(n)
	if err != nil {
		return n, err
	}
	if r.size >= r.maxSize {
		if rotErr := r.Rotate(); rotErr != nil {
			return n, rotErr
		}
	}
	return n, nil
}

// Rotate archives latest.log as log-yyyy-MM-dd-NNN[.gz] and opens a new latest.log.
// Skips when latest.log is empty.
func (r *fileRotator) Rotate() error {
	if r.file != nil {
		_ = r.file.Sync()
		_ = r.file.Close()
		r.file = nil
	}

	path := r.latestPath()
	fi, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return r.openLatest()
		}
		return err
	}
	if fi.Size() == 0 {
		return r.openLatest()
	}

	day := time.Now().Format("2006-01-02")
	seq, err := nextArchiveSeq(r.dir, day)
	if err != nil {
		return err
	}
	base := fmt.Sprintf("log-%s-%03d", day, seq)
	dest := filepath.Join(r.dir, base)

	if err := os.Rename(path, dest); err != nil {
		return fmt.Errorf("logo: rename to %s: %w", dest, err)
	}

	if r.compress {
		gzPath := dest + ".gz"
		if err := gzipFile(dest, gzPath); err != nil {
			return err
		}
		_ = os.Remove(dest)
	}

	if err := r.cleanup(); err != nil {
		// best-effort
		fmt.Fprintf(os.Stderr, "logo: cleanup: %v\n", err)
	}
	return r.openLatest()
}

func nextArchiveSeq(dir, day string) (int, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 1, err
	}
	maxSeq := 0
	prefix := "log-" + day + "-"
	for _, e := range entries {
		name := e.Name()
		if !strings.HasPrefix(name, prefix) {
			continue
		}
		m := archiveNameRe.FindStringSubmatch(name)
		if m == nil {
			continue
		}
		if m[1] != day {
			continue
		}
		n, err := strconv.Atoi(m[2])
		if err != nil {
			continue
		}
		if n > maxSeq {
			maxSeq = n
		}
	}
	return maxSeq + 1, nil
}

func gzipFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer func() { _ = out.Close() }()

	zw := gzip.NewWriter(out)
	zw.Name = filepath.Base(src)
	if _, err := io.Copy(zw, in); err != nil {
		_ = zw.Close()
		return err
	}
	if err := zw.Close(); err != nil {
		return err
	}
	return out.Close()
}

func (r *fileRotator) cleanup() error {
	entries, err := os.ReadDir(r.dir)
	if err != nil {
		return err
	}
	type arch struct {
		name    string
		modTime time.Time
	}
	var list []arch
	for _, e := range entries {
		name := e.Name()
		if !archiveNameRe.MatchString(name) {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		list = append(list, arch{name: name, modTime: info.ModTime()})
	}
	sort.Slice(list, func(i, j int) bool {
		return list[i].modTime.After(list[j].modTime)
	})

	now := time.Now()
	keep := 0
	for _, a := range list {
		if r.maxAgeDays > 0 && now.Sub(a.modTime) > time.Duration(r.maxAgeDays)*24*time.Hour {
			_ = os.Remove(filepath.Join(r.dir, a.name))
			continue
		}
		keep++
		if r.maxBackups > 0 && keep > r.maxBackups {
			_ = os.Remove(filepath.Join(r.dir, a.name))
		}
	}
	return nil
}

func (r *fileRotator) Close() error {
	if r.file == nil {
		return nil
	}
	err := r.file.Close()
	r.file = nil
	return err
}
