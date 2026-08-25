package logo

import (
	"bufio"
	"fmt"
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
)

func startCapture() error {
	captureMu.Lock()
	defer captureMu.Unlock()
	if capturing {
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
	return nil
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
			emitRaw(formatLogLine(unconfiguredScope, level.String(), unconfiguredModule, source, line))
		}
	}()

	select {
	case <-stop:
		_ = r.Close()
		<-done
	case <-done:
	}
}
