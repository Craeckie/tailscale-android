// Copyright (c) Tailscale Inc & AUTHORS
// SPDX-License-Identifier: BSD-3-Clause

// Package locallog keeps a bounded, on-disk copy of the backend's log
// lines that does not depend on logtail's upload switch.
//
// logtail drops entries before buffering them when uploads are disabled,
// so with remote logging off its filch files stay empty. This package
// writes the same lines into its own two-file ring under the app data dir
// so the Kotlin side (LogExport.kt) always has something to export.
package locallog

import (
	"io"
	"log"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"tailscale.com/logtail/filch"
)

// prefix is the filch file prefix; filch derives the two ring files from it.
const prefix = "local"

// maxLineSize mirrors filch.DefaultMaxLineSize; longer writes are truncated
// rather than rejected.
const maxLineSize = filch.DefaultMaxLineSize

// FileNames returns the names of the two ring files under the directory
// passed to New. Which one holds the older content varies; readers should
// order them by mtime. The Kotlin exporter (LogExport.kt) mirrors this
// list; keep both in sync.
func FileNames() []string {
	return []string{prefix + ".log1.txt", prefix + ".log2.txt"}
}

// Writer is an io.Writer that appends timestamped lines to a filch ring.
// Write never returns an error: a broken local file must not break logging.
type Writer struct {
	f      *filch.Filch
	errors atomic.Int64

	mu  sync.Mutex // guards buf; the log package and the onLog forwarder write concurrently
	buf []byte     // owned by Write for reuse
}

// New opens (or creates) the ring under dir. maxBytes is the total for both
// files; together they hold between maxBytes/2 and maxBytes of the most
// recent output.
func New(dir string, maxBytes int64) (*Writer, error) {
	f, err := filch.New(filepath.Join(dir, prefix), filch.Options{
		MaxFileSize: int(maxBytes),
	})
	if err != nil {
		return nil, err
	}
	return &Writer{f: f}, nil
}

// Write prefixes p with a UTC timestamp and appends it to the ring.
// It always reports len(p) bytes written and a nil error.
func (w *Writer) Write(p []byte) (int, error) {
	const stamp = "2006-01-02T15:04:05.000000Z "
	n := len(p)
	w.mu.Lock()
	defer w.mu.Unlock()
	line := time.Now().UTC().AppendFormat(w.buf[:0], stamp)
	room := maxLineSize - len(line) - 1 // leave space for the newline filch appends
	if len(p) > room {
		p = p[:room]
	}
	line = append(line, p...)
	if len(line) == 0 || line[len(line)-1] != '\n' {
		line = append(line, '\n')
	}
	w.buf = line
	if _, err := w.f.Write(line); err != nil {
		w.errors.Add(1)
	}
	return n, nil
}

// Errors reports how many writes the underlying ring rejected.
func (w *Writer) Errors() int64 { return w.errors.Load() }

// Close closes the ring files.
func (w *Writer) Close() error { return w.f.Close() }

// Tee returns a writer that sends everything to primary and, when dir is
// usable, also to a local ring in dir. On failure it logs once through
// primary and returns primary unchanged, so callers need no error path.
func Tee(dir string, primary io.Writer) io.Writer {
	if dir == "" {
		return primary
	}
	local, err := New(dir, 16<<20) // keeps the most recent 8–16 MiB
	if err != nil {
		log.New(primary, "", 0).Printf("locallog: disabled: %v", err)
		return primary
	}
	return io.MultiWriter(primary, local)
}
