param([string]$Exe, [string]$Work, [string]$ScreenshotTool, [ValidateSet('video-first','subtitle-first')][string]$Order='video-first', [switch]$Packaged)
$ErrorActionPreference='Stop'
Add-Type -AssemblyName UIAutomationClient
Add-Type -AssemblyName UIAutomationTypes
Add-Type -AssemblyName System.Windows.Forms
Add-Type -ReferencedAssemblies System.Windows.Forms,System.Drawing @'
using System;
using System.Collections.Specialized;
using System.Runtime.InteropServices;
using System.Windows.Forms;
public static class ComplementaryDrop {
 [StructLayout(LayoutKind.Sequential)] public struct Point {public int X,Y;}
 [DllImport("user32.dll")] public static extern bool SetProcessDPIAware();
 [DllImport("user32.dll")] public static extern bool SetWindowPos(IntPtr h,IntPtr after,int x,int y,int w,int height,uint flags);
 [DllImport("user32.dll")] static extern bool SetForegroundWindow(IntPtr h);
 [DllImport("user32.dll")] static extern bool SetCursorPos(int x,int y);
 [DllImport("user32.dll")] static extern bool GetCursorPos(out Point p);
 [DllImport("user32.dll")] static extern IntPtr WindowFromPoint(Point p);
 [DllImport("user32.dll")] static extern IntPtr GetAncestor(IntPtr h,uint flags);
 [DllImport("user32.dll")] static extern uint GetWindowThreadProcessId(IntPtr h,out uint pid);
 [DllImport("user32.dll")] static extern void mouse_event(uint flags,uint x,uint y,uint data,UIntPtr extra);
 // WinForms supplies real IDataObject/IDropSource and the OLE message loop.
 // The source is a disposable form, not Explorer or the user's clipboard.
 public static void Drop(IntPtr window,int pid,string path,int x,int y) {
  Exception failure=null;
  // Each OLE source owns a fresh STA message loop. Reusing the UIA caller's
  // WinForms loop leaves button/COM state from the preceding cancelled drag.
  var thread=new System.Threading.Thread(()=>{
   try {DropOnThread(window,pid,path,x,y);}catch(Exception ex){failure=ex;}
  });
  thread.SetApartmentState(System.Threading.ApartmentState.STA);
  thread.Start();thread.Join();
  if(failure!=null)throw failure;
 }
 static void DropOnThread(IntPtr window,int pid,string path,int x,int y) {
  Point cursor;GetCursorPos(out cursor);
  try {
   SetForegroundWindow(window);
   if(!SetCursorPos(x,y))throw new Exception("Desktop pointer unavailable");
   // WebView2 child HWNDs can have a renderer PID. The root owner, not
   // that embedded child process, must be this test's OneSub window.
   IntPtr root=GetAncestor(WindowFromPoint(new Point{X=x,Y=y}),2);
   uint owner;GetWindowThreadProcessId(root,out owner);
   if(root!=window||owner!=pid)throw new Exception("Drop point is outside the test window: root="+root+", owner="+owner);
   using(var form=new Form()) {
    form.Width=80;form.Height=60;form.ShowInTaskbar=false;
    form.StartPosition=FormStartPosition.Manual;form.Left=10;form.Top=10;
    Exception failure=null;
    var files=new StringCollection();files.Add(path);
    var data=new DataObject();data.SetFileDropList(files);
    using(var timer=new Timer{Interval=50})
    using(var guard=new Timer{Interval=5000}) {
     int ticks=0;
     timer.Tick+=(sender,args)=>{
      if(!SetCursorPos(x+(ticks%2),y)){failure=new Exception("Desktop pointer lost during drop");ticks=10;}
      if(++ticks>=10){mouse_event(4,0,0,0,UIntPtr.Zero);timer.Stop();}
     };
     guard.Tick+=(sender,args)=>{failure=new Exception("Native drag source did not complete");mouse_event(4,0,0,0,UIntPtr.Zero);form.Close();};
     // DoDragDrop must start from the real client MouseDown event. Starting
     // it before that queued event is consumed can see an unpressed button
     // and return None before the movement timer has run even once.
     form.MouseDown+=(sender,args)=>{
      if(args.Button!=MouseButtons.Left)return;
      try {
       timer.Start();
       var effect=form.DoDragDrop(data,DragDropEffects.Copy);
       if(effect!=DragDropEffects.Copy)failure=new Exception("Native OLE drop rejected: effect="+effect+", moves="+ticks);
      } catch(Exception ex){failure=ex;}
      finally{timer.Stop();guard.Stop();mouse_event(4,0,0,0,UIntPtr.Zero);form.Close();}
     };
     form.Shown+=(s,e)=>form.BeginInvoke(new Action(()=>{
      var start=form.PointToScreen(new System.Drawing.Point(10,10));
      guard.Start();
      if(!SetCursorPos(start.X,start.Y)){failure=new Exception("Drag source pointer unavailable");form.Close();return;}
      mouse_event(2,0,0,0,UIntPtr.Zero);
     }));
     Application.Run(form);
    }
    if(failure!=null)throw failure;
   }
  } finally {mouse_event(4,0,0,0,UIntPtr.Zero);SetCursorPos(cursor.X,cursor.Y);}
 }
}
'@
[void][ComplementaryDrop]::SetProcessDPIAware()
$workPath=[IO.Path]::GetFullPath($Work)
if(Test-Path $workPath){throw 'Use a fresh complementary-import directory'}
New-Item -ItemType Directory "$workPath/input","$workPath/data","$workPath/ipc" -Force | Out-Null
$env:ONESUB_DATA_ROOT="$workPath/data"
$env:XDG_RUNTIME_DIR="$workPath/ipc"
$env:WEBVIEW2_USER_DATA_FOLDER="$workPath/webview"
if($env:XDG_RUNTIME_DIR.Length -gt 65){throw 'Use a shorter isolated IPC path'}
[IO.File]::WriteAllText("$workPath/data/settings.json",'{"version":5,"uiLanguage":"zh-Hans","closeToTray":false,"appearance":"light","topHeight":55}',[Text.UTF8Encoding]::new($false))
if($Packaged){
 foreach($name in @('ONESUB_FFMPEG','ONESUB_FFPROBE','ONESUB_SUBTITLE_WORKER','ONESUB_STYLE_WORKER','ONESUB_SKILL_WORKER','ONESUB_SKILL_ROOT')){Remove-Item "Env:$name" -ErrorAction SilentlyContinue}
}
$video=Join-Path $workPath 'input/picture.mp4';$subtitle=Join-Path $workPath 'input/different.srt'
& ffmpeg -v error -f lavfi -i 'color=c=green:size=640x360:rate=30:duration=10' -c:v libx264 -pix_fmt yuv420p $video
if($LASTEXITCODE -ne 0){throw 'Video fixture creation failed'}
[IO.File]::WriteAllText($subtitle,"1`n00:00:00,100 --> 00:00:09,900`nSource subtitle`n",[Text.UTF8Encoding]::new($false))
$sourceHash=(Get-FileHash $subtitle -Algorithm SHA256).Hash
$first=if($Order -eq 'video-first'){$video}else{$subtitle}
$p=Start-Process -FilePath ([IO.Path]::GetFullPath($Exe)) -ArgumentList @('"'+$first+'"') -RedirectStandardError "$workPath/stderr.log" -PassThru
function Find-All($name) {
 $p.Refresh();if($p.HasExited -or $p.MainWindowHandle -eq 0){return @()}
 $root=[System.Windows.Automation.AutomationElement]::FromHandle($p.MainWindowHandle)
 $condition=[System.Windows.Automation.PropertyCondition]::new([System.Windows.Automation.AutomationElement]::NameProperty,$name)
 return $root.FindAll([System.Windows.Automation.TreeScope]::Descendants,$condition)
}
function Find-UI($name) {return (Find-All $name | Select-Object -First 1)}
function Wait-UI($name) {
 for($i=0;$i -lt 200;$i++) {
  try {$item=Find-UI $name;if($item){return $item}}catch [System.Windows.Automation.ElementNotAvailableException]{}
  Start-Sleep -Milliseconds 100
 }
 throw "Control unavailable: $name"
}
function Input-UI($name) {
 foreach($item in (Find-All $name)) {
  $pattern=$null
  if($item.TryGetCurrentPattern([System.Windows.Automation.ValuePattern]::Pattern,[ref]$pattern)){return $item}
 }
 throw "Input unavailable: $name"
}
function Input-Value($name) {return (Input-UI $name).GetCurrentPattern([System.Windows.Automation.ValuePattern]::Pattern).Current.Value}
function Click-UI($name) {
 for($i=0;$i -lt 50;$i++) {
  try {
   (Wait-UI $name).GetCurrentPattern([System.Windows.Automation.InvokePattern]::Pattern).Invoke()
   Start-Sleep -Milliseconds 200
   return
  } catch [System.Windows.Automation.ElementNotAvailableException] {
   # A committed edit or modal redraw invalidates the old UIA provider;
   # reacquire the current node, as the existing native smoke test does.
  }
  Start-Sleep -Milliseconds 100
 }
 throw "Invocation unavailable: $name"
}
function Current-Project {
 $folder=Get-ChildItem -LiteralPath "$workPath/data/projects" -Directory | Sort-Object CreationTimeUtc -Descending | Select-Object -First 1
 if($folder -and (Test-Path "$($folder.FullName)/project.json")) {
  return @{path=$folder.FullName;manifest=([IO.File]::ReadAllText("$($folder.FullName)/project.json")|ConvertFrom-Json)}
 }
 return $null
}
$dropCount=0
function Drop-File($file,$target) {
 # The screenshot helper activates this exact test-owned root using attached
 # input queues. Plain SetForegroundWindow can be denied by Windows' lock.
 $script:dropCount++
 if($ScreenshotTool){
  & $ScreenshotTool $p.Id (Join-Path $workPath ("before-drop-$script:dropCount.png")) -wait 300 | Out-Host
  if($LASTEXITCODE -ne 0){throw 'Test window capture/activation failed'}
 }
 $rect=(Wait-UI $target).Current.BoundingRectangle
 [ComplementaryDrop]::Drop($p.MainWindowHandle,$p.Id,$file,[int]($rect.X+$rect.Width/2),[int]($rect.Y+$rect.Height/2))
}
function Close-Test {
 $p.Refresh();if($p.HasExited){return}
 [void]$p.CloseMainWindow()
 for($i=0;$i -lt 150;$i++) {
  $p.Refresh();if($p.HasExited){return}
  $discard=Find-UI '不保存'
  if($discard){
   try {$discard.GetCurrentPattern([System.Windows.Automation.InvokePattern]::Pattern).Invoke()}
   catch [System.Windows.Automation.ElementNotAvailableException] {
    # The exit action destroys its UIA provider. Accept that race only when
    # this exact process has actually exited, not while a live dialog remains.
    if(-not $p.WaitForExit(2000)){throw}
   }
  }
  Start-Sleep -Milliseconds 100
 }
 # This process was launched above with isolated data and IPC. A teardown
 # timeout is a failed test, never permission to kill another OneSub instance.
 $p.Kill();$p.WaitForExit();throw 'Test-owned application did not close normally'
}
try {
 [void](Wait-UI '添加文件')
 if($Order -eq 'video-first') {
  [void](Wait-UI '00:00:10.000')
  Drop-File $subtitle '播放器画面区域'
  [void](Wait-UI 'Source subtitle')
 } else {
  [void](Wait-UI 'Source subtitle')
  (Input-UI '字幕文字').GetCurrentPattern([System.Windows.Automation.ValuePattern]::Pattern).SetValue('Reviewed source')
  [void](Wait-UI 'Reviewed source');[void](Wait-UI '未保存')
  $before=Current-Project
  Drop-File $video '字幕列表'
  [void](Wait-UI '是否保存字幕修改？');Click-UI '取消'
  $cancelled=Current-Project
  if($cancelled.path -ne $before.path -or $cancelled.manifest.mediaPath -ne '' -or (Input-Value '字幕文字') -ne 'Reviewed source'){throw 'Cancelling the drop changed the subtitle project'}
  Drop-File $video '字幕列表'
  [void](Wait-UI '是否保存字幕修改？');Click-UI '不保存'
  [void](Wait-UI '00:00:10.000')
 }
 $project=Current-Project
 if($project.manifest.mediaPath -ne $video -or $project.manifest.subtitlePath -ne $subtitle){throw 'Complementary files were not associated'}
 if($Order -eq 'subtitle-first' -and ($project.path -ne $before.path -or $project.manifest.id -ne $before.manifest.id -or (Input-Value '字幕文字') -ne 'Reviewed source' -or -not (Find-UI '撤销').Current.IsEnabled)){throw 'Media association discarded edits or undo history'}
 # Check real playback, not only a loaded filename or the UI's paused flag.
 Click-UI '播放';[void](Wait-UI '暂停');Start-Sleep -Milliseconds 600
 if((Find-UI '播放位置').GetCurrentPattern([System.Windows.Automation.RangeValuePattern]::Pattern).Current.Value -le 0){throw 'Associated video did not advance'}
 Click-UI '暂停';[void](Wait-UI '播放')
 $sizes=@()
 foreach($size in @(@(1800,1050),@(1800,720),@(1200,780),@(1800,1100))) {
  [void][ComplementaryDrop]::SetWindowPos($p.MainWindowHandle,[IntPtr]::Zero,0,0,$size[0],$size[1],0x14)
  Start-Sleep -Milliseconds 350
  $panel=(Wait-UI '字幕编辑面板').Current.BoundingRectangle
  $text=(Input-UI '字幕文字').Current.BoundingRectangle
  if($text.Height -le 0 -or $text.Bottom -gt $panel.Bottom -or $panel.Bottom-$text.Bottom -gt 20){throw 'Editor text does not fill the current panel'}
  for($i=0;$i -lt 6;$i++) {
   Start-Sleep -Milliseconds 40
   if((Input-UI '字幕文字').Current.BoundingRectangle -ne $text){throw 'Editor text continued resizing after layout settled'}
  }
  $sizes+=@{width=$size[0];height=$size[1];textHeight=$text.Height}
 }
 foreach($step in @(@{button='增加开始时间';field='开始';value='00:00:00.101'},@{button='减少开始时间';field='开始';value='00:00:00.100'},@{button='增加结束时间';field='结束';value='00:00:09.901'},@{button='减少结束时间';field='结束';value='00:00:09.900'})) {
  Click-UI $step.button
  if((Input-Value $step.field) -ne $step.value){throw "Time step failed: $($step.button)"}
 }
 $start=(Input-UI '开始').Current.BoundingRectangle;$end=(Input-UI '结束').Current.BoundingRectangle
 if([Math]::Abs($start.Y-$end.Y) -gt 1){throw 'Time inputs are not on the same row in a wide editor'}
 if((Get-FileHash $subtitle -Algorithm SHA256).Hash -ne $sourceHash){throw 'Import or live edits overwrote the source subtitle'}
 if($ScreenshotTool){
  $header=(Wait-UI '字幕样式面板').Current.BoundingRectangle
  & $ScreenshotTool $p.Id "$workPath/editor.png" -mouse ("{0},{1}" -f [int]($header.X+$header.Width/2),[int]($header.Y+12)) -wait 800 | Out-Host
  if($LASTEXITCODE -ne 0){throw 'Editor screenshot failed'}
 }
 Close-Test
 $durable=[IO.File]::ReadAllText("$($project.path)/project.json")|ConvertFrom-Json
 if($durable.mediaPath -ne $video -or $durable.subtitlePath -ne $subtitle){throw 'Media association was not persisted'}
 $result=@{order=$Order;oleDrops=if($Order -eq 'video-first'){1}else{2};unsavedCancel=($Order -eq 'subtitle-first');sourceUnchanged=$true;timeSteps=4;editorSizes=$sizes;associationPersisted=$true}
 [IO.File]::WriteAllText("$workPath/result.json",($result|ConvertTo-Json -Depth 4),[Text.UTF8Encoding]::new($false))
 Write-Output ($result|ConvertTo-Json -Depth 4)
} finally {Close-Test}
