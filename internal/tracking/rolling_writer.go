package tracking

import (
	"errors"
	"fmt"
	"os"
	"sync"
)

// rollingWriter rotates path by size, retaining path.1 through path.N.
type rollingWriter struct {
	mu         sync.Mutex
	path       string
	maxSize    int64
	maxBackups int
	file       *os.File
	size       int64
}

func newRollingWriter(path string, maxSize int64, maxBackups int) (*rollingWriter, error) {
	if maxSize <= 0 {
		return nil, fmt.Errorf("max log size must be positive")
	}
	w := &rollingWriter{path: path, maxSize: maxSize, maxBackups: maxBackups}
	if err := w.open(); err != nil {
		return nil, err
	}
	return w, nil
}

func (w *rollingWriter) open() error {
	f, err := os.OpenFile(w.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	info, err := f.Stat()
	if err != nil {
		f.Close()
		return err
	}
	w.file = f
	w.size = info.Size()
	return nil
}

func (w *rollingWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.file == nil {
		return 0, os.ErrClosed
	}
	if w.size > 0 && w.size+int64(len(p)) > w.maxSize {
		if err := w.rotate(); err != nil {
			// Rotation should never take logging down. rotate attempts to reopen
			// the active file before returning an error, so preserve this entry.
			if w.file == nil {
				return 0, err
			}
		}
	}
	n, err := w.file.Write(p)
	w.size += int64(n)
	return n, err
}

func (w *rollingWriter) rotate() error {
	if err := w.file.Close(); err != nil {
		return err
	}
	w.file = nil

	if w.maxBackups > 0 {
		if err := os.Remove(fmt.Sprintf("%s.%d", w.path, w.maxBackups)); err != nil && !os.IsNotExist(err) {
			_ = w.open()
			return err
		}
		for i := w.maxBackups - 1; i >= 1; i-- {
			from := fmt.Sprintf("%s.%d", w.path, i)
			to := fmt.Sprintf("%s.%d", w.path, i+1)
			if err := os.Rename(from, to); err != nil && !os.IsNotExist(err) {
				_ = w.open()
				return err
			}
		}
		if err := os.Rename(w.path, w.path+".1"); err != nil && !os.IsNotExist(err) {
			_ = w.open()
			return err
		}
	} else if err := os.Remove(w.path); err != nil && !os.IsNotExist(err) {
		_ = w.open()
		return err
	}
	return w.open()
}

func (w *rollingWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.file == nil {
		return nil
	}
	err := errors.Join(w.file.Sync(), w.file.Close())
	w.file = nil
	return err
}
