param([string]$Exe, [string]$Work, [string]$ScreenshotTool, [switch]$OpenArgument, [string]$TranslationEndpoint, [switch]$Packaged, [switch]$InterruptTranslation, [switch]$UIAPlayback, [switch]$BilingualExport, [switch]$SubtitleMenus, [switch]$LayoutNavigation, [switch]$WorkflowLayout, [switch]$ComposeOutput, [string]$ProviderCatalogEndpoint, [ValidateSet('dark','light')][string]$Appearance = 'dark')
$ErrorActionPreference = 'Stop'
if($SubtitleMenus -and ($TranslationEndpoint -or $InterruptTranslation -or $BilingualExport)){throw 'Run subtitle menu presentation as a separate scenario'}
if($LayoutNavigation -and ($TranslationEndpoint -or $InterruptTranslation -or $BilingualExport -or $SubtitleMenus)){throw 'Run layout/navigation as a separate scenario'}
Add-Type -AssemblyName UIAutomationClient
Add-Type -AssemblyName UIAutomationTypes
Add-Type -AssemblyName System.Windows.Forms
Add-Type @'
using System;
using System.Runtime.InteropServices;
public static class SmokeNative {
  [DllImport("user32.dll")] public static extern bool SetForegroundWindow(IntPtr h);
  [DllImport("user32.dll")] public static extern bool ShowWindow(IntPtr h, int command);
  [DllImport("user32.dll")] public static extern IntPtr GetForegroundWindow();
  public delegate bool Visitor(IntPtr h, IntPtr p);
  [DllImport("user32.dll")] public static extern bool EnumWindows(Visitor fn, IntPtr p);
  [DllImport("user32.dll")] public static extern bool EnumChildWindows(IntPtr h, Visitor fn, IntPtr p);
  [DllImport("user32.dll")] public static extern uint GetWindowThreadProcessId(IntPtr h, out uint p);
  [DllImport("user32.dll")] public static extern int GetDlgCtrlID(IntPtr h);
  [DllImport("user32.dll")] public static extern IntPtr GetDlgItem(IntPtr h, int id);
  [DllImport("user32.dll", CharSet=CharSet.Unicode)] public static extern int GetClassName(IntPtr h, System.Text.StringBuilder b, int n);
  [DllImport("user32.dll", CharSet=CharSet.Unicode)] public static extern IntPtr SendMessage(IntPtr h, uint m, IntPtr w, string text);
  [DllImport("user32.dll")] public static extern bool PostMessage(IntPtr h, uint m, IntPtr w, IntPtr l);
  [StructLayout(LayoutKind.Sequential)] public struct Point { public int X,Y; }
  [DllImport("user32.dll")] public static extern bool ScreenToClient(IntPtr h, ref Point p);
  [DllImport("user32.dll", EntryPoint="SendMessageW")] public static extern IntPtr SendValue(IntPtr h,uint m,IntPtr w,IntPtr l);
  [DllImport("user32.dll")] public static extern int GetMenuItemCount(IntPtr h);
  [StructLayout(LayoutKind.Sequential)] public struct MenuItemInfo {
    public uint Size,Mask,Type,State,ID;
    public IntPtr SubMenu,Checked,Unchecked,Data,Text;
    public uint Length;
    public IntPtr Bitmap;
  }
  [DllImport("user32.dll",CharSet=CharSet.Unicode)] public static extern bool GetMenuItemInfo(IntPtr menu,uint item,bool byPosition,ref MenuItemInfo info);
  public static string[] MenuItems(int pid) {
    string[] items=null;
    EnumWindows((popup,p)=>{
      uint id;GetWindowThreadProcessId(popup,out id);
      var name=new System.Text.StringBuilder(128);GetClassName(popup,name,128);
      if(id!=pid||name.ToString()!="#32768")return true;
      IntPtr menu=SendValue(popup,0x1e1,IntPtr.Zero,IntPtr.Zero); // MN_GETHMENU
      IntPtr buffer=Marshal.AllocHGlobal(1024);
      try {
        var labels=new System.Collections.Generic.List<string>();
        for(uint i=0;i<GetMenuItemCount(menu);i++) {
          var info=new MenuItemInfo{Size=(uint)Marshal.SizeOf(typeof(MenuItemInfo)),Mask=0x141,Text=buffer,Length=512};
          if(!GetMenuItemInfo(menu,i,true,ref info))return false;
          labels.Add((info.Type&0x800)!=0?"-":Marshal.PtrToStringUni(buffer).Split('\t')[0]);
        }
        items=labels.ToArray();
      } finally {Marshal.FreeHGlobal(buffer);}
      return false;
    },IntPtr.Zero);
    return items;
  }
  public static bool RightClick(IntPtr window, int pid, int x, int y) {
    return PointerClick(window,pid,x,y,true);
  }
  public static bool PointerClick(IntPtr window, int pid, int x, int y, bool right) {
    // Send native pointer messages only to this test instance's MyGo surface.
    IntPtr surface=IntPtr.Zero;
    EnumChildWindows(window,(child,p)=>{
      uint id;GetWindowThreadProcessId(child,out id);
      var name=new System.Text.StringBuilder(128);GetClassName(child,name,128);
      if(id==pid&&name.ToString()=="MyGoSurface"){surface=child;return false;}
      return true;
    },IntPtr.Zero);
    if(surface==IntPtr.Zero)return false;
    var point=new Point{X=x,Y=y};
    if(!ScreenToClient(surface,ref point))return false;
    var coords=new IntPtr((point.Y<<16)|(point.X&0xffff));
    return PostMessage(surface,512,IntPtr.Zero,coords)&&PostMessage(surface,right?516u:513u,new IntPtr(right?2:1),coords)&&PostMessage(surface,right?517u:514u,IntPtr.Zero,coords);
  }
  public static bool Choose(int pid, string path) {
    bool done=false;
    EnumWindows((dialog,p)=>{
      uint id; GetWindowThreadProcessId(dialog,out id);
      var name=new System.Text.StringBuilder(128);GetClassName(dialog,name,128);
      if(id!=pid||name.ToString()!="#32770")return true;
      // IFileDialog creates its HWND before applying the initial folder and
      // filename. Wait for that initialization, as the polish test does.
      System.Threading.Thread.Sleep(500);
      EnumChildWindows(dialog,(child,q)=>{
        var cl=new System.Text.StringBuilder(128);GetClassName(child,cl,128);
        int control=GetDlgCtrlID(child);
        if(cl.ToString()=="Edit"&&(control==1148||control==1001)){
          SendMessage(child,12,IntPtr.Zero,path);
          // Invoke the real dialog button after setting the filename.
          var button=GetDlgItem(dialog,1);
          if(button==IntPtr.Zero)return true;
          SendValue(button,245,IntPtr.Zero,IntPtr.Zero);done=true;return false;
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
$sourceText = if ($TranslationEndpoint) { 'Hello, world' } elseif ($BilingualExport) { "Upper subtitle`n下行字幕" } else { '测试字幕' }
[IO.File]::WriteAllText($subtitle,"1`n00:00:00,100 --> 00:00:02,900`n$sourceText`n",[Text.UTF8Encoding]::new($false))
if($InterruptTranslation){[IO.File]::WriteAllText($subtitle,"1`n00:00:00,100 --> 00:00:01,400`n$sourceText`n`n2`n00:00:01,500 --> 00:00:02,900`nGood morning`n",[Text.UTF8Encoding]::new($false))}
if($SubtitleMenus){[IO.File]::WriteAllText($subtitle,"1`n00:00:00,100 --> 00:00:00,900`nFirst subtitle`n`n2`n00:00:01,000 --> 00:00:01,900`nSecond subtitle`n`n3`n00:00:02,000 --> 00:00:02,900`nThird subtitle`n",[Text.UTF8Encoding]::new($false))}
if($LayoutNavigation){
    $body=[Text.StringBuilder]::new()
    for($i=0;$i -lt 120;$i++){
        $start=[TimeSpan]::FromMilliseconds($i*20).ToString('hh\:mm\:ss\,fff')
        $end=[TimeSpan]::FromMilliseconds(($i+1)*20).ToString('hh\:mm\:ss\,fff')
        [void]$body.Append("$($i+1)`n$start --> $end`nNavigation subtitle $('{0:000}' -f ($i+1))`n`n")
    }
    [IO.File]::WriteAllText($subtitle,$body.ToString(),[Text.UTF8Encoding]::new($false))
}
$fixture=if($ComposeOutput){'testsrc2=size=1280x720:rate=30:duration=12'}else{'testsrc2=size=320x180:rate=10:duration=3'}
& ffmpeg -v error -f lavfi -i $fixture -c:v libx264 -pix_fmt yuv420p $video
if($LASTEXITCODE -ne 0){throw 'Fixture creation failed'}
$env:ONESUB_DATA_ROOT = Join-Path $workPath 'data'
New-Item -ItemType Directory $env:ONESUB_DATA_ROOT | Out-Null
# Controls are found by their Chinese names; pin the interface language.
# This scenario verifies normal teardown, not tray behavior (covered by
# workbench_polish.ps1). An explicit preference keeps WM_CLOSE meaningful.
$prefs=@{version=1;uiLanguage='zh-Hans';closeToTray=$false;appearance=$Appearance}
if ($TranslationEndpoint) {
    # One translate request only: the fixture does not answer segmentation.
    $prefs+=@{providerUrl=$TranslationEndpoint;providerModel='local-protocol-test';targetLanguage='zh-CN';optimizeTimeline=$false}
}
if ($ProviderCatalogEndpoint) {
    $prefs+=@{providerUrl=$ProviderCatalogEndpoint;providerModel='catalog-alpha';language='en';recognitionFormat='vtt'}
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
    if($p.HasExited){return $null}
    $handle=$p.MainWindowHandle
    if($handle -eq [IntPtr]::Zero){return $null}
    $root=[System.Windows.Automation.AutomationElement]::FromHandle($handle)
    if(-not $root){return $null}
    $condition=[System.Windows.Automation.PropertyCondition]::new([System.Windows.Automation.AutomationElement]::NameProperty,$name)
    return $root.FindFirst([System.Windows.Automation.TreeScope]::Descendants,$condition)
}
function Click-UI($name) {
    for($i=0;$i -lt 100;$i++) {
        try {
            $p.Refresh()
            $root=[System.Windows.Automation.AutomationElement]::FromHandle($p.MainWindowHandle)
            $condition=[System.Windows.Automation.PropertyCondition]::new([System.Windows.Automation.AutomationElement]::NameProperty,$name)
            foreach($item in $root.FindAll([System.Windows.Automation.TreeScope]::Descendants,$condition)) {
                if(-not $item.Current.IsEnabled){continue}
                $pattern=$null
                if($item.TryGetCurrentPattern([System.Windows.Automation.InvokePattern]::Pattern,[ref]$pattern)){$pattern.Invoke();Start-Sleep -Milliseconds 200;return}
                if($item.TryGetCurrentPattern([System.Windows.Automation.ExpandCollapsePattern]::Pattern,[ref]$pattern)){$pattern.Expand();Start-Sleep -Milliseconds 250;return}
            }
        } catch [System.Windows.Automation.ElementNotAvailableException] {
            # A native redraw can invalidate a UIA node; reacquire by name.
        }
        Start-Sleep -Milliseconds 100
    }
    throw "Control unavailable: $name"
}
function Input-UI($name) {
    $root=[System.Windows.Automation.AutomationElement]::FromHandle($p.MainWindowHandle)
    $condition=[System.Windows.Automation.PropertyCondition]::new([System.Windows.Automation.AutomationElement]::NameProperty,$name)
    foreach($item in $root.FindAll([System.Windows.Automation.TreeScope]::Descendants,$condition)) {
        $pattern=$null
        if($item.TryGetCurrentPattern([System.Windows.Automation.ValuePattern]::Pattern,[ref]$pattern)){return $item}
    }
    throw "Input unavailable: $name"
}
function Read-Preferences { return ([IO.File]::ReadAllText((Join-Path $env:ONESUB_DATA_ROOT 'settings.json'))|ConvertFrom-Json) }
function Scroll-PanelEnd($name) {
    $panel=Find-UI $name
    foreach($node in $panel.FindAll([System.Windows.Automation.TreeScope]::Descendants,[System.Windows.Automation.Condition]::TrueCondition)) {
        $scroll=$null
        if($node.TryGetCurrentPattern([System.Windows.Automation.ScrollPattern]::Pattern,[ref]$scroll) -and $scroll.Current.VerticallyScrollable){$scroll.SetScrollPercent(-1,100);Start-Sleep -Milliseconds 350;return}
    }
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
function Select-Value($label) {
    $item=Find-UI $label
    if(-not $item){throw "Selection unavailable: $label"}
    $pattern=$null
    if($item.TryGetCurrentPattern([System.Windows.Automation.SelectionItemPattern]::Pattern,[ref]$pattern)){$pattern.Select()}
    else{$item.GetCurrentPattern([System.Windows.Automation.InvokePattern]::Pattern).Invoke()}
    Start-Sleep -Milliseconds 200
}
function Open-SubtitleMenu($label) {
    $item=Find-UI $label
    if(-not $item){throw "Subtitle missing: $label"}
    $r=$item.Current.BoundingRectangle
    if(-not [SmokeNative]::RightClick($p.MainWindowHandle,$p.Id,[int]($r.X+$r.Width/2),[int]($r.Y+$r.Height/2))){throw 'Test surface did not receive the right-click'}
}
function Wait-SubtitleMenu {
    # Read the owner-drawn popup's Win32 labels, which UIA does not expose.
    # This checks real menu presentation; command activation is covered by
    # the native component tests and still needs interactive desktop input.
    for($i=0;$i -lt 100;$i++) {
        $items=[SmokeNative]::MenuItems($p.Id)
        if($items){return $items}
        Start-Sleep -Milliseconds 100
    }
    throw 'Native subtitle menu did not appear'
}
function Wait-VisibleSubtitle($text) {
    # Search only the virtual table, not the editor's copy of a selected cue.
    for($i=0;$i -lt 100;$i++) {
        $list=Find-UI '字幕列表'
        if($list) {
            $row=$list.FindFirst([System.Windows.Automation.TreeScope]::Descendants,[System.Windows.Automation.PropertyCondition]::new([System.Windows.Automation.AutomationElement]::NameProperty,$text))
            if($row) {
                $r=$row.Current.BoundingRectangle;$bounds=$list.Current.BoundingRectangle
                if(-not $r.IsEmpty -and $r.Height -gt 0 -and $r.Top -ge $bounds.Top -and $r.Bottom -le $bounds.Bottom){return}
            }
        }
        Start-Sleep -Milliseconds 100
    }
    throw "Subtitle not visible in the table: $text"
}
function Capture-Navigation($file) {
    # UIA invocation leaves keyboard focus/tooltip adornments on the button.
    # A pointer click on the inert style header clears them without selecting
    # another subtitle or changing the viewport being captured.
    $header=(Find-UI '字幕样式面板').Current.BoundingRectangle
    if(-not [SmokeNative]::PointerClick($p.MainWindowHandle,$p.Id,[int]($header.X+$header.Width/2),[int]($header.Y+12),$false)){throw 'Could not clear navigation focus adornments'}
    Start-Sleep -Milliseconds 300
    & $ScreenshotTool $p.Id (Join-Path $workPath $file)
}
function Choose-Path($path){
    for($i=0;$i -lt 100;$i++) { if([SmokeNative]::Choose($p.Id,$path)){return};Start-Sleep -Milliseconds 100 }
    throw 'File dialog did not appear'
}
function Press-PlaybackSpace {
    $p.Refresh()
    [void][SmokeNative]::SetForegroundWindow($p.MainWindowHandle)
    $list=Find-UI '字幕列表'
    if(-not $list){throw 'Subtitle list unavailable for keyboard playback test'}
    $list.SetFocus()
    if([SmokeNative]::GetForegroundWindow() -ne $p.MainWindowHandle){throw 'Test window is not foreground; no keyboard input sent'}
    [System.Windows.Forms.SendKeys]::SendWait(' ')
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
    if($WorkflowLayout){[void][SmokeNative]::ShowWindow($p.MainWindowHandle,3);Start-Sleep -Milliseconds 800}
    # UIA is an explicit mode for a locked/remote desktop, not a passing
    # keyboard test. Default runs still require real foreground Space input.
    if($UIAPlayback){Click-UI '播放'}else{Press-PlaybackSpace}
    Start-Sleep -Seconds 2
    if(-not (Find-UI '暂停')){
        & $ScreenshotTool $p.Id (Join-Path $workPath 'failure.png')
        throw 'Playback did not start'
    }
    if($UIAPlayback){Click-UI '暂停'}else{Press-PlaybackSpace}
    for($i=0;$i -lt 30;$i++){if(Find-UI '播放'){break};Start-Sleep -Milliseconds 100}
    if(-not (Find-UI '播放')){throw 'Playback did not pause'}
    # Muting updates the native slider as well as the audio, and restores it.
    $volume=(Find-UI '音量').GetCurrentPattern([System.Windows.Automation.RangeValuePattern]::Pattern).Current.Value
    Click-UI '静音';Start-Sleep -Milliseconds 300
    if((Find-UI '音量').GetCurrentPattern([System.Windows.Automation.RangeValuePattern]::Pattern).Current.Value -ne 0){throw 'Mute did not zero the volume slider'}
    Click-UI '取消静音';Start-Sleep -Milliseconds 300
    if((Find-UI '音量').GetCurrentPattern([System.Windows.Automation.RangeValuePattern]::Pattern).Current.Value -ne $volume){throw 'Unmute did not restore the previous volume'}
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
            if($tab -eq '翻译服务' -and ((Find-UI '并发批数') -or (Find-UI '每批字幕数'))){throw 'Task scheduling is still in service settings'}
            & $ScreenshotTool $p.Id (Join-Path $workPath ("settings-"+$tab+'.png'))
        }
    }
    if($ProviderCatalogEndpoint) {
        for($i=0;$i -lt 100;$i++){if(Find-UI '保留模型 catalog-beta'){break};Start-Sleep -Milliseconds 100}
        $before=(Find-UI '设置内容').Current.BoundingRectangle
        $check=Find-UI '保留模型 catalog-beta'
        if(-not $check){throw 'Fetched models are not visible'}
        $check.GetCurrentPattern([System.Windows.Automation.TogglePattern]::Pattern).Toggle()
        Start-Sleep -Milliseconds 350
        $saved=Read-Preferences
        if($saved.providerModel -ne 'catalog-alpha' -or $saved.providers[0].models.Count -ne 2){throw 'Keeping a model changed selection or was not saved'}
        Click-UI '使用模型 catalog-beta'
        if((Read-Preferences).providerModel -ne 'catalog-beta'){throw 'Model use action did not persist'}
        Click-UI '切换模型'
        $names=Wait-SubtitleMenu
        if($names -notcontains 'catalog-alpha' -or $names -notcontains 'catalog-beta'){throw 'Model dropdown does not contain retained models'}
        [void][SmokeNative]::PostMessage($p.MainWindowHandle,31,[IntPtr]::Zero,[IntPtr]::Zero)
        Start-Sleep -Milliseconds 400
        (Input-UI '搜索模型').GetCurrentPattern([System.Windows.Automation.ValuePattern]::Pattern).SetValue('alpha')
        Start-Sleep -Milliseconds 250
        if(Find-UI '保留模型 catalog-beta'){throw 'Search did not filter the model table'}
        (Input-UI '搜索模型').GetCurrentPattern([System.Windows.Automation.ValuePattern]::Pattern).SetValue('')
        Click-UI '获取模型';Start-Sleep -Milliseconds 500
        if((Read-Preferences).providerModel -ne 'catalog-beta' -or (Read-Preferences).providers[0].models.Count -ne 2){throw 'Refresh lost retained models'}
        if((Find-UI '设置内容').Current.BoundingRectangle -ne $before){throw 'Model state resized settings'}
        $header=(Find-UI '供应商模型目录').Current.BoundingRectangle
        [void][SmokeNative]::PointerClick($p.MainWindowHandle,$p.Id,[int]($header.X+24),[int]($header.Y+12),$false)
        Start-Sleep -Milliseconds 350
        & $ScreenshotTool $p.Id (Join-Path $workPath 'model-library.png')
    }
    Click-UI '关闭'
    if($ProviderCatalogEndpoint) {
        Select-Tab '识别与翻译' 'AI 翻译'
        Click-UI '切换模型';$names=Wait-SubtitleMenu
        if($names -notcontains 'catalog-beta'){throw 'Workbench dropdown lost selected model'}
        [void][SmokeNative]::PostMessage($p.MainWindowHandle,31,[IntPtr]::Zero,[IntPtr]::Zero)
        Start-Sleep -Milliseconds 400
        $toggle=Find-UI '按意群重建时间轴'
        $original=(Read-Preferences).optimizeTimeline
        $toggle.GetCurrentPattern([System.Windows.Automation.TogglePattern]::Pattern).Toggle()
        Start-Sleep -Milliseconds 250
        if((Read-Preferences).optimizeTimeline -eq $original){throw 'Timeline feature toggle did not persist'}
        Capture-Navigation 'translation-workflow.png'
        Select-Tab '识别与翻译' '语音识别'
        Select-Value '文件队列'
        $draft=(Input-UI '字幕文字').GetCurrentPattern([System.Windows.Automation.ValuePattern]::Pattern).Current.Value
        $second=Join-Path $workPath 'queue-second.wav'
        & ffmpeg -v error -f lavfi -i 'anullsrc=r=16000:cl=mono' -t 0.2 $second
        if($LASTEXITCODE -ne 0){throw 'Queue fixture generation failed'}
        $third=Join-Path $workPath 'queue-third.wav';Copy-Item $second $third
        Click-UI '添加音视频';Choose-Path ('"'+$video+'" "'+$second+'"')
        for($i=0;$i -lt 100;$i++){if((Read-Preferences).batchSources.Count -eq 2){break};Start-Sleep -Milliseconds 100}
        if((Read-Preferences).batchSources.Count -ne 2){throw 'Multiple file selection did not enter the recognition queue'}
        Click-UI '添加音视频';Choose-Path $third
        for($i=0;$i -lt 100;$i++){if((Read-Preferences).batchSources.Count -eq 3){break};Start-Sleep -Milliseconds 100}
        $saved=Read-Preferences
        if($saved.batchSources.Count -ne 3 -or $saved.language -ne 'en' -or $saved.recognitionFormat -ne 'vtt'){throw 'Single file addition lost shared queue settings'}
        if((Input-UI '字幕文字').GetCurrentPattern([System.Windows.Automation.ValuePattern]::Pattern).Current.Value -ne $draft){throw 'Queue addition replaced workbench subtitles'}
        Capture-Navigation 'recognition-queue.png'
        Write-Output 'PASS: native provider catalog fetch/search/retention/refresh, dropdown presentation on both surfaces, model use, timeline toggle, single/multiple file picker queue and unchanged workbench'
    }
    if($WorkflowLayout) {
        foreach($tab in @('语音识别','AI 翻译')) {
            Select-Tab '识别与翻译' $tab
            $action=if($tab -eq 'AI 翻译'){'开始翻译字幕'}else{'开始识别字幕'}
            $before=(Find-UI $action).Current.BoundingRectangle
            Scroll-PanelEnd '识别与翻译'
            if((Find-UI $action).Current.BoundingRectangle -ne $before){throw 'Primary action scrolled with parameters'}
            if($tab -eq 'AI 翻译') {
                if(Find-UI '并发批数'){throw 'Advanced scheduling is not initially separate'}
                Click-UI '请求与批次设置';Scroll-PanelEnd '识别与翻译'
                $a=(Find-UI '并发批数').Current.BoundingRectangle
                $b=(Find-UI '每批字幕数').Current.BoundingRectangle
                if([Math]::Abs($a.Y-$b.Y) -gt 1 -or $a.Right -ge $b.Left){throw 'Scheduling peer fields are not aligned'}
            }
            Capture-Navigation ("workflow-"+$tab+'.png')
        }
        Write-Output 'PASS: primary actions stay fixed while inspectors scroll; advanced scheduling is separate and aligned'
    }
    if($LayoutNavigation) {
        for($i=0;$i -lt 100;$i++){
            if((Find-UI '一键识别并翻译') -and (Find-UI '字幕样式面板') -and (Find-UI '字幕编辑面板') -and (Find-UI '返回顶部')){break}
            Start-Sleep -Milliseconds 100
        }
        $style=(Find-UI '字幕样式面板').Current.BoundingRectangle
        $editor=(Find-UI '字幕编辑面板').Current.BoundingRectangle
        $workflow=(Find-UI '识别与翻译').Current.BoundingRectangle
        if(-not ($style.X -lt $editor.X -and $editor.X -lt $workflow.X) -or [Math]::Abs($editor.Width/$style.Width-4/3) -gt .02 -or [Math]::Abs($workflow.Width/$style.Width-1) -gt .02){throw 'Lower panes are not style/editor/workflow at 1.5:2:1.5'}
        $bar=(Find-UI '主操作').Current.BoundingRectangle
        $primary=(Find-UI '一键识别并翻译').Current.BoundingRectangle
        if([Math]::Abs($primary.X+$primary.Width/2-$bar.X-$bar.Width/2) -gt 2){throw 'One-click action is not centered in the toolbar'}
        $draft=(Find-UI '字幕文字').GetCurrentPattern([System.Windows.Automation.ValuePattern]::Pattern).Current.Value
        $top=(Find-UI '返回顶部').Current.BoundingRectangle
        $bottom=(Find-UI '返回底部').Current.BoundingRectangle
        Click-UI '返回底部';Wait-VisibleSubtitle 'Navigation subtitle 120'
        if((Find-UI '字幕文字').GetCurrentPattern([System.Windows.Automation.ValuePattern]::Pattern).Current.Value -ne $draft){throw 'Bottom navigation changed the selected subtitle'}
        Capture-Navigation 'list-bottom.png'
        Click-UI '返回顶部';Wait-VisibleSubtitle 'Navigation subtitle 001'
        if((Find-UI '字幕文字').GetCurrentPattern([System.Windows.Automation.ValuePattern]::Pattern).Current.Value -ne $draft){throw 'Top navigation changed the selected subtitle'}
        if((Find-UI '返回顶部').Current.BoundingRectangle -ne $top -or (Find-UI '返回底部').Current.BoundingRectangle -ne $bottom){throw 'Floating buttons moved with the rows'}
        Capture-Navigation 'layout-navigation.png'
        Write-Output 'PASS: centered one-click toolbar action; style/editor/workflow 1.5:2:1.5; floating top/bottom navigation; selected subtitle unchanged'
    }
    if($SubtitleMenus) {
        # Invoke closes the modal asynchronously. Wait for the workbench's
        # UIA nodes before measuring, as Select-Tab does after dialog changes.
        for($i=0;$i -lt 100;$i++){
            if((Find-UI '插入／删除字幕') -and (Find-UI '字幕列表') -and (Find-UI '字幕编辑面板')){break}
            Start-Sleep -Milliseconds 100
        }
        $menu=(Find-UI '插入／删除字幕').Current.BoundingRectangle
        $list=(Find-UI '字幕列表').Current.BoundingRectangle
        $editor=Find-UI '字幕编辑面板'
        $name=[System.Windows.Automation.PropertyCondition]::new([System.Windows.Automation.AutomationElement]::NameProperty,'插入／删除字幕')
        if($menu.Left -lt $list.Left -or $menu.Right -gt $list.Right -or $menu.Bottom -gt $list.Top -or $editor.FindFirst([System.Windows.Automation.TreeScope]::Descendants,$name)){throw 'Structure menu is not confined to the subtitle list'}
        & $ScreenshotTool $p.Id (Join-Path $workPath 'subtitle-list.png')
        Open-SubtitleMenu 'Second subtitle'
        $items=Wait-SubtitleMenu
        if(($items -join '|') -ne '在前面插入字幕|在后面插入字幕|-|删除字幕'){throw "Incorrect native menu items: $items"}
        # UIA publishes the next view after the synchronous native popup
        # returns; selection and mutations are asserted by component tests.
    }
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
        if($InterruptTranslation){
            for($i=0;$i -lt 150;$i++){if(Find-UI '继续处理'){break};Start-Sleep -Milliseconds 100}
            if(-not (Find-UI '继续处理') -or -not (Find-UI '你好，世界') -or -not (Find-UI '尚未完成')){throw 'Interruption hid completed or draft text'}
            if(Test-Path $translated){throw 'Interrupted translation was published'}
            & $ScreenshotTool $p.Id (Join-Path $workPath 'translation-interrupted.png')
            # Only the process this script launched, with isolated data and IPC,
            # is stopped to exercise recovery after an unexpected application exit.
            if([IO.Path]::GetFullPath($p.Path) -ne [IO.Path]::GetFullPath($Exe)){throw 'Test process identity changed'}
            Stop-Process -Id $p.Id -Force
            $p.WaitForExit()
            $restart=$launch.Clone()
            $restart.Remove('ArgumentList')
            $restart.RedirectStandardError=Join-Path $workPath 'restart-stderr.log'
            $p=Start-Process @restart
            for($i=0;$i -lt 150;$i++){if(Find-UI '继续处理'){break};Start-Sleep -Milliseconds 100}
            if(-not (Find-UI '继续处理') -or -not (Find-UI '你好，世界') -or -not (Find-UI '尚未完成')){throw 'Restart did not restore the translation'}
            & $ScreenshotTool $p.Id (Join-Path $workPath 'translation-restored.png')
            Click-UI '继续处理'
        }
        for($i=0;$i -lt 200;$i++){if(Test-Path $translated){break};Start-Sleep -Milliseconds 100}
        if(-not (Test-Path $translated)){throw 'Translation was not automatically saved'}
        if(-not [IO.File]::ReadAllText($translated).Contains('你好，世界')){throw 'Saved translation has no translated text'}
        if([IO.File]::ReadAllText($translated).Contains($sourceText)){throw 'Default translation save included source text'}
        if(-not [IO.File]::ReadAllText($subtitle).Contains($sourceText)){throw 'Source subtitle was overwritten'}
        if(-not (Find-UI '你好，世界')){throw 'Finished translation did not update the subtitle table'}
        Start-Sleep -Seconds 2
        & $ScreenshotTool $p.Id (Join-Path $workPath 'translation.png')
        Click-UI '撤销'
        Select-Tab '字幕语言' '原始字幕'
        for($i=0;$i -lt 100;$i++){if(Find-UI $sourceText){break};Start-Sleep -Milliseconds 100}
        if(-not (Find-UI $sourceText)){throw 'Translation undo did not restore the original'}
    }
    if($BilingualExport) {
        foreach($choice in @(@{id='upper';label='仅上行字幕';kept='Upper subtitle';removed='下行字幕'},@{id='lower';label='仅下行字幕';kept='下行字幕';removed='Upper subtitle'})) {
            Click-UI '导出字幕'
            for($i=0;$i -lt 100;$i++){if(Find-UI '保留字幕行'){break};Start-Sleep -Milliseconds 100}
            Click-UI '保留字幕行'
            Select-Value $choice.label
            & $ScreenshotTool $p.Id (Join-Path $workPath ("export-"+$choice.id+'.png'))
            Click-UI '导出'
            $output=Join-Path $workPath ("export-"+$choice.id+'.srt')
            Choose-Path $output
            for($i=0;$i -lt 100;$i++){if(Test-Path -LiteralPath $output){break};Start-Sleep -Milliseconds 100}
            if(-not (Test-Path -LiteralPath $output)){throw 'Single-language export did not create a file'}
            $saved=[IO.File]::ReadAllText($output)
            if(-not $saved.Contains($choice.kept) -or $saved.Contains($choice.removed) -or -not $saved.Contains('00:00:00,100 --> 00:00:02,900')){throw 'Single-language export changed content or time'}
        }
        if([IO.File]::ReadAllText($subtitle) -notmatch [regex]::Escape($sourceText)){throw 'Bilingual source was changed by export'}
    }
    if($ComposeOutput) {
        Click-UI '合成视频';Click-UI '开始合成'
        $output=Join-Path $workPath 'test-OneSub.mp4'
        for($i=0;$i -lt 600;$i++){if(Test-Path -LiteralPath $output){break};Start-Sleep -Milliseconds 100}
        if(-not (Test-Path -LiteralPath $output)){throw 'Automatic export did not write beside source'}
        if(Test-Path -LiteralPath ($output+'.onesub.json')){throw 'Video directory contains a report sidecar'}
        Start-Sleep -Milliseconds 300
        & $ScreenshotTool $p.Id (Join-Path $workPath 'compose-result.png')
        Write-Output 'PASS: automatic composition beside source; no report sidecar'
    }
    $inputMode=if($UIAPlayback){'UIA (keyboard not exercised)'}else{'Space'}
    Write-Output "PASS: import (argument=$OpenArgument), same-name subtitle, playback/pause=$inputMode, settings, translation=$([bool]$TranslationEndpoint), upper/lower export=$BilingualExport, subtitle menu presentation=$SubtitleMenus"
} catch {
    Write-Output ("FAIL: native smoke: " + $_.Exception.Message)
    throw
} finally {
    # A failed menu assertion must close this test-owned popup before WM_CLOSE;
    # otherwise its native menu loop can keep the test window alive.
    $p.Refresh()
    if(-not $p.HasExited){[void][SmokeNative]::PostMessage($p.MainWindowHandle,31,[IntPtr]::Zero,[IntPtr]::Zero);Start-Sleep -Milliseconds 200}
    for($i=0;$i -lt 200;$i++){if(-not (Find-UI '取消任务')){break};Start-Sleep -Milliseconds 100}
    $p.Refresh()
    if(-not $p.HasExited){
        $null=$p.CloseMainWindow()
        for($i=0;$i -lt 100;$i++){
            if($p.HasExited){break}
            # Undo leaves the generated translation file out of sync with
            # this disposable project. Dismiss only this test instance's guard.
            if(Find-UI '不保存'){Click-UI '不保存'}
            elseif($i%10 -eq 0){$null=$p.CloseMainWindow()}
            Start-Sleep -Milliseconds 100
            $p.Refresh()
        }
        if(-not $p.WaitForExit(10000)){
            # This PID was created above with isolated state; never leave a
            # failed file-dialog scenario blocking subsequent desktop tests.
            $p.Kill();$p.WaitForExit()
            throw "Test window did not close normally: PID=$($p.Id)"
        }
    }
}
