package vm

import (
	"bufio"
	"io"
	"strings"
	"sync"

	"github.com/sirupsen/logrus"
)

const Sentinel = "===DOCK-FIRE-OUTPUT-START==="

// FilteredWriter wraps a writer and discards everything before the sentinel line.
type FilteredWriter struct {
	out     io.Writer
	mu      sync.Mutex
	started bool
}

func NewFilteredWriter(out io.Writer) *FilteredWriter {
	return &FilteredWriter{out: out}
}

func (f *FilteredWriter) Write(p []byte) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.started {
		return f.out.Write(p)
	}

	// Scan for sentinel in the incoming data
	scanner := bufio.NewScanner(strings.NewReader(string(p)))
	var after []byte
	found := false
	for scanner.Scan() {
		line := scanner.Text()
		if found {
			after = append(after, []byte(line+"\n")...)
		} else if strings.TrimSpace(line) == Sentinel {
			found = true
			f.started = true
		}
	}

	if found && len(after) > 0 {
		return f.out.Write(after)
	}

	// Return len(p) even though we discarded, so the caller doesn't see an error
	return len(p), nil
}

// ReadFilteredOutput reads a VM stdout log file and writes only the content after the sentinel.
func ReadFilteredOutput(r io.Reader, w io.Writer) error {
	scanner := bufio.NewScanner(r)
	started := false

	for scanner.Scan() {
		line := scanner.Text()
		if !started {
			if strings.TrimSpace(line) == Sentinel {
				started = true
			}
			continue
		}
		if _, err := io.WriteString(w, line+"\n"); err != nil {
			return err
		}
	}

	if err := scanner.Err(); err != nil {
		logrus.Debugf("scanner error: %v", err)
	}
	return nil
}
