package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/fyyhub/gorepo/internal/restorefiles"
)

func main() {
	inDir := flag.String("in", "files", "directory containing manifest.json and mirrored files")
	outDir := flag.String("out", "restored", "directory for restored files")
	verifyOnly := flag.Bool("verify-only", false, "verify mirrored files without writing restored chunked files")
	flag.Parse()

	if err := restorefiles.Run(*inDir, *outDir, *verifyOnly); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
