param([Parameter(Mandatory=$true)][string]$Exe, [Parameter(Mandatory=$true)][string]$Work, [Parameter(Mandatory=$true)][string]$ScreenshotTool, [ValidateSet('light','dark')][string]$Appearance='light', [ValidateSet('zh-Hans','en')][string]$Language='zh-Hans', [switch]$CheckFeed)
$ErrorActionPreference='Stop'
if(-not ('PrivateSmoke' -as [type]) -or -not [PrivateSmoke]::IsPrivateDesktop()){throw '通过私有桌面启动器运行设置页验收'}
Add-Type -AssemblyName UIAutomationClient
Add-Type -AssemblyName UIAutomationTypes
Add-Type @'
using System;
using System.Runtime.InteropServices;
public static class AboutTestWindow {
 public delegate bool Visitor(IntPtr h, IntPtr p);
 [DllImport("user32.dll")] static extern bool EnumDesktopWindows(IntPtr desktop, Visitor fn, IntPtr p);
 [DllImport("user32.dll")] static extern IntPtr GetThreadDesktop(uint thread);
 [DllImport("kernel32.dll")] static extern uint GetCurrentThreadId();
 [StructLayout(LayoutKind.Sequential)] public struct Point { public int X,Y; }
 [DllImport("user32.dll")] public static extern bool SetProcessDPIAware();
 [DllImport("user32.dll")] static extern bool EnumChildWindows(IntPtr h, Visitor fn, IntPtr p);
 [DllImport("user32.dll")] static extern uint GetWindowThreadProcessId(IntPtr h, out uint pid);
 [DllImport("user32.dll", CharSet=CharSet.Unicode)] static extern int GetClassName(IntPtr h, System.Text.StringBuilder b, int n);
 [DllImport("user32.dll")] static extern bool ScreenToClient(IntPtr h, ref Point p);
 [DllImport("user32.dll")] public static extern bool PostMessage(IntPtr h, uint m, IntPtr w, IntPtr l);
 [DllImport("user32.dll")] public static extern bool IsWindow(IntPtr h);
 public static IntPtr TestWindow(int pid) {
  IntPtr result=IntPtr.Zero;
  EnumDesktopWindows(GetThreadDesktop(GetCurrentThreadId()),(window,context)=>{
   uint owner;GetWindowThreadProcessId(window,out owner);
   var name=new System.Text.StringBuilder(128);GetClassName(window,name,128);
   if(owner==(uint)pid && name.ToString()=="MyGoWindow"){result=window;return false;}
   return true;
  },IntPtr.Zero);
  return result;
 }
 // The existing native smoke driver uses the same test-owned surface route.
 // Never send a physical click that could land on a foreground user window.
 public static bool ClearFocus(IntPtr window, int pid, int x, int y) {
  IntPtr surface=IntPtr.Zero;
  EnumChildWindows(window,(child,p)=>{
   uint owner;GetWindowThreadProcessId(child,out owner);
   var name=new System.Text.StringBuilder(128);GetClassName(child,name,128);
   if(owner==pid && name.ToString()=="MyGoSurface"){surface=child;return false;}
   return true;
  },IntPtr.Zero);
  if(surface==IntPtr.Zero)return false;
  var point=new Point{X=x,Y=y};
  if(!ScreenToClient(surface,ref point))return false;
  var coords=new IntPtr((point.Y<<16)|(point.X&0xffff));
  return PostMessage(surface,512,IntPtr.Zero,coords) &&
   PostMessage(surface,513,new IntPtr(1),coords) && PostMessage(surface,514,IntPtr.Zero,coords);
 }
}
'@
[void][AboutTestWindow]::SetProcessDPIAware()
$workPath=[IO.Path]::GetFullPath($Work)
$exePath=[IO.Path]::GetFullPath($Exe)
if(Test-Path $workPath){throw 'Use a fresh about-settings test directory'}
New-Item -ItemType Directory "$workPath/data","$workPath/ipc" -Force | Out-Null
if("$workPath/ipc".Length -gt 65){throw 'Use a shorter isolated IPC path'}
$names=@('ONESUB_DATA_ROOT','XDG_RUNTIME_DIR','WEBVIEW2_USER_DATA_FOLDER','ONESUB_SUBTITLE_WORKER','ONESUB_STYLE_WORKER','ONESUB_SKILL_WORKER','ONESUB_SKILL_ROOT','ONESUB_FFMPEG','ONESUB_FFPROBE','ONESUB_ONEASR_ROOT','ONESUB_SHERPA_ROOT')
$previous=@{}
foreach($name in $names){$previous[$name]=[Environment]::GetEnvironmentVariable($name,'Process')}
$existing=@(Get-Process OneSub -ErrorAction SilentlyContinue | ForEach-Object {$_.Id})
$env:ONESUB_DATA_ROOT="$workPath/data"
$env:XDG_RUNTIME_DIR="$workPath/ipc"
$env:WEBVIEW2_USER_DATA_FOLDER="$workPath/webview"
foreach($name in $names | Where-Object {$_ -notin @('ONESUB_DATA_ROOT','XDG_RUNTIME_DIR','WEBVIEW2_USER_DATA_FOLDER')}){[Environment]::SetEnvironmentVariable($name,$null,'Process')}
$preferences="$workPath/data/settings.json"
# Omit autoUpdate to exercise the actual default, not a pre-checked fixture.
[IO.File]::WriteAllText($preferences,(@{version=5;uiLanguage=$Language;appearance=$Appearance;closeToTray=$false}|ConvertTo-Json),[Text.UTF8Encoding]::new($false))
$text=if($Language -eq 'en'){@{settings='Settings';about='About';automatic='Check and install updates automatically';check='Check for updates';close='Close';content='Settings content';missing='No update package published yet';version='Version 0.1.0'}}else{@{settings='设置';about='关于';automatic='自动检查并安装更新';check='检查更新';close='关闭';content='设置内容';missing='尚无可用更新包';version='版本 0.1.0'}}
$p=$null
function Start-TestWindow {
 # Return the process to the caller's scope. ScriptBlock UTF-8 invocation
 # does not give this function the same $script: scope as a file invocation.
 return Start-Process -FilePath $exePath -RedirectStandardError "$workPath/stderr.log" -PassThru
}
function Find-All($name) {
 $p.Refresh()
 $window=[AboutTestWindow]::TestWindow($p.Id)
 if($p.HasExited -or $window -eq [IntPtr]::Zero){return @()}
 $root=[System.Windows.Automation.AutomationElement]::FromHandle($window)
 return $root.FindAll([System.Windows.Automation.TreeScope]::Descendants,[System.Windows.Automation.PropertyCondition]::new([System.Windows.Automation.AutomationElement]::NameProperty,$name))
}
function Wait-UI($name,$requiredPattern=$null) {
 for($i=0;$i -lt 150;$i++){
  try {
   foreach($item in (Find-All $name)){
    $pattern=$null
    if($requiredPattern -and -not $item.TryGetCurrentPattern($requiredPattern,[ref]$pattern)){continue}
    return $item
   }
  }catch [System.Windows.Automation.ElementNotAvailableException]{}
  Start-Sleep -Milliseconds 100
 }
 throw "Control unavailable: $name"
}
function Click-UI($name) {
 for($i=0;$i -lt 100;$i++){
  try {
   foreach($item in (Find-All $name)){
    if(-not $item.Current.IsEnabled){continue}
    $pattern=$null
    if($item.TryGetCurrentPattern([System.Windows.Automation.InvokePattern]::Pattern,[ref]$pattern)){$pattern.Invoke();Start-Sleep -Milliseconds 250;return}
    if($item.TryGetCurrentPattern([System.Windows.Automation.SelectionItemPattern]::Pattern,[ref]$pattern)){$pattern.Select();Start-Sleep -Milliseconds 250;return}
    if($item.TryGetCurrentPattern([System.Windows.Automation.TogglePattern]::Pattern,[ref]$pattern)){$pattern.Toggle();Start-Sleep -Milliseconds 250;return}
   }
  }catch [System.Windows.Automation.ElementNotAvailableException]{}
  Start-Sleep -Milliseconds 100
 }
 throw "Action unavailable: $name"
}
function Capture($name) {
 # Send native pointer events only to this PID's blank content. UIA
 # invocation and screenshot foreground changes cannot redirect the click.
 $area=(Wait-UI $text.content).Current.BoundingRectangle
 if(-not [AboutTestWindow]::ClearFocus(([AboutTestWindow]::TestWindow($p.Id)),$p.Id,[int]($area.Right-12),[int]($area.Top+8))){throw 'Could not clear focus on the test-owned surface'}
 Start-Sleep -Milliseconds 250
 & $ScreenshotTool $p.Id (Join-Path $workPath $name)
 if(-not $?){throw 'Native About screenshot failed'}
}
function Close-TestWindow {
 if($p -and -not $p.HasExited){
  [void][AboutTestWindow]::PostMessage(([AboutTestWindow]::TestWindow($p.Id)),16,[IntPtr]::Zero,[IntPtr]::Zero)
  if(-not $p.WaitForExit(10000)){throw 'The test-owned OneSub did not close normally'}
 }
}
try {
 $p=Start-TestWindow
 Click-UI $text.settings
 Click-UI $text.about
 [void](Wait-UI $text.version)
 [void](Wait-UI 'GitHub')
 [void](Wait-UI 'https://github.com/OUBIGFA/OneSub')
 $shell=(Wait-UI $text.content).Current.BoundingRectangle
 $close=(Wait-UI $text.close).Current.BoundingRectangle
 $automatic=(Wait-UI $text.automatic ([System.Windows.Automation.TogglePattern]::Pattern)).GetCurrentPattern([System.Windows.Automation.TogglePattern]::Pattern)
 if($automatic.Current.ToggleState -ne [System.Windows.Automation.ToggleState]::On){throw 'Automatic updates did not default on'}
 if(-not (Wait-UI $text.check).Current.IsEnabled){throw 'The packaged application cannot check for updates'}
 Capture 'about-default.png'
 Click-UI $text.automatic
 $saved=[IO.File]::ReadAllText($preferences)|ConvertFrom-Json
 if($saved.autoUpdate -ne $false){throw 'The explicit automatic-update opt-out was not saved'}
 if((Wait-UI $text.content).Current.BoundingRectangle -ne $shell -or (Wait-UI $text.close).Current.BoundingRectangle -ne $close){throw 'Changing automatic updates resized the settings dialog'}
 if($CheckFeed){
  Click-UI $text.check
  for($i=0;$i -lt 550;$i++){
   if(@(Find-All $text.missing).Count -gt 0){break}
   Start-Sleep -Milliseconds 100
  }
  if(@(Find-All $text.missing).Count -eq 0){throw 'The live GitHub check did not report the missing release/manifest; inspect the native status and its tooltip'}
  if((Wait-UI $text.content).Current.BoundingRectangle -ne $shell -or (Wait-UI $text.close).Current.BoundingRectangle -ne $close){throw 'Checking updates moved the dialog or close action'}
  Capture 'about-live-check.png'
 }
 Click-UI $text.close
 Close-TestWindow
 $p=Start-TestWindow
 Click-UI $text.settings
 Click-UI $text.about
 $automatic=(Wait-UI $text.automatic ([System.Windows.Automation.TogglePattern]::Pattern)).GetCurrentPattern([System.Windows.Automation.TogglePattern]::Pattern)
 if($automatic.Current.ToggleState -ne [System.Windows.Automation.ToggleState]::Off){throw 'Restart lost the explicit automatic-update opt-out'}
 Capture 'about-restarted.png'
 Click-UI $text.close
 Close-TestWindow
 foreach($id in $existing){if(-not (Get-Process -Id $id -ErrorAction SilentlyContinue)){throw 'A pre-existing OneSub process was affected'}}
 Write-Output 'PASS: packaged native About, GitHub icon/link, default automatic updates, persistent opt-out across restart, fixed dialog and existing user instance preserved'
 if($CheckFeed){Write-Output 'PASS: live GitHub feed reported no published update package, not a false up-to-date result'}
} catch {
 $failure=$_
 if($p -and -not $p.HasExited){
  try {Capture 'failure.png'} catch {Write-Warning ("本次测试截图失败："+$_.Exception.Message)}
 }
 throw $failure
} finally {
 # This process was created with isolated data, IPC and WebView state.
 # A teardown failure cannot authorize closing the user's original app.
 if($p -and -not $p.HasExited){
  [void][AboutTestWindow]::PostMessage(([AboutTestWindow]::TestWindow($p.Id)),16,[IntPtr]::Zero,[IntPtr]::Zero)
  if(-not $p.WaitForExit(10000)){
   if([IO.Path]::GetFullPath($p.Path) -ne $exePath){throw 'Test process identity changed'}
   $p.Kill();$p.WaitForExit()
  }
 }
 foreach($name in $names){[Environment]::SetEnvironmentVariable($name,$previous[$name],'Process')}
}
