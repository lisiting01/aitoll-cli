package codexremote

import (
	"context"
	"errors"
	"io"
	"os"
	"time"
)

// TailLast writes the last n lines of the log file to w.
func TailLast(w io.Writer, n int) error {
	if n <= 0 {
		n = 200
	}
	path, err := LogPath()
	if err != nil {
		return err
	}
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return errors.New("no log file yet (start the daemon first)")
		}
		return err
	}
	defer f.Close()

	lines, err := readLastLines(f, n)
	if err != nil {
		return err
	}
	for _, line := range lines {
		if _, err := w.Write([]byte(line)); err != nil {
			return err
		}
		if _, err := w.Write([]byte("\n")); err != nil {
			return err
		}
	}
	return nil
}

// Follow streams new log lines as they appear, until ctx is cancelled.
func Follow(ctx context.Context, w io.Writer, n int) error {
	path, err := LogPath()
	if err != nil {
		return err
	}

	// First, dump the last n lines.
	if err := TailLast(w, n); err != nil && !errors.Is(err, os.ErrNotExist) {
		// Allow the file not yet existing; we'll wait for it.
	}

	var f *os.File
	for {
		select {
		case <-ctx.Done():
			if f != nil {
				_ = f.Close()
			}
			return nil
		default:
		}
		if f == nil {
			var err error
			f, err = os.Open(path)
			if err != nil {
				if os.IsNotExist(err) {
					time.Sleep(500 * time.Millisecond)
					continue
				}
				return err
			}
			// Seek to end so we don't re-emit the lines TailLast already showed.
			if _, err := f.Seek(0, io.SeekEnd); err != nil {
				_ = f.Close()
				return err
			}
		}
		buf := make([]byte, 4096)
		nRead, err := f.Read(buf)
		if nRead > 0 {
			if _, werr := w.Write(buf[:nRead]); werr != nil {
				_ = f.Close()
				return werr
			}
		}
		if err == io.EOF {
			time.Sleep(300 * time.Millisecond)
			continue
		}
		if err != nil {
			_ = f.Close()
			return err
		}
	}
}

// readLastLines reads up to n trailing lines from f. Reads in 4KB chunks from
// the end and walks backward.
func readLastLines(f *os.File, n int) ([]string, error) {
	st, err := f.Stat()
	if err != nil {
		return nil, err
	}
	size := st.Size()
	if size == 0 {
		return nil, nil
	}

	const chunk = 4096
	var (
		lines  []string
		buf    []byte
		offset = size
	)
	for offset > 0 && len(lines) <= n {
		readSize := int64(chunk)
		if offset < readSize {
			readSize = offset
		}
		offset -= readSize
		tmp := make([]byte, readSize)
		if _, err := f.ReadAt(tmp, offset); err != nil && err != io.EOF {
			return nil, err
		}
		buf = append(tmp, buf...)
		lines = splitLines(buf)
	}
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return lines, nil
}

func splitLines(b []byte) []string {
	var out []string
	start := 0
	for i := 0; i < len(b); i++ {
		if b[i] == '\n' {
			line := b[start:i]
			// Strip trailing \r for CRLF logs (Windows).
			if len(line) > 0 && line[len(line)-1] == '\r' {
				line = line[:len(line)-1]
			}
			out = append(out, string(line))
			start = i + 1
		}
	}
	if start < len(b) {
		out = append(out, string(b[start:]))
	}
	return out
}
