# Shared private-desktop driver functions and test-owned process come from
# native_smoke.ps1. Do not enumerate or send input to the user's desktop.
$draft=(Input-UI '字幕文字').GetCurrentPattern([System.Windows.Automation.ValuePattern]::Pattern).Current.Value
$action=(Find-UI '长度参考线').Current.BoundingRectangle
$table=(Find-UI '字幕列表').Current.BoundingRectangle
$review=(Find-UI '字幕列表操作').Current.BoundingRectangle
$null=Read-AgentSnapshot 'length-guide-before'
# The actions menu has the toolbar's localized name, while the toolbar group
# itself is flattened by UIA. Its right edge is the same as the native table.
if([Math]::Abs($action.Width-$action.Height) -gt 1 -or [Math]::Abs($table.Right-$action.Right) -gt $action.Width/2 -or [Math]::Abs($action.Y-$review.Y) -gt 1){throw "Length guide icon is not at the second toolbar row right edge: $action / $table / $review"}
if(Find-UI '字幕长度参考线'){throw 'Length guide is visible before activation'}
Click-AgentControl '长度参考线'
$line=(Find-UI '字幕长度参考线').Current.BoundingRectangle
if($line.Top -lt $table.Top -or $line.Bottom -gt $table.Bottom+1){throw 'Length guide extends outside the subtitle list'}
if(-not [SmokeNative]::DragReference(([SmokeNative]::TestWindow($p.Id)),$p.Id,[int]($line.X+$line.Width/2),[int]($line.Y+$line.Height/2),50)){throw 'Private reference drag failed'}
for($i=0;$i -lt 100;$i++){$moved=(Find-UI '字幕长度参考线').Current.BoundingRectangle;if([Math]::Abs($moved.X-$line.X-50) -lt 2){break};Start-Sleep -Milliseconds 100}
if([Math]::Abs($moved.X-$line.X-50) -gt 2){$null=Read-AgentSnapshot 'length-guide-drag-failed';throw "Reference did not follow the horizontal drag: $line -> $moved"}
if((Input-UI '字幕文字').GetCurrentPattern([System.Windows.Automation.ValuePattern]::Pattern).Current.Value -ne $draft){throw 'Reference drag selected a different subtitle'}
if(-not [SmokeNative]::Wheel(([SmokeNative]::TestWindow($p.Id)),$p.Id,[int]($table.X+$table.Width/2),[int]($table.Y+$table.Height/2),-1200)){throw 'Private list scroll failed'}
Start-Sleep -Milliseconds 400
if((Find-UI '字幕长度参考线').Current.BoundingRectangle -ne $moved){throw 'Reference moved with the subtitle rows'}
if((Find-UI '字幕列表').Current.BoundingRectangle -ne $table){throw 'Reference resized the subtitle table'}
& $ScreenshotTool $p.Id (Join-Path $workPath 'subtitle-length-guide.png')
$null=Read-AgentSnapshot 'length-guide-scrolled'
Click-AgentControl '长度参考线'
if(Find-UI '字幕长度参考线'){throw 'Length guide did not hide'}
Click-AgentControl '长度参考线'
if((Find-UI '字幕长度参考线').Current.BoundingRectangle -ne $moved){throw 'Length guide lost its dragged position'}
Write-Output 'PASS: length guide icon at toolbar right; native horizontal drag; stable reference during list scroll; rows and selection unchanged; off/on retains position'
