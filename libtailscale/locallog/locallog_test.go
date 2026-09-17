// Copyright (c) Tailscale Inc & AUTHORS
// SPDX-License-Identifier: BSD-3-Clause

package locallog

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
)

// readAll concatenates the on-disk ring files in dir.
func readAll(t *testing.T, dir string) string {
	t.Helper()
	var sb strings.Builder
	for _, name := range FileNames() {
		b, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil && !os.IsNotExist(err) {
			t.Fatal(err)
		}
		sb.Write(b)
	}
	return sb.String()
}

func TestWritePrefixesTimestampAndTerminatesLine(t *testing.T) {
	dir := t.TempDir()
	w, err := New(dir, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()

	n, err := w.Write([]byte("hello world"))
	if err != nil || n != len("hello world") {
		t.Fatalf("Write = %d, %v; want %d, nil", n, err, len("hello world"))
	}

	got := readAll(t, dir)
	re := regexp.MustCompile(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}\.\d{6}Z hello world\n$`)
	if !re.MatchString(got) {
		t.Fatalf("file content = %q; want timestamp-prefixed line", got)
	}
}

func TestContentSurvivesReopen(t *testing.T) {
	dir := t.TempDir()
	w, err := New(dir, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	w.Write([]byte("first run\n"))
	w.Close()

	w2, err := New(dir, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	defer w2.Close()
	w2.Write([]byte("second run\n"))

	got := readAll(t, dir)
	if !strings.Contains(got, "first run") || !strings.Contains(got, "second run") {
		t.Fatalf("content after reopen = %q; want both runs", got)
	}
}

func TestRotationBoundsDiskUsage(t *testing.T) {
	dir := t.TempDir()
	const maxBytes = 4096
	w, err := New(dir, maxBytes)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()

	line := strings.Repeat("x", 100) + "\n"
	for range 500 { // ~50 KiB, far past the cap
		w.Write([]byte(line))
	}

	var total int64
	for _, name := range FileNames() {
		fi, err := os.Stat(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		total += fi.Size()
	}
	// filch splits maxBytes across its two files, so the ring holds between
	// maxBytes/2 (right after a rotation) and maxBytes (just before one).
	if total > int64(maxBytes+2*len(line)+64) {
		t.Fatalf("disk usage %d bytes; want ≤ ~%d", total, maxBytes)
	}
	if total < maxBytes/2 {
		t.Fatalf("disk usage %d bytes; the newest lines were lost", total)
	}
	if !strings.HasSuffix(readAll(t, dir), line) {
		t.Fatal("the most recent line is not the last one on disk")
	}
}

func TestOverlongWriteIsTruncatedNotDropped(t *testing.T) {
	dir := t.TempDir()
	w, err := New(dir, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()

	p := bytes.Repeat([]byte("y"), 100<<10) // 100 KiB, over filch's 64 KiB line cap
	n, err := w.Write(p)
	if err != nil || n != len(p) {
		t.Fatalf("Write = %d, %v; want %d, nil", n, err, len(p))
	}
	got := readAll(t, dir)
	if !strings.Contains(got, "yyyy") {
		t.Fatalf("overlong line was dropped entirely; file = %q", got)
	}
	if strings.Count(got, "\n") != 1 {
		t.Fatalf("expected exactly one line on disk, got %d", strings.Count(got, "\n"))
	}
}

func TestConcurrentWritesDoNotInterleave(t *testing.T) {
	// The log package and the Kotlin log forwarder write from different
	// goroutines; run under -race to catch unsynchronized buffer reuse.
	dir := t.TempDir()
	w, err := New(dir, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()

	const writers, perWriter = 8, 200
	var wg sync.WaitGroup
	for i := range writers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			line := []byte(strings.Repeat(string(rune('a'+i)), 40) + "\n")
			for range perWriter {
				w.Write(line)
			}
		}()
	}
	wg.Wait()

	lines := strings.Split(strings.TrimSuffix(readAll(t, dir), "\n"), "\n")
	if len(lines) != writers*perWriter {
		t.Fatalf("got %d lines; want %d", len(lines), writers*perWriter)
	}
	for _, l := range lines {
		body := l[len("2006-01-02T15:04:05.000000Z "):]
		if len(body) != 40 || strings.Count(body, body[:1]) != 40 {
			t.Fatalf("interleaved or corrupted line: %q", l)
		}
	}
}

func TestTeeWithoutDirReturnsWriterUnchanged(t *testing.T) {
	var buf bytes.Buffer
	if got := Tee("", &buf); got != io.Writer(&buf) {
		t.Fatalf("Tee(\"\", w) = %T; want w itself", got)
	}
}

func TestTeeWritesToBothSinks(t *testing.T) {
	dir := t.TempDir()
	var buf bytes.Buffer
	out := Tee(dir, &buf)
	if out == io.Writer(&buf) {
		t.Fatal("Tee with a usable dir returned the primary writer alone")
	}
	io.WriteString(out, "both\n")
	if buf.String() != "both\n" {
		t.Fatalf("primary got %q", buf.String())
	}
	if got := readAll(t, dir); !strings.HasSuffix(got, " both\n") {
		t.Fatalf("local file got %q", got)
	}
}

func TestFileNames(t *testing.T) {
	want := []string{"local.log1.txt", "local.log2.txt"}
	got := FileNames()
	if len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("FileNames() = %v; want %v", got, want)
	}
}
