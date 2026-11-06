$ErrorActionPreference = 'Stop'
Set-Location (Split-Path -Parent $PSScriptRoot)
$workspace = (Get-Location).Path
$env:GOMODCACHE = Join-Path $workspace '.cache/mod'
$env:GOCACHE = Join-Path $workspace '.cache/build'
$env:GOPATH = Join-Path $workspace '.cache/gopath'
$env:GOTOOLCHAIN = 'go1.27.3'
$env:CGO_ENABLED = '0'
$env:GOAMD64 = 'v1'
$env:GOARM64 = 'v8.0'
New-Item -ItemType Directory -Path 'dist' -Force | Out-Null
$sourceInventory = @()
$sourceFiles = @(Get-ChildItem -LiteralPath 'cmd','internal','scripts','examples' -Recurse -File)
$sourceFiles += Get-Item -LiteralPath 'go.mod','go.sum'
foreach ($sourceFile in ($sourceFiles | Sort-Object FullName)) {
    $relativePath = $sourceFile.FullName.Substring($workspace.Length + 1).Replace('\','/')
    $sourceInventory += @{ path = $relativePath; sha256 = (Get-FileHash -LiteralPath $sourceFile.FullName -Algorithm SHA256).Hash.ToLowerInvariant() }
}
$sourceInventory | ConvertTo-Json -Depth 5 | Set-Content -LiteralPath 'dist/source-inventory.json' -Encoding UTF8
$goVersion = & go version
if ($LASTEXITCODE -ne 0 -or $goVersion -notmatch '^go version go1\.27\.3 ') { throw 'Go 1.27.3 is required' }
& go mod verify
if ($LASTEXITCODE -ne 0) { throw 'Module verification failed' }
$artifacts = @()
foreach ($target in @(
    @{ OS = 'linux'; Arch = 'amd64'; Command = 'cpguard-updater' },
    @{ OS = 'linux'; Arch = 'arm64'; Command = 'cpguard-updater' },
    @{ OS = 'darwin'; Arch = 'amd64'; Command = 'cpguard-sign' },
    @{ OS = 'darwin'; Arch = 'arm64'; Command = 'cpguard-sign' }
)) {
    $env:GOOS = $target.OS
    $env:GOARCH = $target.Arch
    $artifactPath = "dist/$($target.Command)-$($target.OS)-$($target.Arch)"
    if ($target.OS -eq 'linux') { $artifactPath = "dist/$($target.Command)-$($target.Arch)" }
    & go build -mod=readonly -buildvcs=false -trimpath '-ldflags=-s -w -buildid=' -o $artifactPath "./cmd/$($target.Command)"
    if ($LASTEXITCODE -ne 0) { throw "Build failed: $artifactPath" }
    $digest = (Get-FileHash -LiteralPath $artifactPath -Algorithm SHA256).Hash.ToLowerInvariant()
    $artifacts += @{ path = $artifactPath; sha256 = $digest; size = (Get-Item -LiteralPath $artifactPath).Length; os = $target.OS; arch = $target.Arch }
    & go version -m $artifactPath | Set-Content -LiteralPath "$artifactPath.buildinfo.txt" -Encoding UTF8
    if ($LASTEXITCODE -ne 0) { throw 'Build information inspection failed' }
}
Remove-Item Env:GOOS
Remove-Item Env:GOARCH
$audit = & go run ./cmd/cpguard-audit dist/cpguard-updater-amd64 dist/cpguard-updater-arm64
if ($LASTEXITCODE -ne 0) { throw 'Static ELF audit failed' }
$audit | Set-Content -LiteralPath 'dist/static-audit.jsonl' -Encoding UTF8
$modules = & go list -m -f '{{.Path}} {{.Version}}' all
if ($LASTEXITCODE -ne 0) { throw 'Module inventory failed' }
$components = @()
foreach ($moduleLine in $modules) {
    $parts = $moduleLine.Trim().Split(' ', [System.StringSplitOptions]::RemoveEmptyEntries)
    if ($parts.Length -lt 2) { continue }
    $components += @{ type = 'library'; name = $parts[0]; version = $parts[1]; 'bom-ref' = "$($parts[0])@$($parts[1])"; purl = "pkg:golang/$($parts[0])@$($parts[1])" }
}
$sbom = @{ bomFormat = 'CycloneDX'; specVersion = '1.6'; version = 1; serialNumber = "urn:uuid:$([guid]::NewGuid())"; metadata = @{ timestamp = [DateTime]::UtcNow.ToString('o'); component = @{ type = 'application'; name = 'cpguard-updater'; version = 'development' } }; components = $components }
$sbom | ConvertTo-Json -Depth 10 | Set-Content -LiteralPath 'dist/sbom.cdx.json' -Encoding UTF8
$record = @{ toolchain = $goVersion; cgo = '0'; goamd64 = 'v1'; goarm64 = 'v8.0'; flags = @('-mod=readonly','-buildvcs=false','-trimpath','-ldflags=-s -w -buildid='); source_inventory_sha256 = (Get-FileHash -LiteralPath 'dist/source-inventory.json' -Algorithm SHA256).Hash.ToLowerInvariant(); go_mod_sha256 = (Get-FileHash -LiteralPath 'go.mod' -Algorithm SHA256).Hash.ToLowerInvariant(); go_sum_sha256 = (Get-FileHash -LiteralPath 'go.sum' -Algorithm SHA256).Hash.ToLowerInvariant(); site_config_sha256 = (Get-FileHash -LiteralPath 'internal/site/site.json' -Algorithm SHA256).Hash.ToLowerInvariant(); artifacts = $artifacts; production_ready = $false }
$record | ConvertTo-Json -Depth 10 | Set-Content -LiteralPath 'dist/release-record.json' -Encoding UTF8
Write-Output 'Built Linux updaters and macOS publisher tools. Production trust remains unprovisioned.'
