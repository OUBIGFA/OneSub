# Prepare the unchanged sherpa-onnx Windows x64 CPU runtime that runs the
# SenseVoice, Paraformer and Parakeet-TDT models. Models are not bundled.
param([string]$Source)
$ErrorActionPreference = 'Stop'
$root = Split-Path $PSScriptRoot -Parent
$runtime = [IO.File]::ReadAllText((Join-Path $root 'third_party/manifests/runtimes.json')) | ConvertFrom-Json
$sherpa = $runtime.sherpaOnnx
if (-not $sherpa) { throw 'sherpa-onnx is not pinned in the runtime manifest' }
$target = Join-Path $root 'build/output/sherpa-onnx'
$cache = Join-Path $root ".local/downloads/sherpa-onnx-$($sherpa.version)-win-x64.tar.bz2"
function Test-Payload([string]$Path, $Spec) {
    (Test-Path -LiteralPath $Path) -and (Get-Item -LiteralPath $Path).Length -eq $Spec.bytes -and (Get-FileHash -LiteralPath $Path -Algorithm SHA256).Hash.ToLowerInvariant() -eq $Spec.sha256
}
$ready = $true
foreach ($file in $sherpa.files) {
    if (-not (Test-Payload (Join-Path $target $file.file) $file)) { $ready = $false }
}
if ($ready) { return }
$archive = @{ bytes = $sherpa.archiveBytes; sha256 = $sherpa.archiveSha256 }
if (-not $Source) {
    $Source = $cache
    if (-not (Test-Payload $cache $archive)) {
        New-Item -ItemType Directory -Path (Split-Path $cache -Parent) -Force | Out-Null
        $partial = "$cache.partial"
        try {
            Invoke-WebRequest -Uri $sherpa.download -OutFile $partial -UseBasicParsing
            if (-not (Test-Payload $partial $archive)) { throw 'sherpa-onnx archive differs from the pinned official release' }
            Move-Item -LiteralPath $partial -Destination $cache -Force
        } finally {
            Remove-Item -LiteralPath $partial -Force -ErrorAction SilentlyContinue
        }
    }
}
if (-not (Test-Payload $Source $archive)) { throw 'sherpa-onnx archive differs from the pinned official release' }
$extract = Join-Path $root ".local/downloads/sherpa-onnx-$($sherpa.version)-extract"
if (Test-Path -LiteralPath $extract) { Remove-Item -LiteralPath $extract -Recurse -Force }
New-Item -ItemType Directory -Path $extract -Force | Out-Null
try {
    $prefix = [IO.Path]::GetFileNameWithoutExtension([IO.Path]::GetFileNameWithoutExtension(([Uri]$sherpa.download).Segments[-1]))
    $members = $sherpa.files | ForEach-Object { "$prefix/$($_.source)" }
    # Windows bsdtar: a GNU tar on PATH reads "E:" as a remote host.
    & (Join-Path $env:SystemRoot 'System32/tar.exe') -xjf $Source -C $extract @members
    if ($LASTEXITCODE -ne 0) { throw 'sherpa-onnx archive extraction failed' }
    New-Item -ItemType Directory -Path $target -Force | Out-Null
    foreach ($file in $sherpa.files) {
        $from = Join-Path $extract "$prefix/$($file.source)"
        if (-not (Test-Payload $from $file)) { throw "sherpa-onnx file differs from the manifest: $($file.file)" }
        Copy-Item -LiteralPath $from -Destination (Join-Path $target $file.file) -Force
    }
} finally {
    Remove-Item -LiteralPath $extract -Recurse -Force -ErrorAction SilentlyContinue
}
Write-Output "Prepared sherpa-onnx $($sherpa.version): $($sherpa.archiveSha256)"
