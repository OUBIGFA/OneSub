param([Parameter(Mandatory=$true)][string]$Exe, [Parameter(Mandatory=$true)][string]$Work, [switch]$VideoTools, [switch]$PresetReset, [switch]$WorkbenchLayout, [switch]$ChoiceControls, [switch]$Proofreading, [switch]$Development, [switch]$ScrollFollow, [ValidateSet('dark','light')][string]$Appearance = 'dark', [string]$TranslationEndpoint, [switch]$InterruptTranslation, [switch]$ClearTranslation, [switch]$ComposeOutput, [switch]$SkipTranslation, [switch]$TaskTimings, [switch]$LengthGuide, [switch]$SubtitleIssues, [string]$PipelineModelsRoot, [string]$AsrModel='Qwen3-ASR-1.7B-hf')
$ErrorActionPreference = 'Stop'
. (Join-Path $PSScriptRoot 'private_desktop.ps1')
$exePath=[IO.Path]::GetFullPath($Exe)
$workPath=[IO.Path]::GetFullPath($Work)
if(-not (Test-Path -LiteralPath $exePath)){throw 'Test executable is missing'}
if(Test-Path -LiteralPath $workPath){throw 'Use a fresh smoke directory'}
$parent=Split-Path $workPath -Parent
New-Item -ItemType Directory -Path $parent -Force | Out-Null
$log=$workPath+'.log'
function Quote-Literal([string]$value){return "'"+$value.Replace("'","''")+"'"}
$driver=Quote-Literal (Join-Path $PSScriptRoot 'native_smoke.ps1')
$capture=Quote-Literal (Join-Path $PSScriptRoot 'private_screenshot.ps1')
$command='& ([ScriptBlock]::Create([IO.File]::ReadAllText('+ $driver +',[Text.Encoding]::UTF8))) -Exe '+(Quote-Literal $exePath)+' -Work '+(Quote-Literal $workPath)+' -ScreenshotTool '+$capture+' -PrivateDesktop -OpenArgument -UIAPlayback'
if(-not $Development){$command+=' -Packaged'}
if($VideoTools){$command+=' -VideoTools'}
if($PresetReset){$command+=' -PresetReset'}
if($ChoiceControls){$command+=' -ChoiceControls'}
if($Proofreading){$command+=' -Proofreading'}
if($ScrollFollow){$command+=' -ScrollFollow'}
if($WorkbenchLayout){$command+=' -WorkbenchLayout'}
if($TranslationEndpoint){$command+=' -TranslationEndpoint '+(Quote-Literal $TranslationEndpoint)}
if($InterruptTranslation){$command+=' -InterruptTranslation'}
if($ClearTranslation){$command+=' -ClearTranslation'}
if($ComposeOutput){$command+=' -ComposeOutput'}
if($SkipTranslation){$command+=' -SkipTranslation'}
if($TaskTimings){$command+=' -TaskTimings'}
if($LengthGuide){$command+=' -LengthGuide'}
if($SubtitleIssues){$command+=' -SubtitleIssues'}
$command+=' -Appearance '+(Quote-Literal $Appearance)
if($PipelineModelsRoot){
    $bootstrap='$nativeBootstrap=[IO.File]::ReadAllText('+ $driver +',[Text.Encoding]::UTF8); & ([ScriptBlock]::Create($nativeBootstrap.Substring(0,$nativeBootstrap.IndexOf(''$workPath = '')))) -PrivateDesktop -UIAPlayback; '
    $pipeline=Quote-Literal (Join-Path $PSScriptRoot 'process_media.ps1')
    $command=$bootstrap+'& ([ScriptBlock]::Create([IO.File]::ReadAllText('+ $pipeline +',[Text.Encoding]::UTF8))) -Exe '+(Quote-Literal $exePath)+' -Work '+(Quote-Literal $workPath)+' -ScreenshotTool '+$capture+' -TranslationEndpoint '+(Quote-Literal $TranslationEndpoint)+' -ModelsRoot '+(Quote-Literal ([IO.Path]::GetFullPath($PipelineModelsRoot)))+' -AsrModel '+(Quote-Literal $AsrModel)
    if(-not $Development){$command+=' -Packaged'}
}
if($Development){
    $repoRoot=Split-Path (Split-Path $PSScriptRoot -Parent) -Parent
    $runtimeOverrides=@{
        ONESUB_SUBTITLE_WORKER='build/output/subtitle-core/OneSub.SubtitleCore.exe'
        ONESUB_STYLE_WORKER='build/output/style-core/OneSub.StyleCore.exe'
        ONESUB_SKILL_WORKER='build/output/skill-tools/OneSub.SkillTools.exe'
        ONESUB_SKILL_ROOT='third_party/skills/srt-subtitle-translator'
        ONESUB_ONEASR_ROOT='build/output/oneasr'
        ONESUB_SHERPA_ROOT='build/output/sherpa-onnx'
    }
    $setup=''
    foreach($name in $runtimeOverrides.Keys){$setup+='$env:'+ $name +'='+ (Quote-Literal (Join-Path $repoRoot $runtimeOverrides[$name]))+'; '}
    $command=$setup+$command
}
# Transcript captures native helper output without PowerShell 5.1 treating
# normal stderr as a terminating NativeCommandError under stream redirection.
$body='$ErrorActionPreference="Stop"; $env:PSModulePath=(Join-Path $PSHOME "Modules")+";"+$env:PSModulePath; Start-Transcript -Path '+(Quote-Literal $log)+' | Out-Null; try { '+$command+'; exit 0 } catch { Write-Output $_; exit 1 } finally { Stop-Transcript | Out-Null }'
$encoded=[Convert]::ToBase64String([Text.Encoding]::Unicode.GetBytes($body))
# The UIA driver uses Windows PowerShell's in-box desktop assemblies. Resolve
# that executable explicitly, including when the launcher is called from pwsh.
$testShell=Join-Path $env:WINDIR 'System32/WindowsPowerShell/v1.0/powershell.exe'
$code=[PrivateSmoke]::Run($testShell,$encoded,(Get-Location).Path)
Get-Content -LiteralPath $log -Encoding UTF8 | Where-Object {$_ -notmatch '^主机应用程序:|^-EncodedCommand|^Host Application:'} | Select-Object -Last 20
if($code -ne 0){throw "Private desktop smoke failed; see $log"}
