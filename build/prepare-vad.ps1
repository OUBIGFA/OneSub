# Prepare the unchanged FireRedVAD version embedded by OneAsr v1.3.0.
# No user's model directory or installed application is modified.
param([string]$Source)
$ErrorActionPreference = 'Stop'
$root = Split-Path $PSScriptRoot -Parent
$runtime = [IO.File]::ReadAllText((Join-Path $root 'third_party/manifests/runtimes.json')) | ConvertFrom-Json
$vad = $runtime.fireredvad
if (-not $vad) { throw 'FireRedVAD is not pinned in the runtime manifest' }
$target = Join-Path $root "build/output/oneasr/$($vad.file)"
$cache = Join-Path $root ".local/downloads/fireredvad-$($vad.version).exe"
function Assert-VadPayload([string]$Path) {
    if ((Get-Item -LiteralPath $Path).Length -ne $vad.bytes -or (Get-FileHash -LiteralPath $Path -Algorithm SHA256).Hash.ToLowerInvariant() -ne $vad.sha256) {
        throw 'FireRedVAD payload differs from the pinned official release'
    }
}
if (Test-Path -LiteralPath $target) {
    Assert-VadPayload $target
    return
}
if (-not $Source) {
    $Source = $cache
    if (-not (Test-Path -LiteralPath $cache)) {
        New-Item -ItemType Directory -Path (Split-Path $cache -Parent) -Force | Out-Null
        $partial = "$cache.partial"
        try {
            Invoke-WebRequest -Uri $vad.download -OutFile $partial -UseBasicParsing
            Assert-VadPayload $partial
            Move-Item -LiteralPath $partial -Destination $cache
        } finally {
            Remove-Item -LiteralPath $partial -Force -ErrorAction SilentlyContinue
        }
    }
}
Assert-VadPayload $Source
New-Item -ItemType Directory -Path (Split-Path $target -Parent) -Force | Out-Null
Copy-Item -LiteralPath $Source -Destination $target
Assert-VadPayload $target
Write-Output "Prepared FireRedVAD $($vad.version): $($vad.sha256)"
