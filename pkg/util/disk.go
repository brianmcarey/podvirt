package util

import (
	"io"
	"os"
	"path/filepath"
	"strings"
)

var qcow2Magic = [4]byte{'Q', 'F', 'I', 0xfb}

// HasQcow2Magic reports whether path starts with the qcow2 magic bytes.
func HasQcow2Magic(path string) (bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer f.Close()

	var magic [4]byte
	if _, err := io.ReadFull(f, magic[:]); err != nil {
		return false, err
	}
	return magic == qcow2Magic, nil
}

// IsQcow2Image reports whether path refers to a qcow2 disk image.
func IsQcow2Image(path string) bool {
	if ok, err := HasQcow2Magic(path); err == nil {
		return ok
	}
	return strings.EqualFold(filepath.Ext(path), ".qcow2")
}
