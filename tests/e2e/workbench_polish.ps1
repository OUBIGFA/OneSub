param([string]$Exe, [string]$Work, [string]$ScreenshotTool, [switch]$Packaged)
$ErrorActionPreference='Stop'
Add-Type -AssemblyName UIAutomationClient
Add-Type -AssemblyName UIAutomationTypes
Add-Type -AssemblyName System.Windows.Forms
Add-Type @'
using System;
using System.Runtime.InteropServices;
public static class PolishNative {
 [StructLayout(LayoutKind.Sequential)] public struct Rect { public int L,T,R,B; }
 [DllImport("user32.dll")] public static extern bool GetWindowRect(IntPtr h,out Rect r);
 [DllImport("user32.dll")] public static extern bool ShowWindow(IntPtr h,int n);
 [DllImport("user32.dll")] public static extern bool IsWindowVisible(IntPtr h);
 [DllImport("user32.dll")] public static extern bool IsIconic(IntPtr h);
 [DllImport("user32.dll", CharSet=CharSet.Unicode)] public static extern IntPtr SendMessage(IntPtr h,uint m,IntPtr w,string text);
 [DllImport("user32.dll", EntryPoint="SendMessageW")] public static extern IntPtr SendButton(IntPtr h,uint m,IntPtr w,IntPtr l);
 [DllImport("user32.dll")] public static extern bool SetForegroundWindow(IntPtr h);
}
'@
$workPath=[IO.Path]::GetFullPath($Work)
if(Test-Path $workPath){throw 'Use a fresh test directory'}
if(Get-Process OneSub -ErrorAction SilentlyContinue){throw 'Close existing OneSub before running the isolated desktop test'}
New-Item -ItemType Directory $workPath | Out-Null
$video=Join-Path $workPath 'sample.original.mp4'
$srt=Join-Path $workPath 'sample.original.srt'
$body=''
for($i=0;$i -lt 30;$i++){$body+="$($i+1)`n00:00:$('{0:00}' -f $i),000 --> 00:00:$('{0:00}' -f ($i+1)),000`n字幕 $($i+1)`n`n"}
[IO.File]::WriteAllText($srt,$body,[Text.UTF8Encoding]::new($false))
& ffmpeg -v error -f lavfi -i 'testsrc2=size=1280x720:rate=24:duration=30' -c:v libx264 -preset ultrafast -pix_fmt yuv420p $video
if($LASTEXITCODE -ne 0){throw 'Fixture failed'}
$env:ONESUB_DATA_ROOT=Join-Path $workPath 'data'
New-Item -ItemType Directory $env:ONESUB_DATA_ROOT | Out-Null
# Controls are found by their Chinese names; pin the interface language.
[IO.File]::WriteAllText((Join-Path $env:ONESUB_DATA_ROOT 'settings.json'),'{"version":1,"closeToTray":true,"uiLanguage":"zh-Hans"}',[Text.UTF8Encoding]::new($false))
$env:ONESUB_FFMPEG=(Get-Command ffmpeg).Source
$env:ONESUB_FFPROBE=(Get-Command ffprobe).Source
if($Packaged){foreach($name in @('ONESUB_FFPROBE','ONESUB_FFMPEG','ONESUB_SUBTITLE_WORKER','ONESUB_STYLE_WORKER','ONESUB_SKILL_WORKER','ONESUB_SKILL_ROOT')){Remove-Item "Env:$name" -ErrorAction SilentlyContinue}}
$exePath=[IO.Path]::GetFullPath($Exe)
$p=Start-Process -FilePath $exePath -ArgumentList ('"'+$video+'"') -PassThru -RedirectStandardError (Join-Path $workPath 'stderr.log')
$hwnd=[IntPtr]::Zero
function Find-UI($name){
 $root=[System.Windows.Automation.AutomationElement]::FromHandle($hwnd)
 if(-not $root){return $null}
 $condition=[System.Windows.Automation.PropertyCondition]::new([System.Windows.Automation.AutomationElement]::NameProperty,$name)
 return $root.FindFirst([System.Windows.Automation.TreeScope]::Descendants,$condition)
}
function Wait-UI($name){for($i=0;$i -lt 200;$i++){$item=Find-UI $name;if($item){return $item};Start-Sleep -Milliseconds 100};throw "Control not found: $name"}
function Click-UI($name){$item=Wait-UI $name;$item.GetCurrentPattern([System.Windows.Automation.InvokePattern]::Pattern).Invoke();Start-Sleep -Milliseconds 200}
function Select-Tab($name){$item=Wait-UI $name;$item.GetCurrentPattern([System.Windows.Automation.SelectionItemPattern]::Pattern).Select();Start-Sleep -Milliseconds 200}
function Input-UI($name){
 [void](Wait-UI $name)
 $root=[System.Windows.Automation.AutomationElement]::FromHandle($hwnd)
 $condition=[System.Windows.Automation.PropertyCondition]::new([System.Windows.Automation.AutomationElement]::NameProperty,$name)
 foreach($item in $root.FindAll([System.Windows.Automation.TreeScope]::Descendants,$condition)){
  $pattern=$null
  if($item.TryGetCurrentPattern([System.Windows.Automation.ValuePattern]::Pattern,[ref]$pattern)){return $item}
 }
 throw "Editable input missing: $name"
}
function Set-Text($name,$value){$item=Input-UI $name;$item.GetCurrentPattern([System.Windows.Automation.ValuePattern]::Pattern).SetValue($value);Start-Sleep -Milliseconds 200}
function Rect { $r=[PolishNative+Rect]::new();[void][PolishNative]::GetWindowRect($hwnd,[ref]$r);return @($r.L,$r.T,$r.R,$r.B) }
function Dialog {
 $pidCondition=[System.Windows.Automation.PropertyCondition]::new([System.Windows.Automation.AutomationElement]::ProcessIdProperty,$p.Id)
 $classCondition=[System.Windows.Automation.PropertyCondition]::new([System.Windows.Automation.AutomationElement]::ClassNameProperty,'#32770')
 $condition=[System.Windows.Automation.AndCondition]::new($pidCondition,$classCondition)
 return [System.Windows.Automation.AutomationElement]::RootElement.FindFirst([System.Windows.Automation.TreeScope]::Descendants,$condition)
}
function Wait-Dialog {for($i=0;$i -lt 100;$i++){$d=Dialog;if($d){Start-Sleep -Milliseconds 500;return $d};Start-Sleep -Milliseconds 100};
 & $ScreenshotTool $p.Id (Join-Path $workPath 'dialog-failure.png') | Out-Host
 $root=[System.Windows.Automation.AutomationElement]::FromHandle($hwnd)
 $root.FindAll([System.Windows.Automation.TreeScope]::Descendants,[System.Windows.Automation.Condition]::TrueCondition)|ForEach-Object{Write-Host $_.Current.Name}
 throw 'No file dialog'}
