package client

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

const maxPDFBytes = 64 << 20
const maxAttachmentBytes = 100 << 20

// writeDownload streams into a sibling temporary file, publishing it only
// after validation and a complete read. A limit is an error, never truncation.
func writeDownload(body io.Reader, outPath string, limit int64, pdf bool) (int, error) {
	if pdf {
		var header [5]byte
		if _, err := io.ReadFull(body, header[:]); err != nil {
			return 0, fmt.Errorf("reading PDF header: %w", err)
		}
		if !bytes.Equal(header[:], []byte("%PDF-")) {
			return 0, fmt.Errorf("response is not a PDF")
		}
		body = io.MultiReader(bytes.NewReader(header[:]), body)
	}
	mode := os.FileMode(0o600)
	if info, err := os.Lstat(outPath); err == nil {
		if !info.Mode().IsRegular() {
			return 0, fmt.Errorf("download destination must be a regular file")
		}
		mode = info.Mode().Perm()
	} else if !os.IsNotExist(err) {
		return 0, fmt.Errorf("checking download destination: %w", err)
	}
	f, err := os.CreateTemp(filepath.Dir(outPath), ".qb-download-*")
	if err != nil {
		return 0, fmt.Errorf("creating download: %w", err)
	}
	defer func() {
		_ = f.Close()
		_ = os.Remove(f.Name())
	}()
	// Larger bounded chunks avoid one file write per 32 KiB. Hide ReadFrom
	// so io.CopyBuffer uses this buffer rather than os.File's smaller one.
	n, err := io.CopyBuffer(struct{ io.Writer }{f}, io.LimitReader(body, limit+1), make([]byte, 256<<10))
	if err != nil {
		return 0, fmt.Errorf("reading download: %w", err)
	}
	if n > limit {
		return 0, fmt.Errorf("download exceeds %d byte limit", limit)
	}
	if err := f.Chmod(mode); err != nil {
		return 0, fmt.Errorf("setting download permissions: %w", err)
	}
	if err := f.Close(); err != nil {
		return 0, fmt.Errorf("closing download: %w", err)
	}
	if err := os.Rename(f.Name(), outPath); err != nil {
		return 0, fmt.Errorf("saving download: %w", err)
	}
	return int(n), nil
}
