param([string]$GoRoot = $env:GOROOT, [string[]]$Packages = @('./...'), [switch]$GoOnly, [string]$PackageRoot, [string]$TestName)
$ErrorActionPreference = 'Stop'
$root = Split-Path $PSScriptRoot -Parent
. (Join-Path $root 'tests/e2e/private_desktop.ps1')
if (-not [PrivateSmoke]::IsPrivateDesktop()) {
    $serialized = [Management.Automation.PSSerializer]::Serialize($PSBoundParameters)
    $encoded = [Convert]::ToBase64String([Text.Encoding]::Unicode.GetBytes($serialized))
    $script = "'" + $PSCommandPath.Replace("'", "''") + "'"
    $command = '$arguments=[Management.Automation.PSSerializer]::Deserialize([Text.Encoding]::Unicode.GetString([Convert]::FromBase64String(''' + $encoded + '''))); & ' + $script + ' @arguments'
    $log = Join-Path $root ('_temp/test-' + [Guid]::NewGuid().ToString('N') + '/test.log')
    Write-Output "Private-desktop test log: $log"
    $code = Invoke-OneSubPrivateDesktop -Command $command -Log $log -WorkingDirectory $root -TimeoutSeconds 2400
    Get-Content -LiteralPath $log -Encoding UTF8
    if ($code -ne 0) { throw "Private-desktop tests failed with exit code $code" }
    return
}
if (-not $GoRoot) { $GoRoot = Join-Path $root '.local/toolchains/go1.27.4/go' }
$env:GOROOT = $GoRoot
$env:PATH = "$GoRoot/bin;$env:PATH"
$env:GOMODCACHE = Join-Path $root '.local/gomodcache'
$env:GOCACHE = Join-Path $root '.local/gocache'
$patchedMod = & python (Join-Path $root 'tools/prepare_mygo.py')
if ($LASTEXITCODE -ne 0) { throw 'MyGo menu patch preparation failed' }
$env:GOFLAGS = "`"-modfile=$patchedMod`""
$env:PYTHONUTF8 = '1'
$env:ONESUB_SUBTITLE_WORKER = Join-Path $root 'build/output/subtitle-core/OneSub.SubtitleCore.exe'
$env:ONESUB_STYLE_WORKER = Join-Path $root 'build/output/style-core/OneSub.StyleCore.exe'
$env:ONESUB_SKILL_WORKER = Join-Path $root 'build/output/skill-tools/OneSub.SkillTools.exe'
$env:ONESUB_SKILL_ROOT = Join-Path $root 'third_party/skills/srt-subtitle-translator'
$env:ONESUB_ONEASR_ROOT = Join-Path $root 'build/output/oneasr'
$env:ONESUB_SHERPA_ROOT = Join-Path $root 'build/output/sherpa-onnx'
if (-not (Test-Path $env:ONESUB_SKILL_WORKER)) { throw 'Run build/build-skill-tools.ps1 first' }
if (-not (Test-Path $env:ONESUB_STYLE_WORKER)) { throw 'Build workers/style-core first' }
$env:ONESUB_FFMPEG = (Get-Command ffmpeg -ErrorAction Stop).Source
$env:ONESUB_FFPROBE = (Get-Command ffprobe -ErrorAction Stop).Source
if ($PackageRoot) {
    $PackageRoot = [IO.Path]::GetFullPath($PackageRoot)
    $env:ONESUB_SUBTITLE_WORKER = Join-Path $PackageRoot 'subtitle-core/OneSub.SubtitleCore.exe'
    $env:ONESUB_STYLE_WORKER = Join-Path $PackageRoot 'style-core/OneSub.StyleCore.exe'
    $env:ONESUB_SKILL_WORKER = Join-Path $PackageRoot 'skill-tools/OneSub.SkillTools.exe'
    $env:ONESUB_SKILL_ROOT = Join-Path $PackageRoot 'srt-subtitle-translator'
    $env:ONESUB_FFMPEG = Join-Path $PackageRoot 'media-tools/ffmpeg.exe'
    $env:ONESUB_FFPROBE = Join-Path $PackageRoot 'media-tools/ffprobe.exe'
    $env:ONESUB_ONEASR_ROOT = Join-Path $PackageRoot 'oneasr'
    $env:ONESUB_SHERPA_ROOT = Join-Path $PackageRoot 'sherpa-onnx'
}
if (-not (Test-Path $env:ONESUB_SUBTITLE_WORKER)) { throw 'Run build/build-subtitle.ps1 first' }
Push-Location $root
try {
    Push-Location apps/desktop
    try {
        # Message accessors are generated from locales/*.json and not tracked.
        & go run github.com/egoist/mygo/cmd/mygo generate
        if ($LASTEXITCODE -ne 0) { throw 'Message catalog generation failed' }
        & go test github.com/egoist/mygo/internal/windows -run 'Test(Menu|OneSub(Popup|NativeSurface))' -count=1
        if ($LASTEXITCODE -ne 0) { throw 'MyGo native Windows tests failed' }
        & go test github.com/egoist/mygo/ui -run 'Test(OneSub|WebView|Tester|Field|Label|TextArea|GridView)' -count=1
        if ($LASTEXITCODE -ne 0) { throw 'MyGo input and scrolling tests failed' }
        # UI review can select a focused behavior/render pass through the same
        # setup and patched-framework checks as the complete test entry.
        # The complete native render/media suite can exceed Go's default ten
        # minutes on Windows. Keep one bounded full run without skipping tests.
        $testArguments = @('-count=1', '-timeout=20m')
        if ($TestName) { $testArguments += @('-run', $TestName) }
        & go test @Packages @testArguments
        if ($LASTEXITCODE -ne 0) { throw 'Go tests failed' }
    } finally { Pop-Location }
    if ($GoOnly) { return }
    & (Join-Path $PSScriptRoot 'test-updates.ps1')
    Push-Location workers/style-core
    try {
        & bun test
        if ($LASTEXITCODE -ne 0) { throw 'SmartSub style tests failed' }
    } finally { Pop-Location }
    & python -m unittest discover -s tests/regression -v
    if ($LASTEXITCODE -ne 0) { throw 'Subtitle worker regression failed' }
    & python tools/check_repo.py
    if ($LASTEXITCODE -ne 0) { throw 'Repository check failed' }
    & python -m unittest discover -s tests/repository -v
    if ($LASTEXITCODE -ne 0) { throw 'Repository tests failed' }
} finally { Pop-Location }
