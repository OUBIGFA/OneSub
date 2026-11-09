param([int]$ProcessId,[string]$Output)
$ErrorActionPreference='Stop'
# Print only the test-owned window. No activation, focus, cursor or input APIs.
Add-Type -AssemblyName System.Drawing
if(-not ('PrivateCapture' -as [type])) {
Add-Type @'
using System;
using System.Runtime.InteropServices;
public static class PrivateCapture {
  [StructLayout(LayoutKind.Sequential)] public struct Rect { public int left,top,right,bottom; }
  [DllImport("user32.dll")] public static extern bool GetWindowRect(IntPtr window,out Rect rect);
  [DllImport("user32.dll")] public static extern bool PrintWindow(IntPtr window,IntPtr dc,uint flags);
  [DllImport("user32.dll")] public static extern bool SetProcessDPIAware();
}
'@
}
[void][PrivateCapture]::SetProcessDPIAware()
$window=[SmokeNative]::TestWindow($ProcessId)
if($window -eq [IntPtr]::Zero){throw 'Test window is absent'}
$rect=New-Object PrivateCapture+Rect
if(-not [PrivateCapture]::GetWindowRect($window,[ref]$rect)){throw 'Cannot measure test window'}
$bitmap=New-Object Drawing.Bitmap ($rect.right-$rect.left),($rect.bottom-$rect.top)
$graphics=[Drawing.Graphics]::FromImage($bitmap)
$dc=$graphics.GetHdc()
try { if(-not [PrivateCapture]::PrintWindow($window,$dc,2)){throw 'Private window capture failed'} }
finally { $graphics.ReleaseHdc($dc);$graphics.Dispose() }
try { $bitmap.Save([IO.Path]::GetFullPath($Output),[Drawing.Imaging.ImageFormat]::Png) }
finally { $bitmap.Dispose() }
Write-Output "Captured private test window: $Output"
