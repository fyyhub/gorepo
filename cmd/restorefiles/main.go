package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
)

type manifestFile struct {
	Files []manifestEntry `json:"files"`
}

type manifestEntry struct {
	Source    string      `json:"source"`
	Path      string      `json:"path"`
	SHA256    string      `json:"sha256"`
	Size      int64       `json:"size"`
	Chunked   bool        `json:"chunked,omitempty"`
	ChunkSize int64       `json:"chunk_size,omitempty"`
	Chunks    []chunkInfo `json:"chunks,omitempty"`
}

type chunkInfo struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
}

func main() {
	inDir := flag.String("in", "files", "directory containing manifest.json and mirrored files")
	outDir := flag.String("out", "restored", "directory for restored files")
	verifyOnly := flag.Bool("verify-only", false, "verify mirrored files without writing restored chunked files")
	flag.Parse()

	if err := run(*inDir, *outDir, *verifyOnly); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(inDir, outDir string, verifyOnly bool) error {
	if err := validateDir(inDir, false); err != nil {
		return fmt.Errorf("invalid input directory: %w", err)
	}
	if !verifyOnly {
		if err := validateDir(outDir, true); err != nil {
			return fmt.Errorf("invalid output directory: %w", err)
		}
		if err := os.MkdirAll(outDir, 0o755); err != nil {
			return err
		}
	}

	manifest, err := readManifest(filepath.Join(inDir, "manifest.json"))
	if err != nil {
		return err
	}

	for _, entry := range manifest.Files {
		if err := cleanRelativePath(entry.Path); err != nil {
			return fmt.Errorf("invalid manifest path %q: %w", entry.Path, err)
		}
		if entry.Chunked {
			if err := restoreChunked(inDir, outDir, entry, verifyOnly); err != nil {
				return err
			}
			continue
		}
		if err := verifyRegular(inDir, entry); err != nil {
			return err
		}
		fmt.Printf("verified %s (%d bytes)\n", entry.Path, entry.Size)
	}
	return nil
}

func readManifest(name string) (manifestFile, error) {
	data, err := os.ReadFile(name)
	if err != nil {
		return manifestFile{}, fmt.Errorf("read manifest: %w", err)
	}
	var manifest manifestFile
	if err := json.Unmarshal(data, &manifest); err != nil {
		return manifestFile{}, fmt.Errorf("parse manifest: %w", err)
	}
	return manifest, nil
}

func verifyRegular(inDir string, entry manifestEntry) error {
	file := filepath.Join(inDir, filepath.FromSlash(entry.Path))
	sum, size, err := hashFile(file)
	if err != nil {
		return fmt.Errorf("verify %s: %w", entry.Path, err)
	}
	if size != entry.Size {
		return fmt.Errorf("verify %s: size mismatch: got %d want %d", entry.Path, size, entry.Size)
	}
	if sum != entry.SHA256 {
		return fmt.Errorf("verify %s: sha256 mismatch: got %s want %s", entry.Path, sum, entry.SHA256)
	}
	return nil
}

func restoreChunked(inDir, outDir string, entry manifestEntry, verifyOnly bool) error {
	if len(entry.Chunks) == 0 {
		return fmt.Errorf("restore %s: missing chunks", entry.Path)
	}

	var dst *os.File
	var tmp string
	if !verifyOnly {
		target := filepath.Join(outDir, filepath.FromSlash(entry.Path))
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		tmp = target + ".tmp"
		file, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
		if err != nil {
			return err
		}
		dst = file
		defer os.Remove(tmp)
	}

	hasher := sha256.New()
	var total int64
	for i, chunk := range entry.Chunks {
		if err := cleanRelativePath(chunk.Path); err != nil {
			return fmt.Errorf("restore %s: invalid chunk path %q: %w", entry.Path, chunk.Path, err)
		}
		chunkFile := filepath.Join(inDir, filepath.FromSlash(chunk.Path))
		sum, size, err := copyChunk(chunkFile, dst, hasher)
		if err != nil {
			if dst != nil {
				_ = dst.Close()
			}
			return fmt.Errorf("restore %s: chunk %d: %w", entry.Path, i+1, err)
		}
		if size != chunk.Size {
			if dst != nil {
				_ = dst.Close()
			}
			return fmt.Errorf("restore %s: chunk %d size mismatch: got %d want %d", entry.Path, i+1, size, chunk.Size)
		}
		if sum != chunk.SHA256 {
			if dst != nil {
				_ = dst.Close()
			}
			return fmt.Errorf("restore %s: chunk %d sha256 mismatch: got %s want %s", entry.Path, i+1, sum, chunk.SHA256)
		}
		total += size
	}

	if dst != nil {
		if err := dst.Close(); err != nil {
			return err
		}
	}
	if total != entry.Size {
		return fmt.Errorf("restore %s: size mismatch: got %d want %d", entry.Path, total, entry.Size)
	}

	sum := hex.EncodeToString(hasher.Sum(nil))
	if sum != entry.SHA256 {
		return fmt.Errorf("restore %s: sha256 mismatch: got %s want %s", entry.Path, sum, entry.SHA256)
	}
	if !verifyOnly {
		target := filepath.Join(outDir, filepath.FromSlash(entry.Path))
		if err := os.Rename(tmp, target); err != nil {
			return err
		}
	}
	if verifyOnly {
		fmt.Printf("verified %s (%d bytes from %d chunks)\n", entry.Path, entry.Size, len(entry.Chunks))
	} else {
		fmt.Printf("restored %s (%d bytes from %d chunks)\n", entry.Path, entry.Size, len(entry.Chunks))
	}
	return nil
}

func copyChunk(name string, dst *os.File, fullHash io.Writer) (string, int64, error) {
	file, err := os.Open(name)
	if err != nil {
		return "", 0, fmt.Errorf("open %s: %w", name, err)
	}
	defer file.Close()

	chunkHash := sha256.New()
	writer := io.MultiWriter(fullHash, chunkHash)
	if dst != nil {
		writer = io.MultiWriter(dst, fullHash, chunkHash)
	}
	size, err := io.Copy(writer, file)
	if err != nil {
		return "", size, fmt.Errorf("copy %s: %w", name, err)
	}
	return hex.EncodeToString(chunkHash.Sum(nil)), size, nil
}

func hashFile(name string) (string, int64, error) {
	file, err := os.Open(name)
	if err != nil {
		return "", 0, err
	}
	defer file.Close()

	hasher := sha256.New()
	size, err := io.Copy(hasher, file)
	if err != nil {
		return "", size, err
	}
	return hex.EncodeToString(hasher.Sum(nil)), size, nil
}

func cleanRelativePath(rel string) error {
	if rel == "" {
		return errors.New("empty path")
	}
	rel = strings.ReplaceAll(rel, "\\", "/")
	cleaned := path.Clean(rel)
	if cleaned == "." || strings.HasPrefix(cleaned, "../") || cleaned == ".." || path.IsAbs(cleaned) {
		return fmt.Errorf("path must stay inside the directory: %q", rel)
	}
	return nil
}

func validateDir(dir string, allowCreate bool) error {
	if dir == "" {
		return errors.New("empty directory")
	}
	cleaned := filepath.Clean(dir)
	if cleaned == "." && allowCreate {
		return errors.New("output directory cannot be the current directory")
	}
	abs, err := filepath.Abs(cleaned)
	if err != nil {
		return err
	}
	volume := filepath.VolumeName(abs)
	root := volume + string(filepath.Separator)
	if abs == root || abs == string(filepath.Separator) {
		return errors.New("directory cannot be a filesystem root")
	}
	return nil
}
