$ErrorActionPreference = 'Stop'
$root = Split-Path $PSScriptRoot -Parent
Push-Location (Join-Path $root 'apps/desktop/player')
try {
    & bun install --frozen-lockfile --ignore-scripts
    if ($LASTEXITCODE -ne 0) { throw 'Player dependencies failed' }
    & bun run build
    if ($LASTEXITCODE -ne 0) { throw 'Player bundle failed' }
    & bun build ./node_modules/jassub/dist/worker/worker.js --target browser --outfile ./generated/worker.js
    if ($LASTEXITCODE -ne 0) { throw 'JASSUB worker bundle failed' }
    Copy-Item ./node_modules/jassub/dist/wasm/*.wasm ./generated/ -Force
} finally { Pop-Location }
