# Exercise the real public MyGo updater in an isolated executable directory.
# The temporary signing key never matches the product key and is removed.
$ErrorActionPreference = 'Stop'
$root = Split-Path $PSScriptRoot -Parent
$env:GOROOT = Join-Path $root '.local/toolchains/go1.27.3/go'
$env:PATH = "$env:GOROOT/bin;$env:PATH"
$env:GOMODCACHE = Join-Path $root '.local/gomodcache'
$env:GOCACHE = Join-Path $root '.local/gocache'
$patchedMod = & python (Join-Path $root 'tools/prepare_mygo.py')
if ($LASTEXITCODE -ne 0) { throw 'MyGo patch preparation failed' }
$env:GOFLAGS = "`"-modfile=$patchedMod`""
$task = Join-Path $root ("_temp/signed-update-tests/" + [Guid]::NewGuid().ToString('N'))
$keys = Join-Path $task 'keys'
$runner = Join-Path $task 'runner'
$previousKeyFile = $env:ONESUB_UPDATE_TEST_KEY_FILE
$previousRoot = $env:ONESUB_UPDATE_TEST_ROOT
$previousDev = $env:MYGO_DEV
New-Item -ItemType Directory $runner -Force | Out-Null
Push-Location (Join-Path $root 'apps/desktop')
try {
    & go test github.com/egoist/mygo -run '^TestUpdater' -count=1
    if ($LASTEXITCODE -ne 0) { throw 'MyGo updater regression tests failed' }
    & go test github.com/egoist/mygo/internal/update -count=1
    if ($LASTEXITCODE -ne 0) { throw 'MyGo signature, archive and version tests failed' }
    & go run github.com/egoist/mygo/cmd/mygo keygen -o $keys
    if ($LASTEXITCODE -ne 0) { throw 'Temporary update test key generation failed' }
    $publicKey = [IO.File]::ReadAllText((Join-Path $keys 'mygo-update.pub')).Trim()
    $exe = Join-Path $runner 'OneSub.UpdateTests.exe'
    $flags = "-X github.com/egoist/mygo.production=1 -X github.com/egoist/mygo.packageUpdateFeed=https://github.com/OUBIGFA/OneSub/releases/latest/download/update-windows-amd64.json -X github.com/egoist/mygo.packageUpdateKey=$publicKey"
    & go test -c -o $exe -ldflags $flags .
    if ($LASTEXITCODE -ne 0) { throw 'Isolated updater test build failed' }
    $env:ONESUB_UPDATE_TEST_KEY_FILE = Join-Path $keys 'mygo-update.key'
    $env:ONESUB_UPDATE_TEST_ROOT = $runner
    $env:MYGO_DEV = $null
    & $exe @('-test.run', '^TestSignedUpdateBuildOrderingAndVerification$', '-test.v')
    if ($LASTEXITCODE -ne 0) { throw 'Signed internal-build update acceptance failed' }
} finally {
    $env:ONESUB_UPDATE_TEST_KEY_FILE = $previousKeyFile
    $env:ONESUB_UPDATE_TEST_ROOT = $previousRoot
    $env:MYGO_DEV = $previousDev
    Pop-Location
    Remove-Item $task -Recurse -Force
}
