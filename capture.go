package logo

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
)

const (
	unconfiguredScope  = "未配置Logo域和模块"
	unconfiguredModule = "unknown"
)

var (
	realStdout = os.Stdout
	realStderr = os.Stderr

	captureMu   sync.Mutex
	captureStop chan struct{}
	captureWG   sync.WaitGroup
	capturing   bool
	stdoutPipeW *os.File
	stderrPipeW *os.File

	earlyMu  sync.Mutex
	earlyBuf []earlyLine
)

type earlyLine struct {
	level  Level
	source string
	msg    string
}

func underGoTest() bool {
	for _, a := range os.Args {
		if strings.HasPrefix(a, "-test.") {
			return true
		}
	}
	return false
}

func init() {
	// 尽早劫持；go test 下跳过，避免污染测试框架输出
	if underGoTest() {
		return
	}
	_ = ensureCapture()
}

func ensureCapture() error {
	captureMu.Lock()
	defer captureMu.Unlock()
	if capturing {
		rebindStdLogLocked()
		return nil
	}

	outR, outW, err := os.Pipe()
	if err != nil {
		return fmt.Errorf("logo: stdout pipe: %w", err)
	}
	errR, errW, err := os.Pipe()
	if err != nil {
		_ = outR.Close()
		_ = outW.Close()
		return fmt.Errorf("logo: stderr pipe: %w", err)
	}

	os.Stdout = outW
	os.Stderr = errW
	stdoutPipeW = outW
	stderrPipeW = errW
	captureStop = make(chan struct{})
	capturing = true

	captureWG.Add(2)
	go drainPipe(outR, "stdout", LevelWarn, captureStop, &captureWG)
	go drainPipe(errR, "stderr", LevelError, captureStop, &captureWG)

	rebindStdLogLocked()
	return nil
}

// rebindStdLogLocked points the default logger at the current os.Stderr (pipe).
// Call while holding captureMu or after stdout/stderr replacement.
func rebindStdLogLocked() {
	log.SetOutput(os.Stderr)
	log.SetFlags(log.LstdFlags)
}

func stopCapture() {
	captureMu.Lock()
	if !capturing {
		captureMu.Unlock()
		return
	}
	stop := captureStop
	outW := stdoutPipeW
	errW := stderrPipeW
	capturing = false
	captureStop = nil
	stdoutPipeW = nil
	stderrPipeW = nil
	captureMu.Unlock()

	os.Stdout = realStdout
	os.Stderr = realStderr
	log.SetOutput(os.Stderr)

	if outW != nil {
		_ = outW.Close()
	}
	if errW != nil {
		_ = errW.Close()
	}
	if stop != nil {
		close(stop)
	}
	captureWG.Wait()
}

func drainPipe(r *os.File, source string, level Level, stop <-chan struct{}, wg *sync.WaitGroup) {
	defer wg.Done()
	defer r.Close()

	done := make(chan struct{})
	go func() {
		defer close(done)
		sc := bufio.NewScanner(r)
		buf := make([]byte, 0, 64*1024)
		sc.Buffer(buf, 1024*1024)
		for sc.Scan() {
			line := strings.TrimRight(sc.Text(), "\r")
			if line == "" {
				continue
			}
			handleCaptured(level, source, line)
		}
	}()

	select {
	case <-stop:
		_ = r.Close()
		<-done
	case <-done:
	}
}

func handleCaptured(level Level, source, msg string) {
	cfgMu.RLock()
	ready := inited
	cfgMu.RUnlock()

	line := formatLogLine(unconfiguredScope, level.String(), unconfiguredModule, source, msg)

	if !ready {
		earlyMu.Lock()
		earlyBuf = append(earlyBuf, earlyLine{level: level, source: source, msg: msg})
		earlyMu.Unlock()
		// Init 前先打到真实控制台，避免启动期输出丢失
		_, _ = realStdout.Write([]byte(line))
		return
	}
	emitRaw(line)
}

func flushEarlyBuffer() {
	earlyMu.Lock()
	buf := earlyBuf
	earlyBuf = nil
	earlyMu.Unlock()
	if len(buf) == 0 {
		return
	}
	writeMu.Lock()
	defer writeMu.Unlock()
	for _, e := range buf {
		line := formatLogLine(unconfiguredScope, e.level.String(), unconfiguredModule, e.source, e.msg)
		if rotator != nil {
			_, _ = rotator.WriteCheckRotate([]byte(line))
		}
	}
}
