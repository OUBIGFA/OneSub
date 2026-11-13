# Runs only inside native_smoke's private desktop, using its test-owned HWND.
Add-Type @'
using System;
using System.Runtime.InteropServices;
public static class LayoutNative {
  [StructLayout(LayoutKind.Sequential)] struct Point { public int x,y; }
  public delegate bool Visitor(IntPtr h,IntPtr p);
  [DllImport("user32.dll")] static extern bool EnumChildWindows(IntPtr h,Visitor visit,IntPtr p);
  [DllImport("user32.dll")] static extern uint GetWindowThreadProcessId(IntPtr h,out uint pid);
  [DllImport("user32.dll",CharSet=CharSet.Unicode)] static extern int GetClassName(IntPtr h,System.Text.StringBuilder text,int size);
  [DllImport("user32.dll")] static extern bool ScreenToClient(IntPtr h,ref Point p);
  [DllImport("user32.dll")] static extern bool PostMessage(IntPtr h,uint msg,IntPtr w,IntPtr l);
  [DllImport("user32.dll")] public static extern bool SetWindowPos(IntPtr h,IntPtr after,int x,int y,int w,int height,uint flags);
  public static void Drag(IntPtr window,int pid,int x,int y,int targetY) {
    IntPtr surface=IntPtr.Zero;
    EnumChildWindows(window,(h,p)=>{
      uint owner;GetWindowThreadProcessId(h,out owner);
      var name=new System.Text.StringBuilder(64);GetClassName(h,name,64);
      if(owner==pid && name.ToString()=="MyGoSurface"){surface=h;return false;}
      return true;
    },IntPtr.Zero);
    if(surface==IntPtr.Zero)throw new Exception("Private surface missing");
    var from=new Point{x=x,y=y};var to=new Point{x=x,y=targetY};
    if(!ScreenToClient(surface,ref from)||!ScreenToClient(surface,ref to))throw new Exception("Cannot map private coordinates");
    Action<uint,int,Point> send=(m,b,p)=>{
      if(!PostMessage(surface,m,new IntPtr(b),new IntPtr((p.y<<16)|(p.x&0xffff))))throw new Exception("Private pointer delivery failed");
      System.Threading.Thread.Sleep(70);
    };
    send(512,0,from);send(513,1,from);
    send(512,1,to);
    send(514,0,to);
  }
}
'@
$window=[SmokeNative]::TestWindow($p.Id)
# Resize this test window without activation or changing the user's desktop.
if(-not [LayoutNative]::SetWindowPos($window,[IntPtr]::Zero,0,0,3600,1000,0x14)){throw 'Private resize failed'}
Start-Sleep -Milliseconds 700
if(Find-UI '暂停'){Click-UI '暂停'}
foreach($panel in @('字幕样式面板','字幕编辑面板')) {
    $before=(Find-UI $panel).Current.BoundingRectangle
    $x=[int]($before.X+$before.Width/2)
    [LayoutNative]::Drag($window,$p.Id,$x,[int]($before.Y-1),[int]($before.Bottom-4))
    Start-Sleep -Milliseconds 600
    $folded=(Find-UI $panel).Current.BoundingRectangle
    $title=$panel.Replace('面板','')
    $titleBox=(Find-UI $title).Current.BoundingRectangle
    if($folded.Height -gt $before.Height/2 -or $folded.Height -gt 2.6*$titleBox.Height){throw "Inspector did not fold to title: $panel $folded"}
    $content=if($panel -eq '字幕样式面板'){'字幕内容'}else{'字幕文字'}
    if(Find-UI $content){throw "Folded inspector still exposes content: $panel"}
    Capture-Navigation ($title+'-folded.png')
    [LayoutNative]::Drag($window,$p.Id,$x,[int]($folded.Y-1),[int]($before.Y-1))
    Start-Sleep -Milliseconds 600
    $restored=(Find-UI $panel).Current.BoundingRectangle
    if([Math]::Abs($restored.Height-$before.Height) -gt 3){throw "Inspector did not restore height: $panel"}
}
$locate=Find-UI '定位'
if(-not $locate){throw 'Locate button is missing from the wide toolbar'}
$wide=$locate.Current.BoundingRectangle
if($wide.Width -le 1.5*$wide.Height){throw 'Wide second row hid labels too soon'}
Capture-Navigation ('toolbar-wide-'+$Appearance+'.png')
# A narrower workbench spends whitespace while keeping the icon and label.
if(-not [LayoutNative]::SetWindowPos($window,[IntPtr]::Zero,0,0,3200,1000,0x14)){throw 'Private resize failed'}
Start-Sleep -Milliseconds 600
$tight=(Find-UI '定位').Current.BoundingRectangle
if($tight.Width -le 1.5*$tight.Height -or $tight.Width -ge $wide.Width){throw "Toolbar did not tighten before hiding labels: $wide -> $tight"}
$edit=(Find-UI '字幕列表操作').Current.BoundingRectangle
foreach($label in @('查找替换','智能拆分 · 0 条长字幕','检查结果')) {
    $rect=(Find-UI $label).Current.BoundingRectangle
    if([Math]::Abs($rect.Y+$rect.Height/2-$edit.Y-$edit.Height/2) -gt 2){throw "Second toolbar row wrapped: $label"}
}
Capture-Navigation ('toolbar-tight-'+$Appearance+'.png')
if(-not [LayoutNative]::SetWindowPos($window,[IntPtr]::Zero,0,0,1760,900,0x14)){throw 'Private resize failed'}
Start-Sleep -Milliseconds 600
foreach($label in @('定位','查找替换')) {
    $normal=(Find-UI $label).Current.BoundingRectangle
    if($normal.Width -le 1.5*$normal.Height){throw "Normal-width toolbar hid primary label: $label"}
}
Capture-Navigation ('toolbar-normal-'+$Appearance+'.png')
foreach($tab in @('文字','外观','布局')) {
    Select-Tab '样式属性' $tab
    Capture-Navigation ('style-'+$tab+'-'+$Appearance+'.png')
}
Select-Tab '识别对象' '文件队列'
Capture-Navigation ('recognition-queue-'+$Appearance+'.png')
Select-Tab '识别对象' '当前文件'
# At the minimum window the middle pane is only about 390 DIPs. The current
# toolbar uses square actions here so all controls stay on the same row.
if(-not [LayoutNative]::SetWindowPos($window,[IntPtr]::Zero,0,0,1360,900,0x14)){throw 'Private resize failed'}
Start-Sleep -Milliseconds 600
$locate=Find-UI '定位'
if(-not $locate -or -not $locate.Current.IsEnabled){throw 'Minimum-window toolbar lost its locate action'}
$narrow=$locate.Current.BoundingRectangle
if($narrow.Width -le 0 -or $narrow.Width -gt $narrow.Height+2){throw 'Minimum-window toolbar did not keep its locate action compact'}
Click-UI '字幕列表操作'
if(-not (Find-UI '查找替换')){throw 'Narrow edit menu lost find/replace'}
Capture-Navigation ('toolbar-narrow-'+$Appearance+'.png')
Write-Output 'PASS: both inspectors fold and restore; primary toolbar labels remain at normal width; style and recognition switches remain operable'
