param([string]$Exe,[string]$Work,[string]$ScreenshotTool,[string]$TranslationEndpoint,[string]$ModelsRoot,[switch]$Packaged,[string]$AsrModel='Qwen3-ASR-1.7B-hf')
$ErrorActionPreference='Stop'
Add-Type -AssemblyName UIAutomationClient
Add-Type -AssemblyName UIAutomationTypes
Add-Type -AssemblyName System.Speech
Add-Type -AssemblyName System.Drawing
# Test actual desktop pixels: successful decoding and a running clock can
# coexist with an invisible composition-hosted WebView.
Add-Type -ReferencedAssemblies System.Drawing @'
using System;
using System.Drawing;
using System.Runtime.InteropServices;
public static class PipelinePicture {
 [StructLayout(LayoutKind.Sequential)] public struct Rect {public int L,T,R,B;}
 [DllImport("user32.dll")] public static extern bool SetProcessDPIAware();
 [DllImport("user32.dll")] static extern bool GetWindowRect(IntPtr h,out Rect r);
 [DllImport("user32.dll")] static extern bool PrintWindow(IntPtr h,IntPtr dc,uint flags);
 public static bool Visible(IntPtr h,int x,int y,int w,int height) {
  Rect r;GetWindowRect(h,out r);
  using(var image=new Bitmap(r.R-r.L,r.B-r.T)) {
   using(var g=Graphics.FromImage(image)) {
    var dc=g.GetHdc();try {if(!PrintWindow(h,dc,2))throw new Exception("PrintWindow failed");}finally{g.ReleaseHdc(dc);}
   }
   int colorful=0;
   for(int yy=y-r.T+8;yy<y-r.T+height-8;yy+=8)
    for(int xx=x-r.L+8;xx<x-r.L+w-8;xx+=8) {
     var p=image.GetPixel(xx,yy);
     if(Math.Max(p.R,Math.Max(p.G,p.B))-Math.Min(p.R,Math.Min(p.G,p.B))>60)colorful++;
    }
   return colorful>100;
  }
 }
}
'@
[void][PipelinePicture]::SetProcessDPIAware()
$workPath=[IO.Path]::GetFullPath($Work)
if(Test-Path -LiteralPath $workPath){throw 'Use a fresh test directory'}
New-Item -ItemType Directory -Path $workPath | Out-Null
$voice=Join-Path $workPath 'speech.wav'
$speech=[System.Speech.Synthesis.SpeechSynthesizer]::new()
try {
 $english=$speech.GetInstalledVoices() | Where-Object {$_.VoiceInfo.Culture.Name -like 'en-*'} | Select-Object -First 1
 if($english){$speech.SelectVoice($english.VoiceInfo.Name)}
 $speech.SetOutputToWaveFile($voice)
 $speech.Speak('Hello world. This is a subtitle test. Thank you for watching.')
} finally {$speech.Dispose()}
$video=Join-Path $workPath 'pipeline.mp4'
& ffmpeg -v error -f lavfi -i 'testsrc2=size=320x180:rate=10:duration=15' -i $voice -shortest -c:v libx264 -pix_fmt yuv420p -c:a aac $video
if($LASTEXITCODE -ne 0){throw 'Speech fixture creation failed'}
$env:ONESUB_DATA_ROOT=Join-Path $workPath 'data'
$env:WEBVIEW2_USER_DATA_FOLDER=Join-Path $workPath 'webview'
$env:XDG_RUNTIME_DIR=Join-Path $workPath 'ipc'
if($env:XDG_RUNTIME_DIR.Length -gt 65){throw 'Use a shorter test directory for the isolated IPC socket'}
New-Item -ItemType Directory -Path $env:ONESUB_DATA_ROOT,$env:XDG_RUNTIME_DIR | Out-Null
$prefs=@{version=5;uiLanguage='zh-Hans';closeToTray=$false;recognitionMode='manual';asrModel=$AsrModel;aligner='Qwen3-ForcedAligner-0.6B-hf';language='en';backend='auto';providerUrl=$TranslationEndpoint;providerModel='local-protocol-test';targetLanguage='zh-CN';optimizeTimeline=$false;translationConcurrency=2;translationBatchSize=20;modelPaths=@{$AsrModel=(Join-Path $ModelsRoot $AsrModel);'Qwen3-ForcedAligner-0.6B-hf'=(Join-Path $ModelsRoot 'Qwen3-ForcedAligner-0.6B-hf')}}
[IO.File]::WriteAllText((Join-Path $env:ONESUB_DATA_ROOT 'settings.json'),($prefs|ConvertTo-Json -Depth 5),[Text.UTF8Encoding]::new($false))
if($Packaged){foreach($name in @('ONESUB_FFPROBE','ONESUB_FFMPEG','ONESUB_SUBTITLE_WORKER','ONESUB_STYLE_WORKER','ONESUB_SKILL_WORKER','ONESUB_SKILL_ROOT','ONESUB_ONEASR_ROOT')){Remove-Item "Env:$name" -ErrorAction SilentlyContinue}}
else {$env:ONESUB_FFPROBE=(Get-Command ffprobe).Source;$env:ONESUB_FFMPEG=(Get-Command ffmpeg).Source}
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
function Click-UI($name) {
 $node=Wait-UI $name
 $node.GetCurrentPattern([System.Windows.Automation.InvokePattern]::Pattern).Invoke()
}
function Check-Picture($stage) {
 for($i=0;$i -lt 100;$i++) {
  $viewport=Find-UI '媒体视口'
  if($viewport) {
   $r=$viewport.Current.BoundingRectangle
   if($r.Width -gt 100 -and $r.Height -gt 100 -and [PipelinePicture]::Visible($p.MainWindowHandle,$r.X,$r.Y,$r.Width,$r.Height)){return}
  }
  Start-Sleep -Milliseconds 100
 }
 if($ScreenshotTool){& $ScreenshotTool $p.Id (Join-Path $workPath "$stage-black.png") | Out-Host}
 throw "Video pixels missing: $stage"
}
try {
 [void](Wait-UI '一键识别并翻译')
 for($i=0;$i -lt 100;$i++){$node=Find-UI '一键识别并翻译';if($node -and $node.Current.IsEnabled){break};Start-Sleep -Milliseconds 100}
 Check-Picture 'import'
 Click-UI '一键识别并翻译'
 [void](Wait-UI '取消任务')
 Click-UI '取消任务'
 [void](Wait-UI '继续处理' 30)
 Check-Picture 'cancelled'
 Click-UI '继续处理'
 $translated=Join-Path $workPath 'pipeline-zh.srt'
 for($i=0;$i -lt 2400;$i++) {
  if(Test-Path -LiteralPath $translated){break}
  if($p.HasExited){throw 'Application exited during recognition/translation'}
  Start-Sleep -Milliseconds 100
 }
 if(-not (Test-Path -LiteralPath $translated)){
  & $ScreenshotTool $p.Id (Join-Path $workPath 'failure.png')
  $root=[System.Windows.Automation.AutomationElement]::FromHandle($p.MainWindowHandle)
  $root.FindAll([System.Windows.Automation.TreeScope]::Descendants,[System.Windows.Automation.Condition]::TrueCondition) | ForEach-Object {$_.Current.Name}
  throw 'One-click pipeline did not produce a translated file'
 }
 $original=Join-Path $workPath 'pipeline.srt'
 if(-not (Test-Path -LiteralPath $original)){throw 'Recognized subtitles were not preserved'}
 if(-not [IO.File]::ReadAllText($translated).Contains('你好')){throw 'Pipeline output has no translated content'}
 Check-Picture 'translated'
 Click-UI '播放'
 [void](Wait-UI '暂停')
 Start-Sleep -Milliseconds 500
 $position=(Find-UI '播放位置').GetCurrentPattern([System.Windows.Automation.RangeValuePattern]::Pattern).Current.Value
 if($position -le 0){throw 'Playback clock did not advance after translation'}
 Check-Picture 'playing'
 Click-UI '暂停'
 Check-Picture 'paused'
 & $ScreenshotTool $p.Id (Join-Path $workPath 'pipeline.png')
 Write-Output "PASS: one-click recognition and translation ($AsrModel); cancellation and continue; original and translated files; visible video before/after tasks and during playback"
} finally {
 $p.Refresh()
 if(-not $p.HasExited) {
  if(Find-UI '取消任务'){Click-UI '取消任务';Start-Sleep -Seconds 2}
  [void]$p.CloseMainWindow()
  for($i=0;$i -lt 100;$i++){if($p.HasExited){break};if(Find-UI '不保存'){Click-UI '不保存'};Start-Sleep -Milliseconds 100;$p.Refresh()}
  if(-not $p.WaitForExit(10000)){throw "Test application remains open: $($p.Id)"}
 }
}
