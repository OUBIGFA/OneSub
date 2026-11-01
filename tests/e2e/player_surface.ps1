param([string]$Exe, [string]$Work, [string]$ScreenshotTool, [switch]$Baseline, [int]$Width=1800, [int]$Height=1050, [ValidateSet('dark','light')][string]$Appearance='dark', [string]$LayoutFile, [string]$WindowStateFile, [int]$DragDelay=15, [string]$MediaFile, [switch]$CaptureScreen, [switch]$Play)
$ErrorActionPreference='Stop'
if($Play -and -not $MediaFile){throw 'Playback sampling requires the solid-color test video'}
Add-Type -AssemblyName UIAutomationClient
Add-Type -AssemblyName UIAutomationTypes
Add-Type -AssemblyName System.Drawing
Add-Type -ReferencedAssemblies System.Drawing @'
using System;
using System.Drawing;
using System.Drawing.Imaging;
using System.Runtime.InteropServices;
public static class SurfaceProbe {
 [StructLayout(LayoutKind.Sequential)] public struct R {public int L,T,Rt,B;}
 [StructLayout(LayoutKind.Sequential)] public struct P {public int X,Y;}
 public delegate bool Visit(IntPtr h,IntPtr p);
 [DllImport("user32.dll")] public static extern bool EnumChildWindows(IntPtr h,Visit v,IntPtr p);
 [DllImport("user32.dll",CharSet=CharSet.Unicode)] public static extern int GetClassName(IntPtr h,System.Text.StringBuilder b,int n);
 [DllImport("user32.dll")] public static extern bool GetWindowRect(IntPtr h,out R r);
 [DllImport("user32.dll")] public static extern bool ScreenToClient(IntPtr h,ref P p);
 [DllImport("user32.dll")] public static extern bool SetWindowPos(IntPtr h,IntPtr after,int x,int y,int w,int height,uint flags);
 [DllImport("user32.dll")] public static extern IntPtr SendMessage(IntPtr h,uint m,IntPtr w,IntPtr l);
 [DllImport("user32.dll")] public static extern bool PrintWindow(IntPtr h,IntPtr dc,uint flags);
 [DllImport("user32.dll")] public static extern bool SetProcessDPIAware();
 [DllImport("user32.dll")] public static extern bool SetForegroundWindow(IntPtr h);
 [DllImport("user32.dll")] public static extern IntPtr GetForegroundWindow();
 [DllImport("user32.dll")] public static extern bool GetCursorPos(out P p);
 [DllImport("user32.dll")] public static extern bool SetCursorPos(int x,int y);
 [DllImport("user32.dll")] public static extern void mouse_event(uint flags,uint dx,uint dy,uint data,UIntPtr extra);
 public static IntPtr Canvas(IntPtr window) {
  IntPtr found=IntPtr.Zero;
  EnumChildWindows(window,(h,p)=>{var b=new System.Text.StringBuilder(128);GetClassName(h,b,128);if(b.ToString()=="MyGoSurface"){found=h;return false;}return true;},IntPtr.Zero);
  return found;
 }
 public static void Pointer(IntPtr canvas,uint message,int x,int y,bool down) {
  // Physical pointer state must match the test's messages; otherwise the
  // next native leave event can cancel an artificial multi-frame drag.
  if(!SetCursorPos(x,y))throw new Exception("Desktop pointer unavailable");
  if(message==513)mouse_event(2,0,0,0,UIntPtr.Zero);
  if(message==514)mouse_event(4,0,0,0,UIntPtr.Zero);
 }
 // This captures pixels only; it never draws controls into the application.
 static bool MediaPixel(int pixel) {
  int r=(pixel>>16)&255,g=(pixel>>8)&255,b=pixel&255;
  return r<=8&&b<=8&&(g<=8||g>=110);
 }
 public static int Check(IntPtr window,int x,int y,int w,int h,int bx,int by,int bw,int bh,bool video,bool screen,string file) {
  R frame;GetWindowRect(window,out frame);
  using(var image=new Bitmap(frame.Rt-frame.L,frame.B-frame.T,PixelFormat.Format32bppArgb)) {
   using(var graphics=Graphics.FromImage(image)) {
    if(screen) {
     if(GetForegroundWindow()!=window)throw new Exception("Screen capture lost the target window");
     graphics.CopyFromScreen(frame.L,frame.T,0,0,image.Size,CopyPixelOperation.SourceCopy);
    } else {
     var dc=graphics.GetHdc();try{if(!PrintWindow(window,dc,2))throw new Exception("PrintWindow failed");}finally{graphics.ReleaseHdc(dc);}
    }
   }
   int bad=0,green=0;
   var data=image.LockBits(new Rectangle(0,0,image.Width,image.Height),ImageLockMode.ReadOnly,PixelFormat.Format32bppArgb);
   try {
    var row=new int[image.Width];
    int left=x-frame.L,top=y-frame.T;
    // UI Automation can describe the next layout while DWM still captures
    // the previous one. Use this captured picture's perimeter; no settling
    // delay masks a fast-drag defect.
    Marshal.Copy(IntPtr.Add(data.Scan0,(top+2)*data.Stride),row,0,row.Length);
    int right=left+4;
    while(right<image.Width-8&&MediaPixel(row[right]))right++;
    int bottom=top;
    for(int yy=top+4;yy<image.Height-40;yy++) {
     int pixel=Marshal.ReadInt32(IntPtr.Add(data.Scan0,yy*data.Stride+(left+4)*4));
     if(MediaPixel(pixel))bottom=yy+1;
    }
    if(right-left<100||bottom-top<100)throw new Exception("Captured media perimeter missing");
    bx=left+(right-left-bw)/2+frame.L;by=top+(bottom-top-bh)/2+frame.T;
    for(int yy=top+4;yy<bottom-4;yy+=2) {
     Marshal.Copy(IntPtr.Add(data.Scan0,yy*data.Stride),row,0,row.Length);
     for(int xx=left+4;xx<right-4;xx+=2) {
      if(!video&&xx+frame.L>=bx-12&&xx+frame.L<=bx+bw+12&&yy+frame.T>=by-12&&yy+frame.T<=by+bh+12)continue;
      int pixel=row[xx];if(!MediaPixel(pixel))bad++;
      if(((pixel>>8)&255)>=110&&((pixel>>16)&255)<=8&&(pixel&255)<=8)green++;
     }
    }
   } finally {image.UnlockBits(data);}
   if(video&&green<100)throw new Exception("Solid-color test video is not displayed");
   if(bad>0||file.EndsWith("final.png"))image.Save(file,ImageFormat.Png);
   return bad;
  }
 }
}
'@
[void][SurfaceProbe]::SetProcessDPIAware()
$workPath=[IO.Path]::GetFullPath($Work)
if(Test-Path $workPath){throw 'Use a fresh test directory'}
New-Item -ItemType Directory "$workPath/data","$workPath/ipc" -Force | Out-Null
$env:ONESUB_DATA_ROOT="$workPath/data"
$env:XDG_RUNTIME_DIR="$workPath/ipc"
$env:WEBVIEW2_USER_DATA_FOLDER="$workPath/webview"
if($env:XDG_RUNTIME_DIR.Length -gt 65){throw 'Use a shorter isolated IPC path'}
[IO.File]::WriteAllText("$workPath/data/settings.json",('{"version":5,"uiLanguage":"zh-Hans","closeToTray":false,"appearance":"'+$Appearance+'"}'),[Text.UTF8Encoding]::new($false))
if($LayoutFile) {
 $layout=[IO.File]::ReadAllText([IO.Path]::GetFullPath($LayoutFile),[Text.Encoding]::UTF8)|ConvertFrom-Json
 $layout.uiLanguage='zh-Hans';$layout.closeToTray=$false;$layout.appearance=$Appearance
 [IO.File]::WriteAllText("$workPath/data/settings.json",($layout|ConvertTo-Json),[Text.UTF8Encoding]::new($false))
}
if($WindowStateFile) {Copy-Item -LiteralPath ([IO.Path]::GetFullPath($WindowStateFile)) -Destination "$workPath/data/window-state.json"}
$launch=@{FilePath=[IO.Path]::GetFullPath($Exe);PassThru=$true;RedirectStandardError="$workPath/stderr.log"}
if($MediaFile){$launch.ArgumentList=@('"'+[IO.Path]::GetFullPath($MediaFile)+'"')}
$p=Start-Process @launch
function Find-UI($name) {
 $root=[System.Windows.Automation.AutomationElement]::FromHandle($p.MainWindowHandle)
 $condition=[System.Windows.Automation.PropertyCondition]::new([System.Windows.Automation.AutomationElement]::NameProperty,$name)
 return $root.FindFirst([System.Windows.Automation.TreeScope]::Descendants,$condition)
}
function Check-Frame($name) {
 $area=(Find-UI '播放器画面区域').Current.BoundingRectangle
 $button=[System.Windows.Rect]::new(0,0,0,0)
 if(-not $MediaFile){$button=(Find-UI '添加视频').Current.BoundingRectangle}
 return [SurfaceProbe]::Check($p.MainWindowHandle,$area.X,$area.Y,$area.Width,$area.Height,$button.X,$button.Y,$button.Width,$button.Height,[bool]$MediaFile,[bool]$CaptureScreen,(Join-Path $workPath $name))
}
$cursor=[SurfaceProbe+P]::new()
[void][SurfaceProbe]::GetCursorPos([ref]$cursor)
try {
 for($i=0;$i -lt 150;$i++){$p.Refresh();if($p.HasExited){throw 'Test process exited'};if($p.MainWindowHandle -ne 0 -and (Find-UI '添加文件') -and (-not $MediaFile -or (Find-UI '00:00:10.000'))){break};Start-Sleep -Milliseconds 100}
 [void][SurfaceProbe]::SetWindowPos($p.MainWindowHandle,[IntPtr]::Zero,0,0,$Width,$Height,0x14)
 Start-Sleep -Seconds 2
 if($ScreenshotTool){& $ScreenshotTool $p.Id (Join-Path $workPath 'start.png') -wait 300 | Out-Host}
 [void][SurfaceProbe]::SetForegroundWindow($p.MainWindowHandle)
 if([SurfaceProbe]::GetForegroundWindow() -ne $p.MainWindowHandle){throw 'Test window cannot own pointer input'}
 $canvas=[SurfaceProbe]::Canvas($p.MainWindowHandle)
 if($canvas -eq [IntPtr]::Zero){throw 'Native canvas missing'}
 if($Play) {
  (Find-UI '播放').GetCurrentPattern([System.Windows.Automation.InvokePattern]::Pattern).Invoke()
  for($i=0;$i -lt 60;$i++) {
   $position=(Find-UI '播放位置').GetCurrentPattern([System.Windows.Automation.RangeValuePattern]::Pattern).Current.Value
   if((Find-UI '暂停') -and $position -gt 0){break}
   Start-Sleep -Milliseconds 100
  }
  if(-not (Find-UI '暂停') -or $position -le 0){throw 'Test video did not advance during playback'}
 }
 $bad=@();$heights=@();$widths=@()
 $initial=Check-Frame 'initial.png'
 if($initial -gt 0){$bad+=@{frame='initial';pixels=$initial}}
 $style=(Find-UI '字幕样式面板').Current.BoundingRectangle
 $x=[int]($style.X+120);$y=[int]($style.Y-1)
 [SurfaceProbe]::Pointer($canvas,513,$x,$y,$true)
 for($i=0;$i -lt 24;$i++) {
  $dy=@(-140,100,-80,130)[$i%4]
  [SurfaceProbe]::Pointer($canvas,512,$x,($y+$dy),$true)
  if($DragDelay -gt 0){Start-Sleep -Milliseconds $DragDelay}
  $heights+=(Find-UI '播放器画面区域').Current.BoundingRectangle.Height
  $count=Check-Frame ("drag-{0:00}.png" -f $i)
  if($count -gt 0){$bad+=@{frame=$i;pixels=$count}}
 }
 [SurfaceProbe]::Pointer($canvas,514,$x,$y,$false)
 Start-Sleep -Milliseconds 100
 $player=(Find-UI '视频播放区').Current.BoundingRectangle
 $x=[int]($player.Right+1);$y=[int]($player.Top+120)
 [SurfaceProbe]::Pointer($canvas,513,$x,$y,$true)
 for($i=0;$i -lt 24;$i++) {
  $dx=@(-380,180,-250,250)[$i%4]
  [SurfaceProbe]::Pointer($canvas,512,($x+$dx),$y,$true)
  if($DragDelay -gt 0){Start-Sleep -Milliseconds ([Math]::Min(5,$DragDelay))}
  $widths+=(Find-UI '播放器画面区域').Current.BoundingRectangle.Width
  $count=Check-Frame ("horizontal-{0:00}.png" -f $i)
  if($count -gt 0){$bad+=@{frame="horizontal-$i";pixels=$count}}
 }
 [SurfaceProbe]::Pointer($canvas,514,$x,$y,$false)
 Start-Sleep -Milliseconds 400
 $last=Check-Frame 'final.png'
 if($last -gt 0){$bad+=@{frame='final';pixels=$last}}
 $heightRange=($heights|Measure-Object -Maximum -Minimum)
 $widthRange=($widths|Measure-Object -Maximum -Minimum)
 if($heightRange.Maximum-$heightRange.Minimum -lt 100 -or $widthRange.Maximum-$widthRange.Minimum -lt 100){throw 'Native splitter drags did not resize the player'}
 $result=@{frames=50;invalid=$bad.Count;details=$bad;appearance=$Appearance;playing=[bool]$Play;capture=@('PrintWindow','Screen')[[int][bool]$CaptureScreen];heightRange=@($heightRange.Minimum,$heightRange.Maximum);widthRange=@($widthRange.Minimum,$widthRange.Maximum)}
 [IO.File]::WriteAllText("$workPath/result.json",($result|ConvertTo-Json -Depth 4),[Text.UTF8Encoding]::new($false))
 Write-Output ($result|ConvertTo-Json -Depth 4)
 if($bad.Count -gt 0 -and -not $Baseline){throw 'Non-media UI pixels appeared in the player surface'}
} finally {
 [SurfaceProbe]::mouse_event(4,0,0,0,[UIntPtr]::Zero)
 [void][SurfaceProbe]::SetCursorPos($cursor.X,$cursor.Y)
 $p.Refresh()
 if(-not $p.HasExited){[void]$p.CloseMainWindow();if(-not $p.WaitForExit(10000)){$p.Kill();$p.WaitForExit()}}
}
