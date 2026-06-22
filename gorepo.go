package main

import "fmt"

func main() {
	fmt.Println("gorepo stores mirrored files for Go module based transfer.")
	fmt.Println()
	fmt.Println("To restore chunked files, run:")
	fmt.Println(`  go run github.com/fyyhub/gorepo/cmd/restorefiles@<version> -in <files-dir> -out restored`)
	fmt.Println()
	fmt.Println("Example:")
	fmt.Println(`  $version = "v0.0.1"`)
	fmt.Println(`  $module = go mod download -json "github.com/fyyhub/gorepo@$version" | ConvertFrom-Json`)
	fmt.Println(`  go run "github.com/fyyhub/gorepo/cmd/restorefiles@$version" -in (Join-Path $module.Dir "files") -out restored`)
}
