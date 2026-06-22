# gorepo

This repository mirrors external files through GitHub Actions, commits the downloaded files into `files/`, then creates a Go module tag and a GitHub Release.

The important part for an internal Go proxy is the committed `files/` directory at each tag. Release assets are convenient, but the Go module source archive is what the Go proxy normally mirrors.

## Usage

1. Edit `urls.txt` and add one `http` or `https` URL per line.
2. Commit and push to the `main` branch.
3. GitHub Actions runs `.github/workflows/mirror-files.yml`:
   - downloads the URLs with `go run ./cmd/syncfiles -manifest urls.txt -out files -clean`
   - commits the result under `files/`
   - creates the next `v0.0.N` tag
   - creates a GitHub Release with `mirrored-files.tar.gz`
4. From the internal network, fetch the module through your company Go mirror:

```powershell
$version = "v0.0.1"
$module = go mod download -json "github.com/fyyhub/gorepo@$version" | ConvertFrom-Json
```

The mirrored files are included in the module source under `$module.Dir/files`.

## Large files and EXE files

Files larger than the chunk threshold are split automatically. The default threshold is `95 MiB`, so a large file such as `tool.exe` is stored like this:

```text
files/_chunks/tools/tool.exe.parts/part0001
files/_chunks/tools/tool.exe.parts/part0002
files/manifest.json
```

The original target path is still recorded in `files/manifest.json` as `tools/tool.exe`. Restore chunked files after fetching the module:

```powershell
$version = "v0.0.1"
$module = go mod download -json "github.com/fyyhub/gorepo@$version" | ConvertFrom-Json
go run "github.com/fyyhub/gorepo/cmd/restorefiles@$version" -in (Join-Path $module.Dir "files") -out restored
```

The restored file is written to:

```text
restored/tools/tool.exe
```

The restore tool verifies every chunk and the final file with SHA256. You can also verify without writing restored files:

```powershell
$version = "v0.0.1"
$module = go mod download -json "github.com/fyyhub/gorepo@$version" | ConvertFrom-Json
go run "github.com/fyyhub/gorepo/cmd/restorefiles@$version" -in (Join-Path $module.Dir "files") -verify-only
```

To change the chunk threshold in GitHub Actions, edit the `-chunk-size` value in `.github/workflows/mirror-files.yml`. Use `0` to disable chunking.

## URL manifest format

Default output path:

```text
https://example.com/tools/demo.zip
```

This saves to:

```text
files/example.com/tools/demo.zip
```

Custom output path:

```text
https://example.com/download?id=abc => tools/demo.zip
```

This saves to:

```text
files/tools/demo.zip
```

## Notes

- GitHub blocks normal Git files larger than 100 MB, so very large files should not be committed directly.
- Tags use `v0.0.N` to avoid Go's special module path rules for `v2+`.
- If `urls.txt` becomes empty later, the workflow cleans old `files/` content and publishes a new tag for that state.
