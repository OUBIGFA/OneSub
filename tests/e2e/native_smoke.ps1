param([string]$Exe, [string]$Work, [string]$ScreenshotTool, [switch]$OpenArgument, [string]$TranslationEndpoint, [switch]$Packaged)
$ErrorActionPreference = 'Stop'
Add-Type -AssemblyName UIAutomationClient
Add-Type -AssemblyName UIAutomationTypes
Add-Type @'
using System;
using System.Runtime.InteropServices;
public static class SmokeNative {
  public delegate bool Visitor(IntPtr h, IntPtr p);
  [DllImport("user32.dll")] public static extern bool EnumWindows(Visitor fn, IntPtr p);
  [DllImport("user32.dll")] public static extern bool EnumChildWindows(IntPtr h, Visitor fn, IntPtr p);
  [DllImport("user32.dll")] public static extern uint GetWindowThreadProcessId(IntPtr h, out uint p);
  [DllImport("user32.dll")] public static extern int GetDlgCtrlID(IntPtr h);
  [DllImport("user32.dll", CharSet=CharSet.Unicode)] public static extern int GetClassName(IntPtr h, System.Text.StringBuilder b, int n);
  [DllImport("user32.dll", CharSet=CharSet.Unicode)] public static extern IntPtr SendMessage(IntPtr h, uint m, IntPtr w, string text);
  [DllImport("user32.dll")] public static extern bool PostMessage(IntPtr h, uint m, IntPtr w, IntPtr l);
  public static bool Choose(int pid, string path) {
    bool done=false;
    EnumWindows((dialog,p)=>{
      uint id; GetWindowThreadProcessId(dialog,out id);
      var name=new System.Text.StringBuilder(128);GetClassName(dialog,name,128);
      if(id!=pid||name.ToString()!="#32770")return true;
      EnumChildWindows(dialog,(child,q)=>{
        var cl=new System.Text.StringBuilder(128);GetClassName(child,cl,128);
        int control=GetDlgCtrlID(child);
        if(cl.ToString()=="Edit"&&(control==1148||control==1001)){
          SendMessage(child,12,IntPtr.Zero,path);
          PostMessage(dialog,273,new IntPtr(1),IntPtr.Zero);done=true;return false;
        }
        return true;
      },IntPtr.Zero);
      return !done;
    },IntPtr.Zero);
    return done;
  }
}
'@
$workPath = [IO.Path]::GetFullPath($Work)
if (Test-Path $workPath) { throw 'Use a fresh smoke directory' }
New-Item -ItemType Directory $workPath | Out-Null
$video = Join-Path $workPath 'test.mp4'
$subtitle = Join-Path $workPath 'test.srt'
$sourceText = if ($TranslationEndpoint) { 'Hello, world' } else { '测试字幕' }
[IO.File]::WriteAllText($subtitle,"1`n00:00:00,100 --> 00:00:02,900`n$sourceText`n",[Text.UTF8Encoding]::new($false))
& ffmpeg -v error -f lavfi -i 'testsrc2=size=320x180:rate=10:duration=3' -c:v libx264 -pix_fmt yuv420p $video
if($LASTEXITCODE -ne 0){throw 'Fixture creation failed'}
$env:ONESUB_DATA_ROOT = Join-Path $workPath 'data'
New-Item -ItemType Directory $env:ONESUB_DATA_ROOT | Out-Null
# Controls are found by their Chinese names; pin the interface language.
$prefs=@{version=1;uiLanguage='zh-Hans'}
if ($TranslationEndpoint) {
    # One translate request only: the fixture does not answer segmentation.
    $prefs+=@{providerUrl=$TranslationEndpoint;providerModel='local-protocol-test';targetLanguage='zh-CN';optimizeTimeline=$false}
}
[IO.File]::WriteAllText((Join-Path $env:ONESUB_DATA_ROOT 'settings.json'),($prefs|ConvertTo-Json),[Text.UTF8Encoding]::new($false))
if($Packaged){
    foreach($name in @('ONESUB_FFPROBE','ONESUB_FFMPEG','ONESUB_SUBTITLE_WORKER','ONESUB_STYLE_WORKER','ONESUB_SKILL_WORKER','ONESUB_SKILL_ROOT')){Remove-Item "Env:$name" -ErrorAction SilentlyContinue}
}else{
    $env:ONESUB_FFPROBE = (Get-Command ffprobe).Source
    $env:ONESUB_FFMPEG = (Get-Command ffmpeg).Source
}
$env:WEBVIEW2_USER_DATA_FOLDER = Join-Path $workPath 'webview'
# MyGo's single-instance socket must be isolated as well as the project
# data. Otherwise a packaged smoke run activates the user's installed app.
$env:XDG_RUNTIME_DIR = Join-Path $workPath 'ipc'
if($env:XDG_RUNTIME_DIR.Length -gt 65){throw 'Use a shorter smoke directory for the isolated single-instance socket'}
New-Item -ItemType Directory $env:XDG_RUNTIME_DIR | Out-Null
$launch=@{FilePath=[IO.Path]::GetFullPath($Exe);PassThru=$true;RedirectStandardError=(Join-Path $workPath 'stderr.log')}
if($OpenArgument){$launch.ArgumentList=@('"'+$video+'"')}
$p=Start-Process @launch
function Find-UI($name) {
    $p.Refresh()
    if($p.HasExited -or -not $p.MainWindowHandle){return $null}
    $root=[System.Windows.Automation.AutomationElement]::FromHandle($p.MainWindowHandle)
    if(-not $root){return $null}
    $condition=[System.Windows.Automation.PropertyCondition]::new([System.Windows.Automation.AutomationElement]::NameProperty,$name)
    return $root.FindFirst([System.Windows.Automation.TreeScope]::Descendants,$condition)
}
function Click-UI($name) {
    for($i=0;$i -lt 100;$i++) {
        $item=Find-UI $name
        if($item -and $item.Current.IsEnabled) {
            try {
                $invoke=$item.GetCurrentPattern([System.Windows.Automation.InvokePattern]::Pattern)
                $invoke.Invoke();return
            } catch [System.Windows.Automation.ElementNotAvailableException] {
                # A native immediate-mode redraw can invalidate a UIA node.
                # Reacquire by label rather than keeping a stale element handle.
            }
        }
        Start-Sleep -Milliseconds 100
    }
    throw "Control unavailable: $name"
}
function Select-Tab($container, $name) {
    # Invoke/selection is asynchronous; wait for the next UIA tree after
    # closing a modal instead of reading the just-invalidated tree once.
    for($i=0;$i -lt 100;$i++) {
        try {
            $tabs=Find-UI $container
            if($tabs) {
                $condition=[System.Windows.Automation.PropertyCondition]::new([System.Windows.Automation.AutomationElement]::NameProperty,$name)
                $item=$tabs.FindFirst([System.Windows.Automation.TreeScope]::Descendants,$condition)
                if($item) {
                    $pattern=$null
                    if($item.TryGetCurrentPattern([System.Windows.Automation.SelectionItemPattern]::Pattern,[ref]$pattern)){$pattern.Select()}
                    else{$item.GetCurrentPattern([System.Windows.Automation.InvokePattern]::Pattern).Invoke()}
                    Start-Sleep -Milliseconds 300
                    return
                }
            }
        } catch [System.Windows.Automation.ElementNotAvailableException] {
            # Reacquire after an immediate-mode redraw, as Click-UI does.
        }
        Start-Sleep -Milliseconds 100
    }
    throw "Tab unavailable: $container / $name"
}
function Choose-Path($path){
    for($i=0;$i -lt 100;$i++) { if([SmokeNative]::Choose($p.Id,$path)){return};Start-Sleep -Milliseconds 100 }
    throw 'File dialog did not appear'
}
try {
    Start-Sleep -Seconds 4
    if($p.HasExited){throw "Test application exited at startup: $($p.ExitCode)"}
    if(-not $OpenArgument){Click-UI '添加文件'; Choose-Path $video}
    for($i=0;$i -lt 150;$i++){if(Find-UI '字幕文字'){break};Start-Sleep -Milliseconds 100}
    if(-not (Find-UI '字幕文字')){
        & $ScreenshotTool $p.Id (Join-Path $workPath 'failure.png')
        $p.Refresh();$root=[System.Windows.Automation.AutomationElement]::FromHandle($p.MainWindowHandle)
        $root.FindAll([System.Windows.Automation.TreeScope]::Descendants,[System.Windows.Automation.Condition]::TrueCondition) | ForEach-Object {$_.Current.Name}
        throw 'Same-name subtitle was not loaded'
    }
    Click-UI '播放'
    Start-Sleep -Seconds 2
    if(-not (Find-UI '暂停')){
        & $ScreenshotTool $p.Id (Join-Path $workPath 'failure.png')
        throw 'The native play button did not start real video playback'
    }
    Click-UI '暂停'
    & $ScreenshotTool $p.Id (Join-Path $workPath 'editor.png')
    $bold=Find-UI '粗体'
    if($bold){
        $bold.GetCurrentPattern([System.Windows.Automation.TogglePattern]::Pattern).Toggle()
        Start-Sleep -Seconds 2
        & $ScreenshotTool $p.Id (Join-Path $workPath 'style-bold.png')
    }
    Click-UI '设置'
    & $ScreenshotTool $p.Id (Join-Path $workPath 'settings.png')
    if(Find-UI '设置分类'){
        foreach($tab in @('识别与模型','翻译服务')){
            Select-Tab '设置分类' $tab
            & $ScreenshotTool $p.Id (Join-Path $workPath ("settings-"+$tab+'.png'))
        }
    }
    Click-UI '关闭'
    if ($TranslationEndpoint) {
        Select-Tab '识别与翻译' 'AI 翻译'
        Click-UI '开始翻译字幕'
        for($i=0;$i -lt 100;$i++){if(Find-UI '实时译文'){break};Start-Sleep -Milliseconds 100}
        if(-not (Find-UI '实时译文')){throw 'Streaming translation view did not appear'}
        for($i=0;$i -lt 100;$i++){if(Find-UI '你好'){break};Start-Sleep -Milliseconds 100}
        if(-not (Find-UI '你好')){throw 'Partial streamed text did not appear'}
        $translated=Join-Path $workPath 'test-zh.srt'
        if(Test-Path $translated){throw 'Partial translation was published before completion'}
        & $ScreenshotTool $p.Id (Join-Path $workPath 'translation-live.png')
        for($i=0;$i -lt 200;$i++){if(Test-Path $translated){break};Start-Sleep -Milliseconds 100}
        if(-not (Test-Path $translated)){throw 'Translation was not automatically saved'}
        if(-not [IO.File]::ReadAllText($translated).Contains('你好，世界')){throw 'Saved translation has no translated text'}
        if(-not [IO.File]::ReadAllText($subtitle).Contains($sourceText)){throw 'Source subtitle was overwritten'}
        if(-not (Find-UI '你好，世界')){throw 'Finished translation did not update the subtitle table'}
        Start-Sleep -Seconds 2
        & $ScreenshotTool $p.Id (Join-Path $workPath 'translation.png')
        Click-UI '撤销'
        Select-Tab '字幕语言' '原文'
        for($i=0;$i -lt 100;$i++){if(Find-UI $sourceText){break};Start-Sleep -Milliseconds 100}
        if(-not (Find-UI $sourceText)){throw 'Translation undo did not restore the original'}
    }
    Write-Output "PASS: import (argument=$OpenArgument), same-name subtitle, real playback/pause, settings, translation=$([bool]$TranslationEndpoint)"
} finally {
    for($i=0;$i -lt 200;$i++){if(-not (Find-UI '取消任务')){break};Start-Sleep -Milliseconds 100}
    $p.Refresh()
    if(-not $p.HasExited){
        $null=$p.CloseMainWindow()
        for($i=0;$i -lt 100;$i++){
            if($p.HasExited){break}
            # Undo leaves the generated translation file out of sync with
            # this disposable project. Dismiss only this test instance's guard.
            if(Find-UI '不保存'){Click-UI '不保存'}
            Start-Sleep -Milliseconds 100
            $p.Refresh()
        }
        if(-not $p.WaitForExit(10000)){throw "Test window remains open: PID=$($p.Id)"}
    }
}
