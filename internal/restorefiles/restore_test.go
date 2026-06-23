package restorefiles

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestRunRestoresChunkedFile(t *testing.T) {
	payload := make([]byte, 5000)
	for i := range payload {
		payload[i] = byte((i*31 + 7) % 256)
	}

	root := t.TempDir()
	inDir := filepath.Join(root, "files")
	outDir := filepath.Join(root, "restored")
	chunkDir := filepath.Join(inDir, "_chunks", "tools", "tool.exe.parts")
	if err := os.MkdirAll(chunkDir, 0o755); err != nil {
		t.Fatal(err)
	}

	var chunks []ChunkInfo
	for i, start := 0, 0; start < len(payload); i, start = i+1, start+1024 {
		end := start + 1024
		if end > len(payload) {
			end = len(payload)
		}
		part := payload[start:end]
		sum := sha256.Sum256(part)
		name := filepath.Join(chunkDir, fmt.Sprintf("part%04d", i+1))
		if err := os.WriteFile(name, part, 0o644); err != nil {
			t.Fatal(err)
		}
		chunks = append(chunks, ChunkInfo{
			Path:   filepath.ToSlash(filepath.Join("_chunks", "tools", "tool.exe.parts", filepath.Base(name))),
			SHA256: hex.EncodeToString(sum[:]),
			Size:   int64(len(part)),
		})
	}

	sum := sha256.Sum256(payload)
	manifest := ManifestFile{Files: []ManifestEntry{{
		Source:    "https://example.com/tool.exe",
		Path:      "tools/tool.exe",
		SHA256:    hex.EncodeToString(sum[:]),
		Size:      int64(len(payload)),
		Chunked:   true,
		ChunkSize: 1024,
		Chunks:    chunks,
	}}}
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(inDir, "manifest.json"), append(data, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := Run(inDir, outDir, false); err != nil {
		t.Fatal(err)
	}
	restored, err := os.ReadFile(filepath.Join(outDir, "tools", "tool.exe"))
	if err != nil {
		t.Fatal(err)
	}
	if string(restored) != string(payload) {
		t.Fatal("restored payload did not match original")
	}
	if err := Run(inDir, filepath.Join(root, "verify-output"), true); err != nil {
		t.Fatal(err)
	}
}
