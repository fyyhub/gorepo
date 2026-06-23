package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime/debug"
	"strings"

	"github.com/fyyhub/gorepo/internal/restorefiles"
)

const modulePath = "github.com/fyyhub/gorepo"

type downloadInfo struct {
	Path    string `json:"Path"`
	Version string `json:"Version"`
	Dir     string `json:"Dir"`
	Error   string `json:"Error"`
}

func main() {
	version := flag.String("version", "", "module version to restore, for example v0.1.13")
	outDir := flag.String("out", "restored", "directory for restored files")
	verifyOnly := flag.Bool("verify-only", false, "verify mirrored files without writing restored chunked files")
	flag.Parse()

	if *version == "" && flag.NArg() > 0 {
		*version = flag.Arg(0)
	}
	if *version == "" {
		*version = versionFromBuildInfo()
	}

	if err := run(*version, *outDir, *verifyOnly); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func versionFromBuildInfo() string {
	if info, ok := debug.ReadBuildInfo(); ok {
		if info.Main.Version != "" && info.Main.Version != "(devel)" {
			return info.Main.Version
		}
	}
	return ""
}

func run(version, outDir string, verifyOnly bool) error {
	version = strings.TrimSpace(version)
	if version == "" {
		return errors.New("missing version; run this command as github.com/fyyhub/gorepo/cmd/restoreversion@v0.1.13 or use -version v0.1.13")
	}
	if strings.ContainsAny(version, " \t\r\n") {
		return fmt.Errorf("invalid version %q", version)
	}

	info, err := downloadModule(version)
	if err != nil {
		return err
	}
	filesDir := filepath.Join(info.Dir, "files")
	fmt.Printf("using %s@%s from %s\n", info.Path, info.Version, info.Dir)
	return restorefiles.Run(filesDir, outDir, verifyOnly)
}

func downloadModule(version string) (downloadInfo, error) {
	target := modulePath + "@" + version
	cmd := exec.Command("go", "mod", "download", "-json", target)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = strings.TrimSpace(stdout.String())
		}
		if msg == "" {
			msg = err.Error()
		}
		return downloadInfo{}, fmt.Errorf("go mod download %s failed: %s", target, msg)
	}

	var info downloadInfo
	if err := json.Unmarshal(stdout.Bytes(), &info); err != nil {
		return downloadInfo{}, fmt.Errorf("parse go mod download output: %w", err)
	}
	if info.Error != "" {
		return downloadInfo{}, fmt.Errorf("go mod download %s failed: %s", target, info.Error)
	}
	if info.Dir == "" {
		return downloadInfo{}, fmt.Errorf("go mod download %s did not return a module directory", target)
	}
	return info, nil
}
