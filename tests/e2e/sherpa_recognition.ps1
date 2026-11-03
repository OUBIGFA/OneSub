# Native desktop recognition with a sherpa-onnx model: open a video in the
# real window, start recognition, and check the subtitle file it saves.
# ModelsRoot holds one folder per model ID and test_wavs, as for the Go
# integration tests. No model is downloaded and no network is used.
param([string]$Exe,[string]$Work,[string]$ModelsRoot,[string]$ScreenshotTool,[string]$AsrModel='sherpa-sense-voice-small-int8',[string]$Language='zh',[string]$Wav='zh.wav',[string]$Expect=(-join [char[]]@(0x4E0B,0x5348,0x0035,0x70B9)),[switch]$Packaged)
$ErrorActionPreference='Stop'
# Windows PowerShell 5 reads a script without a BOM in the ANSI code page,
# so the window's Chinese names are built from their code points.
$startLabel=(-join [char[]]@(0x5F00,0x59CB,0x8BC6,0x522B,0x5B57,0x5E55))
$discardLabel=(-join [char[]]@(0x4E0D,0x4FDD,0x5B58))
Add-Type -AssemblyName UIAutomationClient
Add-Type -AssemblyName UIAutomationTypes
$workPath=[IO.Path]::GetFullPath($Work)
if(Test-Path -LiteralPath $workPath){throw 'Use a fresh test directory'}
New-Item -ItemType Directory -Path $workPath | Out-Null
$video=Join-Path $workPath 'sherpa.mp4'
# Speech, 20 s of silence, the same speech.
& ffmpeg -v error -f lavfi -i 'testsrc2=size=320x180:rate=10:duration=40' -i (Join-Path $ModelsRoot "test_wavs/$Wav") -f lavfi -t 20 -i 'anullsrc=r=16000:cl=mono' -filter_complex '[1:a]asplit=2[a][b];[a][2:a][b]concat=n=3:v=0:a=1[out]' -map 0:v -map '[out]' -shortest -c:v libx264 -pix_fmt yuv420p -c:a aac $video
if($LASTEXITCODE -ne 0){throw 'Video fixture creation failed'}
$env:ONESUB_DATA_ROOT=Join-Path $workPath 'data'
$env:WEBVIEW2_USER_DATA_FOLDER=Join-Path $workPath 'webview'
$env:XDG_RUNTIME_DIR=Join-Path $workPath 'ipc'
if($env:XDG_RUNTIME_DIR.Length -gt 65){throw 'Use a shorter test directory for the isolated IPC socket'}
New-Item -ItemType Directory -Path $env:ONESUB_DATA_ROOT,$env:XDG_RUNTIME_DIR | Out-Null
$prefs=@{version=4;uiLanguage='zh-Hans';asrModel=$AsrModel;language=$Language;recognitionFormat='srt';closeToTray=$false;modelPaths=@{$AsrModel=(Join-Path $ModelsRoot $AsrModel)}}
[IO.File]::WriteAllText((Join-Path $env:ONESUB_DATA_ROOT 'settings.json'),($prefs|ConvertTo-Json -Depth 5),[Text.UTF8Encoding]::new($false))
if($Packaged){foreach($name in @('ONESUB_FFPROBE','ONESUB_FFMPEG','ONESUB_SUBTITLE_WORKER','ONESUB_STYLE_WORKER','ONESUB_SKILL_WORKER','ONESUB_SKILL_ROOT','ONESUB_ONEASR_ROOT','ONESUB_SHERPA_ROOT')){Remove-Item "Env:$name" -ErrorAction SilentlyContinue}}
$p=Start-Process -FilePath ([IO.Path]::GetFullPath($Exe)) -ArgumentList ('"'+$video+'"') -PassThru -RedirectStandardError (Join-Path $workPath 'stderr.log')
function Find-UI($name) {
 $p.Refresh()
 if($p.HasExited -or $p.MainWindowHandle -eq [IntPtr]::Zero){return $null}
 $root=[System.Windows.Automation.AutomationElement]::FromHandle($p.MainWindowHandle)
 return $root.FindFirst([System.Windows.Automation.TreeScope]::Descendants,[System.Windows.Automation.PropertyCondition]::new([System.Windows.Automation.AutomationElement]::NameProperty,$name))
}
function Wait-UI($name,$seconds=20) {
 for($i=0;$i -lt $seconds*10;$i++){$node=Find-UI $name;if($node){return $node};Start-Sleep -Milliseconds 100}
 throw "Control unavailable: $name"
}
try {
 [void](Wait-UI $startLabel)
 for($i=0;$i -lt 100;$i++){$node=Find-UI $startLabel;if($node -and $node.Current.IsEnabled){break};Start-Sleep -Milliseconds 100}
 $node.GetCurrentPattern([System.Windows.Automation.InvokePattern]::Pattern).Invoke()
 $output=Join-Path $workPath 'sherpa.srt'
 for($i=0;$i -lt 1200;$i++) {
  if(Test-Path -LiteralPath $output){break}
  if($p.HasExited){throw 'Application exited during recognition'}
  Start-Sleep -Milliseconds 100
 }
 if(-not (Test-Path -LiteralPath $output)){
  if($ScreenshotTool){& $ScreenshotTool $p.Id (Join-Path $workPath 'failure.png') | Out-Host}
  throw 'Recognition did not save a subtitle file'
 }
 Start-Sleep -Seconds 2
 $text=[IO.File]::ReadAllText($output,[Text.UTF8Encoding]::new($false))
 $blocks=@($text.Trim() -split "\r?\n\r?\n")
 if($blocks.Count -lt 2 -or -not $text.Contains($Expect)){throw "Unexpected subtitles: $text"}
 # No cue may start in the 20 s of silence after the first speech. A first
 # word may begin inside the 250 ms of context before speech.
 $speech=[double](& ffprobe -v error -show_entries format=duration -of csv=p=0 (Join-Path $ModelsRoot "test_wavs/$Wav"))
 foreach($block in $blocks){
  if($block -match '(\d\d):(\d\d):(\d\d),(\d\d\d) -->'){
   $start=[int]$Matches[2]*60+[int]$Matches[3]+[int]$Matches[4]/1000
   if($start -gt $speech -and $start -lt $speech+20-0.25){throw "A cue starts in the silence: $block"}
  }
 }
 if($ScreenshotTool){& $ScreenshotTool $p.Id (Join-Path $workPath 'recognized.png') | Out-Host}
 Write-Output "PASS: $AsrModel recognized in the native window; $($blocks.Count) cues saved to $output, none in the silence"
 Write-Output $text
} finally {
 $p.Refresh()
 if(-not $p.HasExited) {
  [void]$p.CloseMainWindow()
  for($i=0;$i -lt 100;$i++){if($p.HasExited){break};if(Find-UI $discardLabel){(Find-UI $discardLabel).GetCurrentPattern([System.Windows.Automation.InvokePattern]::Pattern).Invoke()};Start-Sleep -Milliseconds 100;$p.Refresh()}
  if(-not $p.WaitForExit(10000)){throw "Test application remains open: $($p.Id)"}
 }
}
