package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRunSplitsLargeFile(t *testing.T) {
	payload := make([]byte, 5000)
	for i := range payload {
		payload[i] = byte((i*31 + 7) % 256)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(payload)
	}))
	defer server.Close()

	root := t.TempDir()
	manifestPath := filepath.Join(root, "urls.txt")
	outDir := filepath.Join(root, "files")
	if err := os.WriteFile(manifestPath, []byte(server.URL+"/tool.exe => tools/tool.exe\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := run(manifestPath, outDir, true, 1024, time.Minute); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(filepath.Join(outDir, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest manifestFile
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatal(err)
	}
	if len(manifest.Files) != 1 {
		t.Fatalf("got %d files, want 1", len(manifest.Files))
	}
	entry := manifest.Files[0]
	if !entry.Chunked {
		t.Fatal("entry was not chunked")
	}
	if entry.Path != "tools/tool.exe" {
		t.Fatalf("path = %q, want tools/tool.exe", entry.Path)
	}
	if len(entry.Chunks) != 5 {
		t.Fatalf("got %d chunks, want 5", len(entry.Chunks))
	}
	wantSum := sha256.Sum256(payload)
	if entry.SHA256 != hex.EncodeToString(wantSum[:]) {
		t.Fatalf("sha256 = %s, want %s", entry.SHA256, hex.EncodeToString(wantSum[:]))
	}
	if _, err := os.Stat(filepath.Join(outDir, "tools", "tool.exe")); !os.IsNotExist(err) {
		t.Fatalf("chunked original file should not exist, stat err = %v", err)
	}
	for _, chunk := range entry.Chunks {
		if _, err := os.Stat(filepath.Join(outDir, filepath.FromSlash(chunk.Path))); err != nil {
			t.Fatalf("missing chunk %s: %v", chunk.Path, err)
		}
	}
}
