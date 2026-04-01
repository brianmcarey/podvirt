package util

import (
	"os"
	"path/filepath"
	"testing"
)

func TestHasQcow2Magic(t *testing.T) {
	path := filepath.Join(t.TempDir(), "disk.img")
	if err := os.WriteFile(path, []byte{'Q', 'F', 'I', 0xfb, 0x00}, 0644); err != nil {
		t.Fatalf("writing disk image: %v", err)
	}

	ok, err := HasQcow2Magic(path)
	if err != nil {
		t.Fatalf("HasQcow2Magic returned error: %v", err)
	}
	if !ok {
		t.Fatal("expected qcow2 magic to be detected")
	}
}

func TestIsQcow2Image_FallsBackToExtension(t *testing.T) {
	if !IsQcow2Image("/does/not/exist/disk.qcow2") {
		t.Fatal("expected .qcow2 extension fallback to be treated as qcow2")
	}
}
