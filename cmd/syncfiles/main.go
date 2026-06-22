package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type manifestEntry struct {
	Source string `json:"source"`
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
}

type manifestFile struct {
	Files []manifestEntry `json:"files"`
}

type item struct {
	line int
	url  string
	rel  string
}

func main() {
	manifestPath := flag.String("manifest", "urls.txt", "URL manifest file")
	outDir := flag.String("out", "files", "directory for downloaded files")
	clean := flag.Bool("clean", false, "remove the output directory before downloading")
	timeout := flag.Duration("timeout", 5*time.Minute, "per-request timeout")
	flag.Parse()

	if err := run(*manifestPath, *outDir, *clean, *timeout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(manifestPath, outDir string, clean bool, timeout time.Duration) error {
	items, err := parseManifest(manifestPath)
	if err != nil {
		return err
	}

	outDir = filepath.Clean(outDir)
	if err := validateOutputDir(outDir); err != nil {
		return fmt.Errorf("refusing to use unsafe output directory %q", outDir)
	}

	if clean {
		if err := os.RemoveAll(outDir); err != nil {
			return fmt.Errorf("clean output directory: %w", err)
		}
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return fmt.Errorf("create output directory: %w", err)
	}

	client := &http.Client{Timeout: timeout}
	seen := map[string]string{}
	entries := make([]manifestEntry, 0, len(items))

	for _, it := range items {
		rel := it.rel
		if rel == "" {
			rel, err = outputPathForURL(it.url)
			if err != nil {
				return fmt.Errorf("line %d: %w", it.line, err)
			}
		}
		rel, err = cleanRelativePath(rel)
		if err != nil {
			return fmt.Errorf("line %d: invalid output path: %w", it.line, err)
		}
		if prior, ok := seen[rel]; ok {
			return fmt.Errorf("line %d: output path %q is already used by %s", it.line, rel, prior)
		}
		seen[rel] = it.url

		entry, err := download(client, it.url, outDir, rel)
		if err != nil {
			return fmt.Errorf("line %d: %w", it.line, err)
		}
		entries = append(entries, entry)
		fmt.Printf("downloaded %s -> %s (%d bytes)\n", it.url, entry.Path, entry.Size)
	}

	sort.SliceStable(entries, func(i, j int) bool {
		return entries[i].Path < entries[j].Path
	})
	return writeJSON(filepath.Join(outDir, "manifest.json"), manifestFile{Files: entries})
}

func parseManifest(name string) ([]item, error) {
	data, err := os.ReadFile(name)
	if err != nil {
		return nil, fmt.Errorf("read manifest: %w", err)
	}

	var items []item
	for i, raw := range strings.Split(string(data), "\n") {
		line := strings.TrimSpace(raw)
		line = strings.TrimPrefix(line, "\ufeff")
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		source := line
		rel := ""
		if strings.Contains(line, "=>") {
			parts := strings.SplitN(line, "=>", 2)
			source = strings.TrimSpace(parts[0])
			rel = strings.TrimSpace(parts[1])
			if rel == "" {
				return nil, fmt.Errorf("line %d: missing output path after =>", i+1)
			}
		}
		if _, err := parseHTTPURL(source); err != nil {
			return nil, fmt.Errorf("line %d: %w", i+1, err)
		}
		items = append(items, item{line: i + 1, url: source, rel: rel})
	}
	return items, nil
}

func download(client *http.Client, source, outDir, rel string) (manifestEntry, error) {
	req, err := http.NewRequest(http.MethodGet, source, nil)
	if err != nil {
		return manifestEntry{}, err
	}
	req.Header.Set("User-Agent", "gorepo-file-mirror/1.0")

	resp, err := client.Do(req)
	if err != nil {
		return manifestEntry{}, fmt.Errorf("download %s: %w", source, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return manifestEntry{}, fmt.Errorf("download %s: unexpected status %s", source, resp.Status)
	}

	dst := filepath.Join(outDir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return manifestEntry{}, err
	}

	tmp := dst + ".tmp"
	file, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return manifestEntry{}, err
	}

	hasher := sha256.New()
	size, copyErr := io.Copy(io.MultiWriter(file, hasher), resp.Body)
	closeErr := file.Close()
	if copyErr != nil {
		_ = os.Remove(tmp)
		return manifestEntry{}, copyErr
	}
	if closeErr != nil {
		_ = os.Remove(tmp)
		return manifestEntry{}, closeErr
	}
	if err := os.Rename(tmp, dst); err != nil {
		_ = os.Remove(tmp)
		return manifestEntry{}, err
	}

	return manifestEntry{
		Source: source,
		Path:   filepath.ToSlash(rel),
		SHA256: hex.EncodeToString(hasher.Sum(nil)),
		Size:   size,
	}, nil
}

func outputPathForURL(raw string) (string, error) {
	u, err := parseHTTPURL(raw)
	if err != nil {
		return "", err
	}

	segments := []string{sanitizeSegment(u.Host)}
	for _, part := range strings.Split(strings.TrimPrefix(u.EscapedPath(), "/"), "/") {
		if part == "" {
			continue
		}
		unescaped, err := url.PathUnescape(part)
		if err != nil {
			unescaped = part
		}
		segments = append(segments, sanitizeSegment(unescaped))
	}
	if len(segments) == 1 {
		segments = append(segments, "index")
	}

	if u.RawQuery != "" {
		last := segments[len(segments)-1]
		ext := path.Ext(last)
		base := strings.TrimSuffix(last, ext)
		sum := sha256.Sum256([]byte(u.RawQuery))
		segments[len(segments)-1] = fmt.Sprintf("%s-%s%s", base, hex.EncodeToString(sum[:4]), ext)
	}
	return path.Join(segments...), nil
}

func parseHTTPURL(raw string) (*url.URL, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return nil, err
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, fmt.Errorf("only http and https URLs are supported: %s", raw)
	}
	if u.Host == "" {
		return nil, fmt.Errorf("missing URL host: %s", raw)
	}
	return u, nil
}

func cleanRelativePath(rel string) (string, error) {
	if rel == "" {
		return "", errors.New("empty path")
	}
	rel = strings.ReplaceAll(rel, "\\", "/")
	cleaned := path.Clean(rel)
	if cleaned == "." || strings.HasPrefix(cleaned, "../") || cleaned == ".." || path.IsAbs(cleaned) {
		return "", fmt.Errorf("path must stay inside the output directory: %q", rel)
	}
	return cleaned, nil
}

func validateOutputDir(outDir string) error {
	if outDir == "." {
		return errors.New("output directory cannot be the current directory")
	}
	abs, err := filepath.Abs(outDir)
	if err != nil {
		return err
	}
	volume := filepath.VolumeName(abs)
	root := volume + string(filepath.Separator)
	if abs == root || abs == string(filepath.Separator) {
		return errors.New("output directory cannot be a filesystem root")
	}
	return nil
}

func sanitizeSegment(s string) string {
	s = strings.TrimSpace(s)
	if s == "" || s == "." || s == ".." {
		return "file"
	}
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case strings.ContainsRune("._-+@", r):
			b.WriteRune(r)
		default:
			b.WriteRune('_')
		}
	}
	out := strings.Trim(b.String(), ".")
	if out == "" {
		return "file"
	}
	return out
}

func writeJSON(name string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(name, data, 0o644)
}