function Set-DialogPath($path){
 $d=Wait-Dialog
 $all=$d.FindAll([System.Windows.Automation.TreeScope]::Descendants,[System.Windows.Automation.Condition]::TrueCondition)
 $edit=$null
 foreach($item in $all){if($item.Current.AutomationId -in @('1148','1001') -and $item.Current.ClassName -eq 'Edit'){$edit=$item;break}}
 if(-not $edit){throw 'Filename input missing'}
 if($edit.Current.Name -notlike '*-OneSub.mp4'){throw 'Default composition filename is incorrect'}
 [void][PolishNative]::SendMessage([IntPtr]$edit.Current.NativeWindowHandle,12,[IntPtr]::Zero,$path)
 # Native common-dialog controls can be exposed as UIA Pane on Windows 11.
 # Use their exact HWND/control ID, never a desktop coordinate or address bar.
 $button=$all|Where-Object{$_.Current.AutomationId -eq '1' -and $_.Current.NativeWindowHandle -ne 0}|Select-Object -First 1
 [void][PolishNative]::SendButton([IntPtr]$button.Current.NativeWindowHandle,245,[IntPtr]::Zero,[IntPtr]::Zero)
}
try {
 for($i=0;$i -lt 100;$i++){$p.Refresh();if($p.MainWindowHandle -ne 0){$hwnd=$p.MainWindowHandle;break};Start-Sleep -Milliseconds 100}
 [void](Wait-UI '字幕文字')
 Start-Sleep -Seconds 2
 # The second process must exit and reactivate the same HWND, including tray.
 foreach($mode in @('visible','minimized','tray')){
  if($mode -eq 'minimized'){[void][PolishNative]::ShowWindow($hwnd,6)}
  if($mode -eq 'tray'){[void]$p.CloseMainWindow();Start-Sleep -Milliseconds 400;if([PolishNative]::IsWindowVisible($hwnd)){throw 'Close to tray failed'}}
  $second=Start-Process -FilePath $exePath -PassThru
  if(-not $second.WaitForExit(10000)){throw 'Second launch stayed alive'}
  Start-Sleep -Milliseconds 400
  if(-not [PolishNative]::IsWindowVisible($hwnd) -or [PolishNative]::IsIconic($hwnd)){throw "Activation failed: $mode"}
 }
 # Edits go live into the list; the list's Save replaces the original file.
 Set-Text '字幕文字' '原文修改'
 [void](Wait-UI '未保存')
 Click-UI '保存'
 for($i=0;$i -lt 100;$i++){if([IO.File]::ReadAllText($srt) -match '原文修改'){break};Start-Sleep -Milliseconds 100}
 if([IO.File]::ReadAllText($srt) -notmatch '原文修改'){throw 'Save did not replace the original subtitle'}
 # Source and translation are edited independently.
 Select-Tab '重建字幕'
 Set-Text '字幕文字' 'Translated text'
 Start-Sleep -Milliseconds 1000
 Select-Tab '原始字幕'
 if((Input-UI '字幕文字').GetCurrentPattern([System.Windows.Automation.ValuePattern]::Pattern).Current.Value -ne '原文修改'){throw 'Source and translation were mixed'}
 # Clearing with unsaved changes asks first; Cancel keeps everything.
 Set-Text '字幕文字' 'Unsaved change'
 [void](Wait-UI '未保存')
 Click-UI '清空工作台'
 [void](Wait-UI '清空工作台？')
 Click-UI '取消'
 if((Input-UI '字幕文字').GetCurrentPattern([System.Windows.Automation.ValuePattern]::Pattern).Current.Value -ne 'Unsaved change'){throw 'Cancel lost the edit'}
 # Confirm every settings tab keeps exactly the same content rectangle.
 Click-UI '设置';$initial=(Wait-UI '设置内容').Current.BoundingRectangle
 foreach($tab in @('识别与模型','翻译','常规')){Select-Tab $tab;$next=(Wait-UI '设置内容').Current.BoundingRectangle;if($next.Height -ne $initial.Height -or $next.Top -ne $initial.Top){throw "Settings changed size: $tab"}}
 Click-UI '关闭'
 # Compare internal panes as well as outer bounds when dialog/task status changes.
 $before=Rect
 $paneBefore=(Wait-UI '视频播放区').Current.BoundingRectangle
 Click-UI '添加文件';$d=Wait-Dialog
 $paneDuring=(Wait-UI '视频播放区').Current.BoundingRectangle
 if($paneBefore -ne $paneDuring){throw "File dialog changed internal layout: $paneBefore -> $paneDuring"}
 $during=Rect
 if(($before -join ',') -ne ($during -join ',')){throw "File dialog resized window: $before -> $during"}
 $all=$d.FindAll([System.Windows.Automation.TreeScope]::Descendants,[System.Windows.Automation.Condition]::TrueCondition)
 $cancelButton=$all|Where-Object{$_.Current.AutomationId -eq '2' -and $_.Current.NativeWindowHandle -ne 0}|Select-Object -First 1
 if(-not $cancelButton){throw 'Cancel button missing'}
 [void][PolishNative]::SendButton([IntPtr]$cancelButton.Current.NativeWindowHandle,245,[IntPtr]::Zero,[IntPtr]::Zero)
 Start-Sleep -Milliseconds 500
 if(($before -join ',') -ne ((Rect) -join ',')){throw 'File dialog changed size on close'}
 if((Wait-UI '视频播放区').Current.BoundingRectangle -ne $paneBefore){throw 'Internal layout changed after dialog cancellation'}
 # Compose using the real save dialog and observe progress inside the player.
 Click-UI '合成视频';Click-UI '开始合成'
 $output=Join-Path $workPath 'sample.original-OneSub.mp4'
 Set-DialogPath $output
 [void](Wait-UI '视频合成进度')
 & $ScreenshotTool $p.Id (Join-Path $workPath 'compose-progress.png')
 for($i=0;$i -lt 600;$i++){if(Test-Path -LiteralPath $output){break};Start-Sleep -Milliseconds 200}
 if(-not(Test-Path -LiteralPath $output)){throw 'Composition did not publish verified output'}
 if(Test-Path -LiteralPath ($output+'.onesub.json')){throw 'Composition left a sidecar beside the video'}
 $reports=@(Get-ChildItem (Join-Path $env:ONESUB_DATA_ROOT 'projects') -Filter 'compose-*.json' -Recurse)
 if($reports.Count -ne 1){throw 'Validation report missing from recovery project'}
 Start-Sleep -Seconds 1
 & $ScreenshotTool $p.Id (Join-Path $workPath 'workbench.png')
 Write-Output 'PASS: single instance visible/minimized/tray; live edit and list save; independent source/translation; unsaved prompt on clearing; equal settings; file dialog bounds; real composition/player progress'
} finally {
 # Only this test-owned process is terminated; no user instance is touched.
 $p.Refresh();if(-not $p.HasExited){$p.Kill();$p.WaitForExit()}
}
