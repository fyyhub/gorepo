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
go get github.com/fyyhub/gorepo@v0.0.1
```

The mirrored files are included in the module source under `files/`.

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
