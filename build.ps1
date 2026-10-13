# Builds unicornping for Windows and macOS into .\dist
$ErrorActionPreference = "Stop"
Set-Location $PSScriptRoot
$targets = @(
    @{ GOOS = "windows"; GOARCH = "amd64"; Out = "unicornping.exe" },
    @{ GOOS = "windows"; GOARCH = "arm64"; Out = "unicornping-arm64.exe" },
    @{ GOOS = "darwin";  GOARCH = "arm64"; Out = "unicornping-mac-arm64" },
    @{ GOOS = "darwin";  GOARCH = "amd64"; Out = "unicornping-mac-amd64" }
)
foreach ($t in $targets) {
    $env:GOOS = $t.GOOS; $env:GOARCH = $t.GOARCH; $env:CGO_ENABLED = "0"
    go build -trimpath -ldflags "-s -w" -o "dist/$($t.Out)" .
    Write-Host "built dist/$($t.Out)"
}
Remove-Item Env:GOOS, Env:GOARCH, Env:CGO_ENABLED
