[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)]
    [string]$OutputDirectory
)

$ErrorActionPreference = "Stop"

$archiveName = "wintun-0.14.1.zip"
$archiveUrl = "https://www.wintun.net/builds/$archiveName"
$expectedSha256 = "07c256185d6ee3652e09fa55c0b673e2624b565e02c4b9091c79ca7d2f24ef51"
$tempRoot = Join-Path ([System.IO.Path]::GetTempPath()) ("tunnelkit-wintun-" + [System.Guid]::NewGuid().ToString("N"))
$archivePath = Join-Path $tempRoot $archiveName
$extractPath = Join-Path $tempRoot "extracted"

New-Item -ItemType Directory -Path $tempRoot -Force | Out-Null
New-Item -ItemType Directory -Path $OutputDirectory -Force | Out-Null

try {
    Invoke-WebRequest -Uri $archiveUrl -OutFile $archivePath

    $actualSha256 = (Get-FileHash -LiteralPath $archivePath -Algorithm SHA256).Hash.ToLowerInvariant()
    if ($actualSha256 -ne $expectedSha256) {
        throw "Wintun archive SHA-256 mismatch: expected $expectedSha256, got $actualSha256"
    }

    Expand-Archive -LiteralPath $archivePath -DestinationPath $extractPath -Force

    foreach ($arch in @("amd64", "arm64")) {
        $sourceDll = Join-Path $extractPath "wintun/bin/$arch/wintun.dll"
        if (-not (Test-Path -LiteralPath $sourceDll -PathType Leaf)) {
            throw "Wintun archive does not contain wintun/bin/$arch/wintun.dll"
        }
        $targetDll = Join-Path $OutputDirectory "wintun-$arch.dll"
        Copy-Item -LiteralPath $sourceDll -Destination $targetDll -Force
    }
}
finally {
    Remove-Item -LiteralPath $tempRoot -Recurse -Force -ErrorAction SilentlyContinue
}
