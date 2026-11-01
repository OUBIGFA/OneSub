param([string]$Exe, [string]$Work, [string]$ModelsRoot, [string]$ScreenshotTool, [switch]$Packaged)
$ErrorActionPreference = 'Stop'
Add-Type -AssemblyName UIAutomationClient
Add-Type -AssemblyName UIAutomationTypes
Add-Type -AssemblyName System.Speech
Add-Type @'
using System;
using System.Runtime.InteropServices;
public static class QueueNative {
    public delegate bool Visitor(IntPtr h, IntPtr p);
    [StructLayout(LayoutKind.Sequential)] public struct Point { public int X,Y; }
    [DllImport("user32.dll")] public static extern bool EnumChildWindows(IntPtr h, Visitor visit, IntPtr p);
    [DllImport("user32.dll")] public static extern uint GetWindowThreadProcessId(IntPtr h, out uint pid);
    [DllImport("user32.dll", CharSet=CharSet.Unicode)] public static extern int GetClassName(IntPtr h, System.Text.StringBuilder name, int length);
    [DllImport("user32.dll")] public static extern bool ScreenToClient(IntPtr h, ref Point p);
    [DllImport("user32.dll")] public static extern bool PostMessage(IntPtr h, uint message, IntPtr w, IntPtr l);
    public static bool Click(IntPtr window, int pid, int x, int y) {
        IntPtr surface=IntPtr.Zero;
        EnumChildWindows(window,(child,p)=>{
            uint owner; GetWindowThreadProcessId(child,out owner);
            var name=new System.Text.StringBuilder(128); GetClassName(child,name,128);
            if(owner==pid && name.ToString()=="MyGoSurface"){surface=child;return false;}
            return true;
        },IntPtr.Zero);
        if(surface==IntPtr.Zero)return false;
        var point=new Point{X=x,Y=y};
        if(!ScreenToClient(surface,ref point))return false;
        var coords=new IntPtr((point.Y<<16)|(point.X&0xffff));
        return PostMessage(surface,512,IntPtr.Zero,coords) && PostMessage(surface,513,new IntPtr(1),coords) && PostMessage(surface,514,IntPtr.Zero,coords);
    }
}
'@
$root = (Get-Location).Path
if (-not (Test-Path (Join-Path $root 'apps/desktop/go.mod'))) { throw 'Run the queue test from the repository root' }
$workPath = [IO.Path]::GetFullPath($Work)
if (Test-Path $workPath) { throw 'Use a fresh queue test directory' }
$asr = Join-Path $ModelsRoot 'Qwen3-ASR-1.7B-hf'
$aligner = Join-Path $ModelsRoot 'Qwen3-ForcedAligner-0.6B-hf'
if (-not (Test-Path $asr) -or -not (Test-Path $aligner)) { throw 'Existing opt-in recognition models are required; this test never downloads them' }
$media = Join-Path $workPath 'media'
$output = Join-Path $workPath 'output'
$env:ONESUB_DATA_ROOT = Join-Path $workPath 'data'
$env:WEBVIEW2_USER_DATA_FOLDER = Join-Path $workPath 'webview'
$env:XDG_RUNTIME_DIR = Join-Path $workPath 'ipc'
if ($env:XDG_RUNTIME_DIR.Length -gt 65) { throw 'Use a shorter queue test directory for IPC' }
New-Item -ItemType Directory -Path $media,$output,$env:ONESUB_DATA_ROOT,$env:XDG_RUNTIME_DIR | Out-Null
$first = Join-Path $media 'first.wav'
$second = Join-Path $media 'second.wav'
$bad = Join-Path $media 'unreadable.mp4'
[IO.File]::WriteAllText($bad,'not a media container',[Text.UTF8Encoding]::new($false))
$speech = [System.Speech.Synthesis.SpeechSynthesizer]::new()
try {
    $voice = $speech.GetInstalledVoices() | Where-Object {$_.VoiceInfo.Culture.Name -like 'en-*'} | Select-Object -First 1
    if ($voice) { $speech.SelectVoice($voice.VoiceInfo.Name) }
    $speech.SetOutputToWaveFile($first)
    $speech.Speak('Hello world. This is a subtitle test. Thank you for watching.')
} finally { $speech.Dispose() }
Copy-Item $first $second
$current = Join-Path $workPath 'current.srt'
$currentText = "1`n00:00:00,000 --> 00:00:02,000`nKeep the current subtitles unchanged.`n"
[IO.File]::WriteAllText($current,$currentText,[Text.UTF8Encoding]::new($false))
$prefs = @{
    version=5;uiLanguage='zh-Hans';closeToTray=$false;appearance='light';topHeight=40;
    asrModel='Qwen3-ASR-1.7B-hf';aligner='Qwen3-ForcedAligner-0.6B-hf';language='en';backend='auto';
    chunkSeconds=30;maxTokens=1024;recognitionFormat='vtt';recognitionDir=$output;stripPunctuation=$false;
    batchSources=@($bad,$first,$second);modelPaths=@{'Qwen3-ASR-1.7B-hf'=$asr;'Qwen3-ForcedAligner-0.6B-hf'=$aligner}
}
$settingsPath = Join-Path $env:ONESUB_DATA_ROOT 'settings.json'
[IO.File]::WriteAllText($settingsPath,($prefs|ConvertTo-Json -Depth 6),[Text.UTF8Encoding]::new($false))
if ($Packaged) {
    foreach($name in @('ONESUB_FFPROBE','ONESUB_FFMPEG','ONESUB_SUBTITLE_WORKER','ONESUB_STYLE_WORKER','ONESUB_SKILL_WORKER','ONESUB_SKILL_ROOT','ONESUB_ONEASR_ROOT')) { Remove-Item "Env:$name" -ErrorAction SilentlyContinue }
} else {
    $env:ONESUB_FFMPEG=(Get-Command ffmpeg).Source
    $env:ONESUB_FFPROBE=(Get-Command ffprobe).Source
    $env:ONESUB_SUBTITLE_WORKER=Join-Path $root 'build/output/subtitle-core/OneSub.SubtitleCore.exe'
    $env:ONESUB_STYLE_WORKER=Join-Path $root 'build/output/style-core/OneSub.StyleCore.exe'
    $env:ONESUB_SKILL_WORKER=Join-Path $root 'build/output/skill-tools/OneSub.SkillTools.exe'
    $env:ONESUB_SKILL_ROOT=Join-Path $root 'third_party/skills/srt-subtitle-translator'
    $env:ONESUB_ONEASR_ROOT=Join-Path $root 'build/output/oneasr'
}
$p=Start-Process -FilePath ([IO.Path]::GetFullPath($Exe)) -ArgumentList ('"'+$current+'"') -PassThru -RedirectStandardError (Join-Path $workPath 'stderr.log')
function Find-UI($name) {
    $p.Refresh()
    if($p.HasExited -or $p.MainWindowHandle -eq [IntPtr]::Zero) { return $null }
    $window=[System.Windows.Automation.AutomationElement]::FromHandle($p.MainWindowHandle)
    return $window.FindFirst([System.Windows.Automation.TreeScope]::Descendants,[System.Windows.Automation.PropertyCondition]::new([System.Windows.Automation.AutomationElement]::NameProperty,$name))
}
function Wait-UI($name) {
    for($i=0;$i -lt 200;$i++) { $node=Find-UI $name;if($node){return $node};Start-Sleep -Milliseconds 100 }
    throw "Control unavailable: $name"
}
function Click-UI($name) { (Wait-UI $name).GetCurrentPattern([System.Windows.Automation.InvokePattern]::Pattern).Invoke();Start-Sleep -Milliseconds 200 }
function Select-Value($name) {
    $node=Wait-UI $name
    $pattern=$null
    if($node.TryGetCurrentPattern([System.Windows.Automation.SelectionItemPattern]::Pattern,[ref]$pattern)){$pattern.Select()}
    else{$node.GetCurrentPattern([System.Windows.Automation.InvokePattern]::Pattern).Invoke()}
    Start-Sleep -Milliseconds 200
}
function Wait-Idle($seconds) {
    for($i=0;$i -lt $seconds*5;$i++) {
        $p.Refresh();if($p.HasExited){throw 'Queue test process exited during recognition'}
        if(-not (Find-UI '取消任务')){return}
        Start-Sleep -Milliseconds 200
    }
    throw 'Recognition queue did not finish before its deadline'
}
try {
    [void](Wait-UI '字幕文字')
    $start=Wait-UI '开始识别队列'
    if(-not $start.Current.IsEnabled){throw 'Restored queue is not ready to start'}
    Click-UI '开始识别队列'
    [void](Wait-UI '取消任务')
    # The queue inherits Current file settings without duplicating the form.
    # Editing there applies to the next task; this queue keeps frozen VTT.
    if(Find-UI '识别设置'){throw 'Queue repeats current-file settings'}
    Select-Value '当前文件'
    $format=Wait-UI '文件格式'
    $condition=[System.Windows.Automation.PropertyCondition]::new([System.Windows.Automation.AutomationElement]::ControlTypeProperty,[System.Windows.Automation.ControlType]::ComboBox)
    $combo=$format.FindFirst([System.Windows.Automation.TreeScope]::Descendants,$condition)
    if(-not $combo){$combo=$format}
    $combo.GetCurrentPattern([System.Windows.Automation.ExpandCollapsePattern]::Pattern).Expand()
    Start-Sleep -Milliseconds 200
    # Select's popup rows require native pointer input in this pinned MyGo
    # version. Resolve the real option bounds and address only our own surface.
    $bounds=(Wait-UI 'SRT').Current.BoundingRectangle
    if(-not [QueueNative]::Click($p.MainWindowHandle,$p.Id,[int]($bounds.X+$bounds.Width/2),[int]($bounds.Y+$bounds.Height/2))){throw 'Native format selection failed'}
    Start-Sleep -Milliseconds 300
    $saved=[IO.File]::ReadAllText($settingsPath)|ConvertFrom-Json
    if($saved.recognitionFormat -ne 'srt'){throw 'Next-task output format did not save'}
    Select-Value '文件队列'
    Wait-Idle 360
    $files=@(Get-ChildItem $output -File)
    if($files.Count -ne 2 -or ($files|Where-Object {$_.Extension -ne '.vtt'})){throw 'Queue did not inherit frozen VTT format and output folder'}
    foreach($file in $files) {
        $text=[IO.File]::ReadAllText($file.FullName)
        if(-not $text.StartsWith('WEBVTT') -or $text -notmatch '(?i)hello'){throw 'Real recognition output is empty or not VTT'}
    }
    if(@(Get-ChildItem $media -File).Count -ne 3){throw 'Recognition wrote sidecars or subtitles beside the media despite a selected output folder'}
    if([IO.File]::ReadAllText($current) -ne $currentText -or -not (Find-UI 'Keep the current subtitles unchanged.')){throw 'Queue replaced the workbench subtitles'}
    [void](Wait-UI '批量识别完成：2 个成功，1 个失败，共 3 个。')
    $hashes=@($files|ForEach-Object {(Get-FileHash $_.FullName -Algorithm SHA256).Hash})
    Click-UI '开始识别队列'
    Start-Sleep -Milliseconds 500
    Wait-Idle 30
    if(@(Get-ChildItem $output -File).Count -ne 2){throw 'Retry processed already successful files again'}
    $again=@($files|ForEach-Object {(Get-FileHash $_.FullName -Algorithm SHA256).Hash})
    if(($hashes -join ',') -ne ($again -join ',')){throw 'Retry modified successful outputs'}
    if($ScreenshotTool){& $ScreenshotTool $p.Id (Join-Path $workPath 'queue-result.png')}
    Write-Output 'PASS: real queue uses shared ASR/language/decoding settings, frozen VTT and selected directory; one failed file does not stop two successful files; retry skips successes; current subtitles unchanged; no sidecars'
} finally {
    $p.Refresh()
    if(-not $p.HasExited){
        if(Find-UI '取消任务'){Click-UI '取消任务';Wait-Idle 30}
        [void]$p.CloseMainWindow()
        if(-not $p.WaitForExit(10000)){$p.Kill();$p.WaitForExit();throw 'Queue test window did not close normally'}
    }
}
