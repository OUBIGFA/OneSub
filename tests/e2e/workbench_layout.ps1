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
# The style catalog loads independently of the subtitle editor. Exercise the
# finished inspector, rather than trying to resize its loading placeholder.
for($i=0;$i -lt 100 -and -not (Find-UI '字幕内容');$i++){Start-Sleep -Milliseconds 100}
if(-not (Find-UI '字幕内容')){throw 'The style inspector did not finish loading'}
# Verify the shortened selection label through the existing private driver,
# which resolves this test's HWND directly instead of MainWindowHandle.
if(-not (Find-UI '选择')){throw 'The subtitle selection entry is missing'}
Click-AgentControl '选择'
if(-not (Find-UI '重译 · 1 条')){throw 'Select did not check the current subtitle'}
Click-AgentControl '取消勾选'
if(-not (Find-UI '选择')){throw 'Clearing selection did not restore the Select entry'}
$fold=Find-UI '智能拆分功能'
# Windows UIA exposes a native button as one leaf, including its text and
# decorative icon. Production render tests check the icon and chevron bounds;
# this run checks the complete label, actual actions and captured native pixels.
if(-not $fold -or $fold.Current.BoundingRectangle.Width -le $fold.Current.BoundingRectangle.Height){throw 'Folded smart split tools must show the complete label'}
Capture-Navigation ('toolbar-folded-'+$Appearance+'.png')
if((Find-UI '智能拆分 · 0 条长字幕') -or (Find-UI '问题类型') -or (Find-UI '下一个')){throw 'Smart split tools are expanded by default'}
Click-AgentControl '智能拆分功能'
foreach($label in @('智能拆分 · 0 条长字幕','问题类型','上一个','下一个','收起智能拆分功能')){
    if(-not (Find-UI $label)){throw "Expanded toolbar is missing $label"}
}
Capture-Navigation ('toolbar-expanded-'+$Appearance+'.png')
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
Click-AgentControl '收起智能拆分功能'
if(-not (Find-UI '智能拆分功能') -or (Find-UI '问题类型')){throw 'Smart split toolbar cannot collapse again'}
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
$fold=Find-UI '智能拆分功能'
if(-not $fold -or $fold.Current.BoundingRectangle.Width -le $fold.Current.BoundingRectangle.Height){throw 'Narrow workbench removed the complete smart split label'}
Click-UI '字幕列表操作'
if(-not (Find-UI '查找替换')){throw 'Narrow edit menu lost find/replace'}
Capture-Navigation ('toolbar-narrow-'+$Appearance+'.png')
Write-Output 'PASS: smart split tools start folded with label and icon, expand with a category chevron and collapse again; both inspectors and primary toolbar labels resize correctly'
