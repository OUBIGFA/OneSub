param([string]$Exe, [string]$Work, [string]$ScreenshotTool, [switch]$OpenArgument, [string]$TranslationEndpoint, [switch]$Packaged, [switch]$InterruptTranslation, [switch]$UIAPlayback, [switch]$BilingualExport, [switch]$SubtitleMenus, [switch]$LayoutNavigation, [switch]$VideoTools, [switch]$WorkflowLayout, [switch]$ComposeOutput, [switch]$SubtitleClicks, [string]$ProviderCatalogEndpoint, [ValidateSet('dark','light')][string]$Appearance = 'dark', [switch]$PresetReset, [switch]$PrivateDesktop, [switch]$ChoiceControls)
$ErrorActionPreference = 'Stop'
if(-not $PrivateDesktop){throw 'Run through private_native_smoke.ps1 to preserve the user desktop'}
if(-not $UIAPlayback){throw 'Private desktop playback must use UIA, never global keyboard input'}
if($SubtitleMenus -and ($TranslationEndpoint -or $InterruptTranslation -or $BilingualExport)){throw 'Run subtitle menu presentation as a separate scenario'}
if($LayoutNavigation -and ($TranslationEndpoint -or $InterruptTranslation -or $BilingualExport -or $SubtitleMenus)){throw 'Run layout/navigation as a separate scenario'}
Add-Type -AssemblyName UIAutomationClient
Add-Type -AssemblyName UIAutomationTypes
Add-Type -AssemblyName System.Windows.Forms
Add-Type @'
using System;
using System.Runtime.InteropServices;
using System.Text;
public static class SmokeNative {
  [DllImport("user32.dll")] public static extern bool SetProcessDPIAware();
  [DllImport("kernel32.dll")] public static extern uint GetCurrentThreadId();
  [DllImport("user32.dll")] public static extern IntPtr GetThreadDesktop(uint id);
  [DllImport("user32.dll",CharSet=CharSet.Unicode)] static extern bool GetUserObjectInformation(IntPtr obj,int index,StringBuilder name,int size,out int needed);
  public static bool IsPrivateDesktop() {
    var name=new StringBuilder(256);int needed;
    return GetUserObjectInformation(GetThreadDesktop(GetCurrentThreadId()),2,name,512,out needed)&&name.ToString().StartsWith("OneSub-test-");
  }
  [DllImport("user32.dll")] public static extern bool ShowWindow(IntPtr h, int command);
  [DllImport("user32.dll")] public static extern IntPtr GetForegroundWindow();
  [DllImport("user32.dll")] public static extern uint GetDoubleClickTime();
  public delegate bool Visitor(IntPtr h, IntPtr p);
  [DllImport("user32.dll")] public static extern bool EnumDesktopWindows(IntPtr desktop,Visitor fn,IntPtr p);
  public static IntPtr TestWindow(int pid) {
    IntPtr result=IntPtr.Zero;
    EnumDesktopWindows(GetThreadDesktop(GetCurrentThreadId()),(h,p)=>{
      uint id;GetWindowThreadProcessId(h,out id);
      var name=new StringBuilder(128);GetClassName(h,name,128);
      if(id==(uint)pid&&name.ToString()=="MyGoWindow"){result=h;return false;}
      return true;
    },IntPtr.Zero);
    return result;
  }
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
  public static string MenuActivationError="";
  static bool PressMenuKey(IntPtr owner, byte key) {
    if(!PostMessage(owner,256,new IntPtr(key),IntPtr.Zero)||!PostMessage(owner,257,new IntPtr(key),IntPtr.Zero))return false;
    System.Threading.Thread.Sleep(75);return true;
  }
  public static bool ChooseMenuItem(IntPtr owner, int pid, string label) {
    IntPtr popup=IntPtr.Zero;
    EnumWindows((candidate,p)=>{
      uint id;GetWindowThreadProcessId(candidate,out id);
      var name=new System.Text.StringBuilder(128);GetClassName(candidate,name,128);
      if(id!=pid||name.ToString()!="#32768")return true;
      popup=candidate;return false;
    },IntPtr.Zero);
    if(popup==IntPtr.Zero){MenuActivationError="No test-owned popup";return false;}
    IntPtr menu=SendValue(popup,0x1e1,IntPtr.Zero,IntPtr.Zero);
    IntPtr buffer=Marshal.AllocHGlobal(1024);
    uint target=uint.MaxValue;
    int count=GetMenuItemCount(menu);
    try {
      for(uint i=0;i<count;i++) {
        var info=new MenuItemInfo{Size=(uint)Marshal.SizeOf(typeof(MenuItemInfo)),Mask=0x141,Text=buffer,Length=512};
        if(!GetMenuItemInfo(menu,i,true,ref info)){MenuActivationError="Cannot read menu item";return false;}
        if((info.Type&0x800)!=0||(info.State&3)!=0)continue;
        if(Marshal.PtrToStringUni(buffer).Split('\t')[0]==label){target=i;break;}
      }
    } finally {Marshal.FreeHGlobal(buffer);}
    if(target==uint.MaxValue){MenuActivationError="Requested item is absent or disabled";return false;}
    // Context menus can start without a highlighted row and ignore Home.
    // Navigate by observed native state instead of assuming a key count.
    // Input stays outside enumeration and only reaches the verified owner;
    // never synthesize an application WM_COMMAND.
    System.Threading.Thread.Sleep(100);
    for(int step=0;step<=count;step++) {
      var state=new MenuItemInfo{Size=(uint)Marshal.SizeOf(typeof(MenuItemInfo)),Mask=1};
      if(!GetMenuItemInfo(menu,target,true,ref state)){MenuActivationError="Cannot read highlighted item";return false;}
      if((state.State&0x80)!=0)return PressMenuKey(popup,13); // Enter
      if(step<count&&!PressMenuKey(popup,40))return false; // Down
    }
    MenuActivationError="Target was not highlighted after native navigation";
    return false;
  }
  public static bool RightClick(IntPtr window, int pid, int x, int y) {
    return PointerClick(window,pid,x,y,true);
  }
  public static bool PointerClick(IntPtr window, int pid, int x, int y, bool right, int hold=0) {
    // Send native pointer messages only to this test instance's MyGo surface.
    IntPtr surface=TestSurface(window,pid);
    if(surface==IntPtr.Zero)return false;
    var point=new Point{X=x,Y=y};
    if(!ScreenToClient(surface,ref point))return false;
    var coords=new IntPtr((point.Y<<16)|(point.X&0xffff));
    if(!PostMessage(surface,512,IntPtr.Zero,coords)||!PostMessage(surface,right?516u:513u,new IntPtr(right?2:1),coords))return false;
    if(hold>0)System.Threading.Thread.Sleep(hold);
    return PostMessage(surface,right?517u:514u,IntPtr.Zero,coords);
  }
  public static bool Wheel(IntPtr window,int pid,int x,int y,int delta) {
    IntPtr surface=TestSurface(window,pid);
    if(surface==IntPtr.Zero)return false;
    // WM_MOUSEWHEEL takes screen coordinates and targets the native surface,
    // exactly as MyGo's Windows test hook does. No global input is sent.
    SendValue(surface,0x20a,new IntPtr(delta<<16),new IntPtr((y<<16)|(x&0xffff)));
    return true;
  }
  static IntPtr TestSurface(IntPtr window,int pid) {
    IntPtr surface=IntPtr.Zero;
    EnumChildWindows(window,(child,p)=>{
      uint id;GetWindowThreadProcessId(child,out id);
      var name=new System.Text.StringBuilder(128);GetClassName(child,name,128);
      if(id==pid&&name.ToString()=="MyGoSurface"){surface=child;return false;}
      return true;
    },IntPtr.Zero);
    return surface;
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
if(-not [SmokeNative]::IsPrivateDesktop()){throw 'The driver is not attached to the isolated test desktop'}
[void][SmokeNative]::SetProcessDPIAware()
$workPath = [IO.Path]::GetFullPath($Work)
if (Test-Path $workPath) { throw 'Use a fresh smoke directory' }
New-Item -ItemType Directory $workPath | Out-Null
$video = Join-Path $workPath 'test.mp4'
$subtitle = Join-Path $workPath 'test.srt'
$sourceText = if ($TranslationEndpoint) { 'Hello, world' } elseif ($BilingualExport) { "Upper subtitle`n下行字幕" } else { '测试字幕' }
[IO.File]::WriteAllText($subtitle,"1`n00:00:00,100 --> 00:00:02,900`n$sourceText`n",[Text.UTF8Encoding]::new($false))
if($InterruptTranslation){[IO.File]::WriteAllText($subtitle,"1`n00:00:00,100 --> 00:00:01,400`n$sourceText`n`n2`n00:00:01,500 --> 00:00:02,900`nGood morning`n",[Text.UTF8Encoding]::new($false))}
if($SubtitleMenus){[IO.File]::WriteAllText($subtitle,"1`n00:00:00,100 --> 00:00:00,900`nFirst subtitle.`n`n2`n00:00:01,000 --> 00:00:01,900`nSecond subtitle.`n`n3`n00:00:02,000 --> 00:00:02,500`n第一段字幕。`n`n4`n00:00:02,500 --> 00:00:02,900`n，第二段字幕。`n",[Text.UTF8Encoding]::new($false))}
if($LayoutNavigation){
    $body=[Text.StringBuilder]::new()
    for($i=0;$i -lt 120;$i++){
        $start=[TimeSpan]::FromMilliseconds($i*20).ToString('hh\:mm\:ss\,fff')
        $end=[TimeSpan]::FromMilliseconds(($i+1)*20).ToString('hh\:mm\:ss\,fff')
        [void]$body.Append("$($i+1)`n$start --> $end`nNavigation subtitle $('{0:000}' -f ($i+1))`n`n")
    }
    [IO.File]::WriteAllText($subtitle,$body.ToString(),[Text.UTF8Encoding]::new($false))
}
if($SubtitleClicks){
    $body=[Text.StringBuilder]::new()
    for($i=0;$i -lt 160;$i++){
        $start=[TimeSpan]::FromMilliseconds($i*1000).ToString('hh\:mm\:ss\,fff')
        $end=[TimeSpan]::FromMilliseconds($i*1000+900).ToString('hh\:mm\:ss\,fff')
        [void]$body.Append("$($i+1)`n$start --> $end`nClick subtitle $('{0:000}' -f ($i+1))`n`n")
    }
    [IO.File]::WriteAllText($subtitle,$body.ToString(),[Text.UTF8Encoding]::new($false))
}
$fixture=if($SubtitleClicks){'testsrc2=size=320x180:rate=2:duration=170'}elseif($ComposeOutput){'testsrc2=size=1280x720:rate=30:duration=12'}elseif($VideoTools -or $PresetReset -or $ChoiceControls){'testsrc2=size=320x180:rate=10:duration=12'}else{'testsrc2=size=320x180:rate=10:duration=3'}
if($VideoTools){
    & ffmpeg -v error -f lavfi -i $fixture -f lavfi -i 'sine=frequency=440:duration=12' -c:v libx264 -pix_fmt yuv420p -c:a aac $video
}else{
    & ffmpeg -v error -f lavfi -i $fixture -c:v libx264 -pix_fmt yuv420p $video
}
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
    # Persist two complete accounts and their catalogs before launch. Their
    # credential scopes are unique to this disposable local fixture.
    $providerId='native-catalog-test-'+[guid]::NewGuid().ToString('N')
    $prefs+=@{providerUrl=$ProviderCatalogEndpoint;providerModel='catalog-alpha';language='en';recognitionFormat='vtt';activeProvider=$providerId;providers=@(
        @{id=$providerId;name='本地目录 A';url=$ProviderCatalogEndpoint;api='chat-completions';model='catalog-alpha';models=@(@{id='catalog-alpha'});catalog=@(@{id='catalog-alpha'},@{id='catalog-cached'})},
        @{id=($providerId+'-other');name='本地目录 B';url=$ProviderCatalogEndpoint;api='chat-completions';model='catalog-other';models=@(@{id='catalog-other'});catalog=@(@{id='catalog-other'})}
    )}
}
if ($ChoiceControls) {
    $models=@(0..17 | ForEach-Object { @{id=('fixture-model-{0:d2}' -f $_)} })
    $prefs+=@{providerUrl='https://example.invalid/v1';providerModel='fixture-model-00';activeProvider='choice-fixture';providers=@(
        @{id='choice-fixture';name='Choices';url='https://example.invalid/v1';api='chat-completions';model='fixture-model-00';models=$models;catalog=$models}
    )}
}
[IO.File]::WriteAllText((Join-Path $env:ONESUB_DATA_ROOT 'settings.json'),($prefs|ConvertTo-Json -Depth 8),[Text.UTF8Encoding]::new($false))
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
# The window must render on this verified non-input desktop; SW_HIDE prevents
# MyGo from creating its normal workbench. No window is shown on the user desktop.
$launch=@{FilePath=[IO.Path]::GetFullPath($Exe);PassThru=$true;WindowStyle='Normal';RedirectStandardError=(Join-Path $workPath 'stderr.log')}
if($OpenArgument){$launch.ArgumentList=@('"'+$video+'"')}
$p=Start-Process @launch
function Find-UI($name) {
    $p.Refresh()
    if($p.HasExited){return $null}
    $handle=([SmokeNative]::TestWindow($p.Id))
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
            $root=[System.Windows.Automation.AutomationElement]::FromHandle(([SmokeNative]::TestWindow($p.Id)))
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
    $root=[System.Windows.Automation.AutomationElement]::FromHandle(([SmokeNative]::TestWindow($p.Id)))
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
    if(-not [SmokeNative]::RightClick(([SmokeNative]::TestWindow($p.Id)),$p.Id,[int]($r.X+$r.Width/2),[int]($r.Y+$r.Height/2))){throw 'Test surface did not receive the right-click'}
}
function Wait-SubtitleMenu {
    # Lobe menus live in the native surface and expose UIA menu items. Read
    # only this test-owned window; there is no separate Win32 popup anymore.
    for($i=0;$i -lt 100;$i++) {
        $root=[System.Windows.Automation.AutomationElement]::FromHandle(([SmokeNative]::TestWindow($p.Id)))
        $condition=[System.Windows.Automation.PropertyCondition]::new([System.Windows.Automation.AutomationElement]::ControlTypeProperty,[System.Windows.Automation.ControlType]::MenuItem)
        $items=@($root.FindAll([System.Windows.Automation.TreeScope]::Descendants,$condition) | ForEach-Object { $_.Current.Name })
        if($items.Count){return $items}
        Start-Sleep -Milliseconds 100
    }
    throw 'Native subtitle menu did not appear'
}
function Wait-ModelChoices {
    for($i=0;$i -lt 100;$i++) {
        $root=[System.Windows.Automation.AutomationElement]::FromHandle(([SmokeNative]::TestWindow($p.Id)))
        $condition=[System.Windows.Automation.PropertyCondition]::new([System.Windows.Automation.AutomationElement]::ControlTypeProperty,[System.Windows.Automation.ControlType]::ListItem)
        $items=@($root.FindAll([System.Windows.Automation.TreeScope]::Descendants,$condition) | ForEach-Object { $_.Current.Name })
        if($items.Count){return $items}
        Start-Sleep -Milliseconds 100
    }
    throw 'Native model options did not appear'
}
function Select-Choice($label) {
    # The model catalog can show the same text behind the popup. Only the
    # actual popup item is a selection target.
    $root=[System.Windows.Automation.AutomationElement]::FromHandle(([SmokeNative]::TestWindow($p.Id)))
    $condition=[System.Windows.Automation.PropertyCondition]::new([System.Windows.Automation.AutomationElement]::ControlTypeProperty,[System.Windows.Automation.ControlType]::ListItem)
    $item=@($root.FindAll([System.Windows.Automation.TreeScope]::Descendants,$condition) | Where-Object {$_.Current.Name -eq $label})[0]
    if(-not $item -or $item.Current.IsOffscreen){throw "Visible choice unavailable: $label"}
    Pointer-ClickRow $item.Current.BoundingRectangle
    Start-Sleep -Milliseconds 200
}
function Choose-SubtitleMenu($label) {
    $items=Wait-SubtitleMenu
    if($items -notcontains $label){throw "Native menu item unavailable: $label"}
    $root=[System.Windows.Automation.AutomationElement]::FromHandle(([SmokeNative]::TestWindow($p.Id)))
    $condition=[System.Windows.Automation.AndCondition]::new(
        [System.Windows.Automation.PropertyCondition]::new([System.Windows.Automation.AutomationElement]::ControlTypeProperty,[System.Windows.Automation.ControlType]::MenuItem),
        [System.Windows.Automation.PropertyCondition]::new([System.Windows.Automation.AutomationElement]::NameProperty,$label))
    $item=$root.FindFirst([System.Windows.Automation.TreeScope]::Descendants,$condition)
    if(-not $item -or -not $item.Current.IsEnabled){throw "Native menu item cannot be activated: $label"}
    $item.GetCurrentPattern([System.Windows.Automation.InvokePattern]::Pattern).Invoke()
    Start-Sleep -Milliseconds 350
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
    if(-not [SmokeNative]::PointerClick(([SmokeNative]::TestWindow($p.Id)),$p.Id,[int]($header.X+$header.Width/2),[int]($header.Y+12),$false)){throw 'Could not clear navigation focus adornments'}
    Start-Sleep -Milliseconds 300
    & $ScreenshotTool $p.Id (Join-Path $workPath $file)
}
function Choose-Path($path){
    for($i=0;$i -lt 100;$i++) { if([SmokeNative]::Choose($p.Id,$path)){return};Start-Sleep -Milliseconds 100 }
    throw 'File dialog did not appear'
}
function Read-PlaybackPosition {
    return (Find-UI '播放位置').GetCurrentPattern([System.Windows.Automation.RangeValuePattern]::Pattern).Current.Value
}
function Set-PlaybackPosition($seconds) {
    # This pinned MyGo UIA RangeValue.SetValue only steps once; use the
    # actual native slider track for an absolute seek and hold through a frame.
    $slider=Find-UI '播放位置'
    $r=$slider.Current.BoundingRectangle
    $max=$slider.GetCurrentPattern([System.Windows.Automation.RangeValuePattern]::Pattern).Current.Maximum
    $x=$r.X+$r.Height/2+($r.Width-$r.Height)*$seconds/$max
    if(-not [SmokeNative]::PointerClick(([SmokeNative]::TestWindow($p.Id)),$p.Id,[int]$x,[int]($r.Y+$r.Height/2),$false,80)){throw 'Native slider seek could not be sent'}
    Start-Sleep -Milliseconds 350
    if([Math]::Abs((Read-PlaybackPosition)-$seconds) -gt 1){throw 'Native slider did not seek to the requested test position'}
}
function Visible-ClickRow {
    Wait-VisibleSubtitle 'Click subtitle 156'
    $list=Find-UI '字幕列表'
    $row=$list.FindFirst([System.Windows.Automation.TreeScope]::Descendants,[System.Windows.Automation.PropertyCondition]::new([System.Windows.Automation.AutomationElement]::NameProperty,'Click subtitle 156'))
    return $row.Current.BoundingRectangle
}
function Pointer-ClickRow($r) {
    if(-not [SmokeNative]::PointerClick(([SmokeNative]::TestWindow($p.Id)),$p.Id,[int]($r.X+$r.Width/2),[int]($r.Y+$r.Height/2),$false)){throw 'Native row click could not be sent'}
}
function Select-NativeOption($field,$label) {
    # Select's popover uses its actual trigger, not a duplicate field label.
    $root=[System.Windows.Automation.AutomationElement]::FromHandle(([SmokeNative]::TestWindow($p.Id)))
    $condition=[System.Windows.Automation.PropertyCondition]::new([System.Windows.Automation.AutomationElement]::NameProperty,$field)
    $trigger=$null
    foreach($node in $root.FindAll([System.Windows.Automation.TreeScope]::Descendants,$condition)) {
        $pattern=$null
        if($node.TryGetCurrentPattern([System.Windows.Automation.ExpandCollapsePattern]::Pattern,[ref]$pattern)){$trigger=$node;break}
    }
    if(-not $trigger){throw "Native select trigger unavailable: $field"}
    Pointer-ClickRow $trigger.Current.BoundingRectangle
    for($i=0;$i -lt 30;$i++){if(Find-UI $label){break};Start-Sleep -Milliseconds 100}
    $choice=Find-UI $label
    if(-not $choice){throw "Native select option unavailable: $field / $label"}
    if(($field -eq '单击字幕' -or $field -eq '双击字幕') -and (Find-UI '定位，保持播放状态')){throw 'Retired seek action is still in a click menu'}
    Pointer-ClickRow $choice.Current.BoundingRectangle
    Start-Sleep -Milliseconds 300
}
function Set-SingleClickAction($label) {
    Click-UI '设置';Select-Tab '设置分类' '常规'
    Select-NativeOption '单击字幕' $label
    Click-UI '关闭'
}
function Wait-PlaybackStarted {
    for($i=0;$i -lt 50;$i++){if(Find-UI '暂停'){return};Start-Sleep -Milliseconds 50}
    throw 'Playback did not start before the row-click scenario'
}
function Test-SubtitleClicks {
    $wait=[int][SmokeNative]::GetDoubleClickTime()+400
    Click-UI '设置';Select-Tab '设置分类' '常规'
    Select-NativeOption '单击字幕' '定位并暂停'
    Select-NativeOption '双击字幕' '定位并播放'
    Click-UI '关闭'
    Set-PlaybackPosition 20.2;Click-UI '播放';Wait-PlaybackStarted;Click-UI '返回底部'
    $before=Visible-ClickRow
    Pointer-ClickRow $before
    Start-Sleep -Milliseconds 80
    $between=Visible-ClickRow
    if([Math]::Abs($before.Y-$between.Y) -gt 1 -or -not (Find-UI '暂停') -or (Read-PlaybackPosition) -gt 25){throw 'First click moved the row or paused/seeks before double-click detection'}
    Pointer-ClickRow $before
    Start-Sleep -Milliseconds 400
    if(-not (Find-UI '暂停') -or (Read-PlaybackPosition) -lt 155 -or (Read-PlaybackPosition) -gt 158){throw 'Double-click did not seek and play the intended subtitle'}
    # A lone pause click works during playback, without forced centering.
    Click-UI '暂停';Set-PlaybackPosition 20.2;Click-UI '播放';Wait-PlaybackStarted;Click-UI '返回底部'
    $before=Visible-ClickRow;Pointer-ClickRow $before
    Start-Sleep -Milliseconds $wait
    $after=Visible-ClickRow
    if(-not (Find-UI '播放') -or [Math]::Abs((Read-PlaybackPosition)-155) -gt .05 -or [Math]::Abs($before.Y-$after.Y) -gt 1){throw 'Single seek/pause failed or centered the clicked row'}
    Capture-Navigation 'single-pause.png'
    # Select-only holds both selection and viewport until the locate command,
    # including pause/resume. Playback itself must keep advancing.
    Set-SingleClickAction '仅选择字幕'
    Set-PlaybackPosition 20.2;Click-UI '播放';Wait-PlaybackStarted;Click-UI '返回底部'
    $before=Visible-ClickRow;Pointer-ClickRow $before
    Start-Sleep -Milliseconds $wait
    Start-Sleep -Seconds 5
    $selected=(Input-UI '字幕文字').GetCurrentPattern([System.Windows.Automation.ValuePattern]::Pattern).Current.Value
    $after=Visible-ClickRow
    if($selected -ne 'Click subtitle 156' -or [Math]::Abs($before.Y-$after.Y) -gt 1 -or (Read-PlaybackPosition) -gt 35 -or -not (Find-UI '暂停')){throw 'Select-only did not retain selection/viewport or changed playback'}
    Click-UI '暂停';Click-UI '播放';Start-Sleep -Milliseconds 400
    if((Input-UI '字幕文字').GetCurrentPattern([System.Windows.Automation.ValuePattern]::Pattern).Current.Value -ne 'Click subtitle 156'){throw 'Playback resume released explicit selection hold'}
    Click-UI '回到当前字幕';Start-Sleep -Milliseconds 400
    if((Input-UI '字幕文字').GetCurrentPattern([System.Windows.Automation.ValuePattern]::Pattern).Current.Value -eq 'Click subtitle 156'){throw 'Locate command did not release follow hold'}
    # Seek/play must work while the video is already playing, then keep following.
    Click-UI '暂停';Set-SingleClickAction '定位并播放'
    Set-PlaybackPosition 60.2;Click-UI '播放';Wait-PlaybackStarted;Click-UI '返回底部'
    Pointer-ClickRow (Visible-ClickRow)
    Start-Sleep -Milliseconds $wait
    if(-not (Find-UI '暂停') -or (Read-PlaybackPosition) -lt 155 -or (Read-PlaybackPosition) -gt 158){throw 'Single seek/play did not work during playback'}
    Set-PlaybackPosition 80.2;Start-Sleep -Milliseconds 300
    $selected=(Input-UI '字幕文字').GetCurrentPattern([System.Windows.Automation.ValuePattern]::Pattern).Current.Value
    if($selected -notmatch '^Click subtitle 08[1-3]$'){throw "Seek/play stopped later follow: $selected"}
    Wait-VisibleSubtitle $selected
    Capture-Navigation 'single-play-follow.png'
    Write-Output 'PASS: native pointer double-click during playback; lone pause without centering; select-only hold including pause/resume and locate; single play during playback and later follow'
}
try {
    Start-Sleep -Seconds 4
    if($p.HasExited){throw "Test application exited at startup: $($p.ExitCode)"}
    if(-not $OpenArgument){Click-UI '添加文件'; Choose-Path $video}
    for($i=0;$i -lt 150;$i++){if(Find-UI '字幕文字'){break};Start-Sleep -Milliseconds 100}
    if(-not (Find-UI '字幕文字')){
        & $ScreenshotTool $p.Id (Join-Path $workPath 'failure.png')
        $p.Refresh();$root=[System.Windows.Automation.AutomationElement]::FromHandle(([SmokeNative]::TestWindow($p.Id)))
        $root.FindAll([System.Windows.Automation.TreeScope]::Descendants,[System.Windows.Automation.Condition]::TrueCondition) | ForEach-Object {$_.Current.Name}
        throw 'Same-name subtitle was not loaded'
    }
    if($SubtitleClicks){Test-SubtitleClicks;return}
    if($WorkflowLayout){[void][SmokeNative]::ShowWindow(([SmokeNative]::TestWindow($p.Id)),3);Start-Sleep -Milliseconds 800}
    # Playback is exercised through UIA on the private desktop. This does not
    # claim a global keyboard-input test on the user's interactive desktop.
    Click-UI '播放'
    Start-Sleep -Seconds 2
    if(-not (Find-UI '暂停')){
        & $ScreenshotTool $p.Id (Join-Path $workPath 'failure.png')
        throw 'Playback did not start'
    }
    Click-UI '暂停'
    for($i=0;$i -lt 30;$i++){if(Find-UI '播放'){break};Start-Sleep -Milliseconds 100}
    if(-not (Find-UI '播放')){throw 'Playback did not pause'}
    # Muting updates the native slider as well as the audio, and restores it.
    $volume=(Find-UI '音量').GetCurrentPattern([System.Windows.Automation.RangeValuePattern]::Pattern).Current.Value
    Click-UI '静音';Start-Sleep -Milliseconds 300
    if((Find-UI '音量').GetCurrentPattern([System.Windows.Automation.RangeValuePattern]::Pattern).Current.Value -ne 0){throw 'Mute did not zero the volume slider'}
    Click-UI '取消静音';Start-Sleep -Milliseconds 300
    if((Find-UI '音量').GetCurrentPattern([System.Windows.Automation.RangeValuePattern]::Pattern).Current.Value -ne $volume){throw 'Unmute did not restore the previous volume'}
    & $ScreenshotTool $p.Id (Join-Path $workPath 'editor.png')
    $initialStyle=(Read-Preferences).lastStyle
    $bold=Find-UI '粗体'
    if($bold){
        $bold.GetCurrentPattern([System.Windows.Automation.TogglePattern]::Pattern).Toggle()
        Start-Sleep -Seconds 2
        & $ScreenshotTool $p.Id (Join-Path $workPath 'style-bold.png')
    }
    if($PresetReset) {
        $size=Input-UI '字号'
        $size.GetCurrentPattern([System.Windows.Automation.ValuePattern]::Pattern).SetValue('31')
        Start-Sleep -Seconds 1
        if((Read-Preferences).lastStyle.fontSize -ne 31){throw 'Style edit was not saved before reset'}
        Scroll-PanelEnd '字幕样式面板'
        Click-UI '预设操作'
        $items=Wait-SubtitleMenu
        if($items -notcontains '恢复默认'){throw 'Preset menu has no reset action'}
        Choose-SubtitleMenu '恢复默认'
        Start-Sleep -Seconds 1
        $restored=Read-Preferences
        if($restored.stylePreset -ne 'default' -or ($restored.lastStyle|ConvertTo-Json -Compress) -ne ($initialStyle|ConvertTo-Json -Compress)){throw 'Preset reset did not restore and save the original style'}
        & $ScreenshotTool $p.Id (Join-Path $workPath 'style-restored.png')
        Write-Output 'PASS: built-in preset restored through its native three-dot menu and persisted'
    }
    if($ChoiceControls){
        . ([ScriptBlock]::Create([IO.File]::ReadAllText((Join-Path (Split-Path $ScreenshotTool) 'choice_controls.ps1'),[Text.Encoding]::UTF8)))
        Test-ChoiceControls
        return
    }

    Click-UI '设置'
    & $ScreenshotTool $p.Id (Join-Path $workPath 'settings.png')
    if(Find-UI '设置分类'){
        foreach($tab in @('识别与模型','语言与模型','翻译规则','MCP 服务','关于','翻译服务')){
            Select-Tab '设置分类' $tab
            if($tab -eq '翻译服务' -and ((Find-UI '并发批数') -or (Find-UI '每批字幕数'))){throw 'Task scheduling is still in service settings'}
            if($tab -eq '关于') {
                $check=Find-UI '检查更新'
                $version=Find-UI '版本 0.0.1'
                if(-not $check -or -not $version -or -not (Find-UI 'https://github.com/OUBIGFA/OneSub')){throw 'About lost its version, update action or project link'}
                $button=$check.Current.BoundingRectangle
                $badge=$version.Current.BoundingRectangle
                if($button.Top -gt $badge.Bottom -or $button.Bottom -lt $badge.Top){throw 'About update action is detached from version information'}
            }
            & $ScreenshotTool $p.Id (Join-Path $workPath ("settings-"+$tab+'.png'))
        }
    }
    if($ProviderCatalogEndpoint) {
        if((Invoke-RestMethod "$ProviderCatalogEndpoint/test-state").requests -ne 0){throw 'Opening settings fetched models automatically'}
        if((Find-UI '获取模型失败') -or -not (Find-UI '保留模型 catalog-cached')){throw 'Saved catalog was not restored without discovery'}
        Select-Value '接口配置'
        # This is not a real service credential. The fixture verifies that a
        # manual fetch reads the saved key from this exact account scope.
        (Input-UI 'API Key').GetCurrentPattern([System.Windows.Automation.ValuePattern]::Pattern).SetValue('onesub-local-catalog-test')
        Click-UI '保存 Key'
        Select-Value '接口配置'
        $keyBounds=(Input-UI 'API Key').Current.BoundingRectangle
        for($reveal=0;$reveal -lt 3;$reveal++) {
            Click-UI '显示密钥'
            $keyInput=Input-UI 'API Key'
            if($keyInput.Current.IsPassword -or $keyInput.GetCurrentPattern([System.Windows.Automation.ValuePattern]::Pattern).Current.Value -ne 'onesub-local-catalog-test'){throw 'Saved key was not revealed'}
            Click-UI '隐藏密钥'
            $keyInput=Input-UI 'API Key'
            if(-not $keyInput.Current.IsPassword -or $keyInput.Current.BoundingRectangle -ne $keyBounds){throw 'Key did not remask or its field moved'}
        }
        & $ScreenshotTool $p.Id (Join-Path $workPath 'api-settings.png')
        Select-Value '模型目录'
        if((Invoke-RestMethod "$ProviderCatalogEndpoint/test-state").requests -ne 0){throw 'Saving a key fetched models automatically'}
        Select-NativeOption '供应商' '本地目录 B'
        if((Read-Preferences).providerModel -ne 'catalog-other' -or -not (Find-UI '保留模型 catalog-other')){throw 'Switching provider lost its saved model/catalog'}
        Select-NativeOption '供应商' '本地目录 A'
        if((Read-Preferences).providerModel -ne 'catalog-alpha' -or -not (Find-UI '保留模型 catalog-cached') -or (Invoke-RestMethod "$ProviderCatalogEndpoint/test-state").requests -ne 0){throw 'Provider switching fetched a catalog or lost saved choices'}
        Select-Value '接口配置'
        $address=(Input-UI '服务地址').GetCurrentPattern([System.Windows.Automation.ValuePattern]::Pattern)
        $address.SetValue($ProviderCatalogEndpoint+'/');Start-Sleep -Milliseconds 200
        $address=(Input-UI '服务地址').GetCurrentPattern([System.Windows.Automation.ValuePattern]::Pattern)
        $address.SetValue($ProviderCatalogEndpoint);Start-Sleep -Milliseconds 200
        Select-Value '模型目录'
        if((Invoke-RestMethod "$ProviderCatalogEndpoint/test-state").requests -ne 0){throw 'Editing an endpoint fetched models automatically'}
        Click-UI '获取模型'
        for($i=0;$i -lt 100;$i++){if(Find-UI '获取模型失败'){break};Start-Sleep -Milliseconds 100}
        if(-not (Find-UI '获取模型失败')){throw 'Failed manual fetch did not report error'}
        # Retry on the same saved provider; switching must not be necessary.
        Click-UI '获取模型'
        for($i=0;$i -lt 100;$i++){if(Find-UI '保留模型 catalog-beta'){break};Start-Sleep -Milliseconds 100}
        if((Invoke-RestMethod "$ProviderCatalogEndpoint/test-state").requests -ne 2 -or (Find-UI '获取模型失败')){throw 'Same-provider retry failed or sent extra requests'}
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
        $names=Wait-ModelChoices
        if($names.Count -ne 2 -or $names -notcontains 'catalog-alpha' -or $names -notcontains 'catalog-beta'){throw 'Model dropdown must contain only enabled models'}
        Select-Choice 'catalog-beta'
        Start-Sleep -Milliseconds 400
        (Input-UI '搜索模型').GetCurrentPattern([System.Windows.Automation.ValuePattern]::Pattern).SetValue('alpha')
        Start-Sleep -Milliseconds 250
        if(Find-UI '保留模型 catalog-beta'){throw 'Search did not filter the model table'}
        (Input-UI '搜索模型').GetCurrentPattern([System.Windows.Automation.ValuePattern]::Pattern).SetValue('')
        Click-UI '获取模型';Start-Sleep -Milliseconds 500
        if((Read-Preferences).providerModel -ne 'catalog-beta' -or (Read-Preferences).providers[0].models.Count -ne 2){throw 'Refresh lost retained models'}
        if((Find-UI '设置内容').Current.BoundingRectangle -ne $before){throw 'Model state resized settings'}
        $used=(Find-UI '使用模型 catalog-alpha').Current.BoundingRectangle
        $unused=(Find-UI '使用模型 catalog-gamma').Current.BoundingRectangle
        $current=(Find-UI '使用模型 catalog-beta').Current.BoundingRectangle
        $catalog=(Find-UI '供应商模型目录').Current.BoundingRectangle
        if([Math]::Abs($used.Width-$unused.Width) -gt 1 -or [Math]::Abs($current.Width-$unused.Width) -gt 1 -or $unused.Right -gt $catalog.Right-12){throw 'Native model status widths differ or overlap the scrollbar gutter'}
        $header=(Find-UI '供应商模型目录').Current.BoundingRectangle
        [void][SmokeNative]::PointerClick(([SmokeNative]::TestWindow($p.Id)),$p.Id,[int]($header.X+24),[int]($header.Y+12),$false)
        Start-Sleep -Milliseconds 350
        & $ScreenshotTool $p.Id (Join-Path $workPath 'model-library.png')
        Select-Value '接口配置';Click-UI '删除 Key';Select-Value '模型目录'
        if((Invoke-RestMethod "$ProviderCatalogEndpoint/test-state").requests -ne 3){throw 'Key deletion or view switching fetched models automatically'}
        Write-Output 'PASS: saved catalogs restored; zero automatic GETs on entry/provider/key/endpoint changes; same-provider retry with saved credentials; equal native status widths and scrollbar gutter'
    }
    Click-UI '关闭'
    if($ProviderCatalogEndpoint) {
        Select-Tab '识别与翻译' 'AI 翻译'
        Click-UI '切换模型';$names=Wait-ModelChoices
        if($names.Count -ne 2 -or $names -notcontains 'catalog-beta'){throw 'Workbench dropdown did not use the same enabled model list'}
        Select-Choice 'catalog-beta'
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
                Click-UI '批次与标点设置';Scroll-PanelEnd '识别与翻译'
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
        # Center on the visible table, excluding the track tabs and toolbar.
        $list=(Find-UI '字幕列表').Current.BoundingRectangle
        if([Math]::Abs(($top.Top+$bottom.Bottom)/2-$list.Top-$list.Height/2) -gt 2){throw 'Floating navigation is not vertically centered in the list'}
        Click-UI '返回底部';Wait-VisibleSubtitle 'Navigation subtitle 120'
        if((Find-UI '字幕文字').GetCurrentPattern([System.Windows.Automation.ValuePattern]::Pattern).Current.Value -ne $draft){throw 'Bottom navigation changed the selected subtitle'}
        Capture-Navigation 'list-bottom.png'
        Click-UI '返回顶部';Wait-VisibleSubtitle 'Navigation subtitle 001'
        if((Find-UI '字幕文字').GetCurrentPattern([System.Windows.Automation.ValuePattern]::Pattern).Current.Value -ne $draft){throw 'Top navigation changed the selected subtitle'}
        if((Find-UI '返回顶部').Current.BoundingRectangle -ne $top -or (Find-UI '返回底部').Current.BoundingRectangle -ne $bottom){throw 'Floating buttons moved with the rows'}
        Capture-Navigation 'layout-navigation.png'
        Write-Output 'PASS: centered one-click toolbar action; style/editor/workflow 1.5:2:1.5; vertically centered top/bottom navigation; selected subtitle unchanged'
    }
    if($VideoTools) {
        $originalSubtitleHash=(Get-FileHash $subtitle -Algorithm SHA256).Hash
        $external=Join-Path $workPath 'different-zh.srt'
        [IO.File]::WriteAllText($external,"1`n00:00:00,100 --> 00:00:02,900`nImported translation`n",[Text.UTF8Encoding]::new($false))
        Click-UI '导入字幕';Choose-Path $external
        Wait-VisibleSubtitle 'Imported translation'
        $startTime=(Input-UI '开始').GetCurrentPattern([System.Windows.Automation.ValuePattern]::Pattern).Current.Value
        $endTime=(Input-UI '结束').GetCurrentPattern([System.Windows.Automation.ValuePattern]::Pattern).Current.Value
        if($startTime -ne '00:00:00.100' -or $endTime -ne '00:00:02.900'){throw 'Editor timestamps do not match the list/player format'}
        $input=Input-UI '字幕文字'
        $input.GetCurrentPattern([System.Windows.Automation.ValuePattern]::Pattern).SetValue('Reviewed translation')
        Wait-VisibleSubtitle 'Reviewed translation'
        Click-UI '保存'
        for($i=0;$i -lt 100;$i++){if([IO.File]::ReadAllText($external).Contains('Reviewed translation')){break};Start-Sleep -Milliseconds 100}
        if(-not [IO.File]::ReadAllText($external).Contains('Reviewed translation') -or (Get-FileHash $subtitle -Algorithm SHA256).Hash -ne $originalSubtitleHash){throw 'Independent import/save modified the wrong subtitle file'}
        Click-UI '合成视频';Click-UI '开始合成'
        $burned=Join-Path $workPath 'test-OneSub.mp4'
        for($i=0;$i -lt 600;$i++){if(Test-Path $burned){break};Start-Sleep -Milliseconds 100}
        if(-not (Test-Path $burned)){throw 'Nonmatching subtitle did not compose with the current video'}
        # Standalone media now retains current subtitles when no same-name
        # match exists. Clear through the confirmed UI action to test a new,
        # genuinely subtitle-free workbench rather than a media reassociation.
        # Move this fixture out of discovery without deleting a file permanently.
        Move-Item -LiteralPath $subtitle -Destination ($subtitle+'.saved')
        Click-UI '清空工作台';Click-UI '清空'
        Click-UI '添加文件';Choose-Path $video
        for($i=0;$i -lt 100;$i++){if(Find-UI '未加载字幕'){break};Start-Sleep -Milliseconds 100}
        if(-not (Find-UI '未加载字幕')){throw 'Subtitle-free import was not empty'}
        Click-UI '音视频处理'
        if(Find-UI '字幕内容'){throw 'Compression exposes subtitle output options'}
        Click-UI '开始压缩'
        $compressed=Join-Path $workPath 'test-OneSub-compressed.mp4'
        for($i=0;$i -lt 600;$i++){if(Test-Path $compressed){break};Start-Sleep -Milliseconds 100}
        if(-not (Test-Path $compressed)){throw 'Subtitle-free compression did not publish a video'}
        $streams=(& ffprobe -v error -show_streams -of json $compressed | Out-String | ConvertFrom-Json).streams
        if($LASTEXITCODE -ne 0 -or @($streams|Where-Object codec_type -eq 'video').Count -ne 1 -or @($streams|Where-Object codec_type -eq 'subtitle').Count -ne 0){throw 'Compression added subtitles or lost video'}
        Click-UI '音视频处理'
        Select-NativeOption '处理方式' '提取音频'
        if((Read-Preferences).audioFormat -ne 'mp3' -or (Find-UI '编码质量')){throw 'Audio mode did not default to MP3 or still shows video quality'}
        & $ScreenshotTool $p.Id (Join-Path $workPath 'audio-tools.png')
        Click-UI '开始提取'
        $audio=Join-Path $workPath 'test-OneSub-audio.mp3'
        for($i=0;$i -lt 600;$i++){if(Test-Path $audio){break};Start-Sleep -Milliseconds 100}
        if(-not (Test-Path $audio)){throw 'Audio extraction did not publish MP3'}
        $streams=(& ffprobe -v error -show_streams -of json $audio | Out-String | ConvertFrom-Json).streams
        if($LASTEXITCODE -ne 0 -or @($streams).Count -ne 1 -or $streams[0].codec_type -ne 'audio' -or $streams[0].codec_name -ne 'mp3'){throw 'Audio extraction produced incorrect streams'}
        & ffmpeg -nostdin -v error -xerror -i $audio -f null -
        if($LASTEXITCODE -ne 0){throw 'Extracted MP3 cannot be decoded'}
        Capture-Navigation 'video-tools.png'
        Write-Output 'PASS: independent subtitle import/edit/save/compose; video compression and MP3 extraction without subtitles; MP3 decoded'
    }
    if($SubtitleMenus) {
        # Invoke closes the modal asynchronously. Wait for the workbench's
        # UIA nodes before measuring, as Select-Tab does after dialog changes.
        for($i=0;$i -lt 100;$i++){
            if((Find-UI '字幕列表操作') -and (Find-UI '字幕列表') -and (Find-UI '字幕编辑面板')){break}
            Start-Sleep -Milliseconds 100
        }
        $menu=(Find-UI '字幕列表操作').Current.BoundingRectangle
        $list=(Find-UI '字幕列表').Current.BoundingRectangle
        $editor=Find-UI '字幕编辑面板'
        $name=[System.Windows.Automation.PropertyCondition]::new([System.Windows.Automation.AutomationElement]::NameProperty,'字幕列表操作')
        if($menu.Left -lt $list.Left -or $menu.Right -gt $list.Right -or $menu.Bottom -gt $list.Top -or $editor.FindFirst([System.Windows.Automation.TreeScope]::Descendants,$name)){throw 'Structure menu is not confined to the subtitle list'}
        $sourceHash=(Get-FileHash $subtitle -Algorithm SHA256).Hash
        Open-SubtitleMenu 'Second subtitle.'
        $items=Wait-SubtitleMenu
        # UIA returns actionable items; separators have their own role. The
        # current menu also includes retranslation, location and split actions.
        if(($items -join '|') -ne '勾选／取消此条|重译 · 0 条|全选|取消勾选|定位视频|在前面插入字幕|在后面插入字幕|智能拆分|强制拆分|与上一条合并|与下一条合并|删除字幕'){throw "Incorrect native menu items: $items"}
        Choose-SubtitleMenu '与上一条合并'
        Wait-VisibleSubtitle 'First subtitle, Second subtitle.'
        if(-not (Find-UI '# 1') -or (Input-UI '开始').GetCurrentPattern([System.Windows.Automation.ValuePattern]::Pattern).Current.Value -ne '00:00:00.100' -or (Input-UI '结束').GetCurrentPattern([System.Windows.Automation.ValuePattern]::Pattern).Current.Value -ne '00:00:01.900'){throw 'Previous merge lost numbering or its full time range'}
        Open-SubtitleMenu '第一段字幕。';Choose-SubtitleMenu '与下一条合并'
        Wait-VisibleSubtitle '第一段字幕，第二段字幕。'
        if(-not (Find-UI '# 2')){throw 'Next merge did not renumber the selected subtitle'}
        Click-UI '撤销';Wait-VisibleSubtitle '，第二段字幕。'
        Click-UI '重做';Wait-VisibleSubtitle '第一段字幕，第二段字幕。'
        # The list toolbar exposes the same merge commands and each merge is
        # one undo step. Returning to two cues is the saved result below.
        Click-UI '字幕列表操作';Choose-SubtitleMenu '与上一条合并'
        Wait-VisibleSubtitle 'First subtitle, Second subtitle，第一段字幕，第二段字幕。'
        Click-UI '撤销';Wait-VisibleSubtitle 'First subtitle, Second subtitle.'
        Wait-VisibleSubtitle '第一段字幕，第二段字幕。'
        # The visible label is Unsaved, while the accessible action remains
        # Save. Enabled Save and the unchanged file verify the pending write.
        if((Get-FileHash $subtitle -Algorithm SHA256).Hash -ne $sourceHash -or -not (Find-UI '保存').Current.IsEnabled){throw 'Merge silently wrote the source file or did not enable saving'}
        Click-UI '保存'
        $expected="1`n00:00:00,100 --> 00:00:01,900`nFirst subtitle, Second subtitle.`n`n2`n00:00:02,000 --> 00:00:02,900`n第一段字幕，第二段字幕。"
        for($i=0;$i -lt 100;$i++){
            $saved=[IO.File]::ReadAllText($subtitle).Replace("`r`n","`n").TrimEnd([char]10)
            if($saved -eq $expected){break}
            Start-Sleep -Milliseconds 100
        }
        if($saved -ne $expected){throw 'Saved merge text, times or continuous numbering differ from the two cues'}
        Capture-Navigation 'subtitle-merge.png'
        Write-Output 'PASS: native context/toolbar previous/next merges; English/Chinese comma boundaries; full time ranges; single-step undo/redo; source unchanged until save; saved continuous numbering'
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
            # Recovery must restore playable media as well as subtitle text.
            for($i=0;$i -lt 100;$i++){
                $slider=Find-UI '播放位置'
                if($slider -and $slider.GetCurrentPattern([System.Windows.Automation.RangeValuePattern]::Pattern).Current.Maximum -gt 2.9){break}
                Start-Sleep -Milliseconds 100
            }
            if(-not $slider -or $slider.GetCurrentPattern([System.Windows.Automation.RangeValuePattern]::Pattern).Current.Maximum -lt 2.9){throw 'Restart did not restore media duration'}
            Click-UI '播放'
            Wait-PlaybackStarted
            Start-Sleep -Milliseconds 500
            if((Read-PlaybackPosition) -le 0){throw 'Recovered video did not advance'}
            Click-UI '暂停'
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
        $sourceHash=(Get-FileHash $subtitle -Algorithm SHA256).Hash
        $exportRoot=Join-Path $workPath 'exports'
        New-Item -ItemType Directory $exportRoot | Out-Null
        foreach($format in @('srt','txt')) {
            foreach($choice in @(@{id='all';label='全部字幕行';kept=$sourceText},@{id='upper';label='仅上行字幕';kept='Upper subtitle'},@{id='lower';label='仅下行字幕';kept='下行字幕'})) {
                Click-UI '导出字幕'
                for($i=0;$i -lt 100;$i++){if(Find-UI '保留字幕行'){break};Start-Sleep -Milliseconds 100}
                $action=(Find-UI '导出').Current.BoundingRectangle
                Select-Value $format.ToUpperInvariant()
                # The select's current-value text is not an actionable UIA
                # option. Reuse the driver that clicks its real popup trigger.
                Select-NativeOption '保留字幕行' $choice.label
                if((Find-UI '导出').Current.BoundingRectangle -ne $action){throw 'Format/line choice moved the export action'}
                if($format -eq 'txt' -and -not (Find-UI '仅导出文字，不含序号、时间轴或样式')){throw 'TXT note is missing'}
                $heading=(Find-UI '导出字幕').Current.BoundingRectangle
                if(-not [SmokeNative]::PointerClick(([SmokeNative]::TestWindow($p.Id)),$p.Id,[int]($heading.X+24),[int]($heading.Y+16),$false)){throw 'Could not clear export focus adornments'}
                Start-Sleep -Milliseconds 350
                & $ScreenshotTool $p.Id (Join-Path $workPath ("export-"+$choice.id+'-'+$format+'.png'))
                if($LASTEXITCODE -ne 0){throw 'Export screenshot failed'}
                Click-UI '导出'
                $output=Join-Path $exportRoot ("export-"+$choice.id+'.'+$format)
                # Also exercise extension completion through the real picker.
                $chosen=if($format -eq 'txt' -and $choice.id -eq 'all'){[IO.Path]::ChangeExtension($output,$null)}else{$output}
                Choose-Path $chosen
                for($i=0;$i -lt 100;$i++){if(Test-Path -LiteralPath $output){break};Start-Sleep -Milliseconds 100}
                if(-not (Test-Path -LiteralPath $output)){throw 'Subtitle/text export did not create the selected file'}
                $saved=[IO.File]::ReadAllText($output)
                if($format -eq 'txt'){
                    $expected=[Text.Encoding]::UTF8.GetBytes($choice.kept+"`n")
                    if([BitConverter]::ToString([IO.File]::ReadAllBytes($output)) -ne [BitConverter]::ToString($expected)){throw 'TXT contains wrong text, times, numbers, styling or a BOM'}
                }else{
                    $expected="1`n00:00:00,100 --> 00:00:02,900`n"+$choice.kept
                    if($saved.Replace("`r`n","`n").TrimEnd([char]10) -ne $expected){throw 'Timed export changed content, numbering or time'}
                }
            }
        }
        if((Get-FileHash $subtitle -Algorithm SHA256).Hash -ne $sourceHash){throw 'Export changed the bilingual source file'}
        if(@(Get-ChildItem -LiteralPath $exportRoot -Force).Count -ne 6){throw 'Export directory contains unrelated files'}
        Write-Output 'PASS: packaged SRT/TXT all/upper/lower exports; exact UTF-8 text without BOM/times/numbers/styling; extension completion; fixed dialog action; source unchanged; only requested files'
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
    Write-Output "PASS: import (argument=$OpenArgument), same-name subtitle, playback/pause=$inputMode, settings, translation=$([bool]$TranslationEndpoint), upper/lower export=$BilingualExport, subtitle merge/menu=$SubtitleMenus"
} catch {
    Write-Output ("FAIL: native smoke: " + $_.Exception.Message)
    $p.Refresh()
    if(-not $p.HasExited -and ([SmokeNative]::TestWindow($p.Id)) -ne 0) {
        # Capture the actual modal/workbench state before teardown so a picker
        # or save guard failure is not mistaken for a failed import service.
        & $ScreenshotTool $p.Id (Join-Path $workPath 'failure.png') | Out-Host
        $root=[System.Windows.Automation.AutomationElement]::FromHandle(([SmokeNative]::TestWindow($p.Id)))
        $root.FindAll([System.Windows.Automation.TreeScope]::Descendants,[System.Windows.Automation.Condition]::TrueCondition) | ForEach-Object { Write-Output ("UIA: " + $_.Current.Name) }
    }
    throw
} finally {
    # A failed menu assertion must close this test-owned popup before WM_CLOSE;
    # otherwise its native menu loop can keep the test window alive.
    $p.Refresh()
    if(-not $p.HasExited){[void][SmokeNative]::PostMessage(([SmokeNative]::TestWindow($p.Id)),31,[IntPtr]::Zero,[IntPtr]::Zero);Start-Sleep -Milliseconds 200}
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
