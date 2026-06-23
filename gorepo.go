package main

import "fmt"

func main() {
	fmt.Println("gorepo stores mirrored files for Go module based transfer.")
	fmt.Println()
	fmt.Println("To restore chunked files, run:")
	fmt.Println(`  go run github.com/fyyhub/gorepo/cmd/restoreversion@<version> -out restored`)
	fmt.Println()
	fmt.Println("Example:")
	fmt.Println(`  go run "github.com/fyyhub/gorepo/cmd/restoreversion@v0.1.13" -out restored`)
}
