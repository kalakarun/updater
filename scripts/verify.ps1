$ErrorActionPreference = 'Stop'
Set-Location (Split-Path -Parent $PSScriptRoot)
$env:GOMODCACHE = "$PWD/.cache/mod"
$env:GOCACHE = "$PWD/.cache/build"
$env:GOPATH = "$PWD/.cache/gopath"
$env:GOTOOLCHAIN = 'go1.27.3'
& go test ./...
if ($LASTEXITCODE -ne 0) { throw 'Tests failed' }
& go vet ./...
if ($LASTEXITCODE -ne 0) { throw 'Vet failed' }
& go run golang.org/x/vuln/cmd/govulncheck@v1.9.0 ./...
if ($LASTEXITCODE -ne 0) { throw 'Vulnerability scan failed' }
