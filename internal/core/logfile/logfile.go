// Package logfile reads lines of a large, growing file on demand, without
// loading it into memory.
package logfile

import (
	"bytes"
	"os"
	"strings"
	"sync"
	"unicode/utf8"
)

const (
	// Stride is how many lines apart the index records an offset.
	Stride    = 256
	chunkSize = 1 << 20
	// MaxLineBytes caps how much of one line is returned for display.
	MaxLineBytes = 1 << 20
)

// File indexes a file in the background and reads line ranges with ReadAt.
// It never maps the file: a mapped file truncated by copytruncate raises
// SIGBUS on access past its new end.
type File struct {
	path       string
	onProgress func()

	mu       sync.Mutex
	f        *os.File
	gen      int
	marks    []int64 // marks[k] is the offset of line k*Stride
	lines    int     // complete lines indexed
	indexed  int64   // offset just past the last indexed newline
	size     int64   // file size when indexing last reached the end
	running  bool
	replaced bool
}

// Open starts indexing path. onProgress is called from the indexing goroutine
// whenever more lines are indexed or the file is reset.
func Open(path string, onProgress func()) (*File, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	lf := &File{path: path, onProgress: onProgress, f: f, marks: []int64{0}}
	lf.startLocked()
	return lf, nil
}

func (lf *File) Path() string { return lf.path }

// Lines returns how many lines are known: the indexed lines, plus a final line
// without a newline once indexing has reached the end of the file.
func (lf *File) Lines() int {
	lf.mu.Lock()
	defer lf.mu.Unlock()
	return lf.linesLocked()
}

func (lf *File) linesLocked() int {
	n := lf.lines
	if !lf.running && lf.size > lf.indexed {
		n++
	}
	return max(n, 1)
}

// Indexing reports whether the background index has not reached the end yet.
func (lf *File) Indexing() bool {
	lf.mu.Lock()
	defer lf.mu.Unlock()
	return lf.running
}

// TakeReplaced reports whether the file was truncated or replaced since the
// last call, and clears the flag.
func (lf *File) TakeReplaced() bool {
	lf.mu.Lock()
	defer lf.mu.Unlock()
	r := lf.replaced
	lf.replaced = false
	return r
}

// Refresh checks the file on disk: a grown file indexes only the new bytes;
// a shrunk or replaced one is indexed again from the start.
func (lf *File) Refresh() {
	info, err := os.Stat(lf.path)
	if err != nil {
		return
	}
	lf.mu.Lock()
	defer lf.mu.Unlock()
	open, ferr := lf.f.Stat()
	if ferr != nil || !os.SameFile(open, info) || info.Size() < lf.indexed {
		if f, err := os.Open(lf.path); err == nil {
			lf.f.Close()
			lf.f = f
		}
		lf.gen++
		lf.marks, lf.lines, lf.indexed, lf.size = []int64{0}, 0, 0, 0
		lf.replaced = true
		lf.running = false
	}
	if !lf.running && info.Size() != lf.size {
		lf.startLocked()
	}
}

func (lf *File) Close() {
	lf.mu.Lock()
	defer lf.mu.Unlock()
	lf.gen++
	lf.f.Close()
}

func (lf *File) startLocked() {
	lf.running = true
	go lf.index(lf.gen, lf.f, lf.indexed, lf.lines)
}

func (lf *File) index(gen int, f *os.File, off int64, lines int) {
	buf := make([]byte, chunkSize)
	lastNL := off
	var marks []int64
	for {
		n, err := f.ReadAt(buf, off)
		data := buf[:n]
		pos := off
		for {
			i := bytes.IndexByte(data, '\n')
			if i < 0 {
				break
			}
			pos += int64(i) + 1
			lastNL = pos
			data = data[i+1:]
			lines++
			if lines%Stride == 0 {
				marks = append(marks, pos)
			}
		}
		off += int64(n)
		done := n < len(buf) || err != nil
		lf.mu.Lock()
		if lf.gen != gen {
			lf.mu.Unlock()
			return
		}
		lf.marks = append(lf.marks, marks...)
		marks = marks[:0]
		lf.lines, lf.indexed = lines, lastNL
		if done {
			lf.running = false
			lf.size = off
		}
		lf.mu.Unlock()
		if lf.onProgress != nil {
			lf.onProgress()
		}
		if done {
			return
		}
	}
}

// ReadLines returns up to n lines starting at line start. Lines past the
// indexed part are read too when they are already on disk.
func (lf *File) ReadLines(start, n int) []string {
	if n <= 0 || start < 0 {
		return nil
	}
	lf.mu.Lock()
	k := min(start/Stride, len(lf.marks)-1)
	off := lf.marks[k]
	skip := start - k*Stride
	f := lf.f
	lf.mu.Unlock()

	out := make([]string, 0, n)
	buf := make([]byte, 256*1024)
	var partial []byte
	for len(out) < n {
		m, err := f.ReadAt(buf, off)
		data := buf[:m]
		off += int64(m)
		for len(out) < n {
			i := bytes.IndexByte(data, '\n')
			if i < 0 {
				if len(partial) < MaxLineBytes {
					partial = append(partial, data[:min(len(data), MaxLineBytes-len(partial))]...)
				}
				break
			}
			line := data[:i]
			if len(partial) > 0 {
				line = append(partial, line[:min(len(line), max(0, MaxLineBytes-len(partial)))]...)
				partial = partial[:0]
			}
			if skip > 0 {
				skip--
			} else {
				out = append(out, displayLine(line))
			}
			data = data[i+1:]
		}
		if m < len(buf) || err != nil {
			if len(out) < n && skip == 0 && (len(partial) > 0 || start == 0 && len(out) == 0) {
				out = append(out, displayLine(partial))
			}
			break
		}
	}
	return out
}

func displayLine(b []byte) string {
	b = bytes.TrimSuffix(b, []byte{'\r'})
	if len(b) > MaxLineBytes {
		b = b[:MaxLineBytes]
	}
	if utf8.Valid(b) {
		return string(b)
	}
	return strings.ToValidUTF8(string(b), "�")
}
