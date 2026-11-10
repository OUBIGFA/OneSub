# Runs only through native_smoke.ps1 on its verified private desktop.
# Wheel messages target this test window; no global mouse or keyboard input.
function Visible-ScrollCue {
    $list=Find-UI '字幕列表'
    $bounds=$list.Current.BoundingRectangle
    foreach($node in $list.FindAll([System.Windows.Automation.TreeScope]::Descendants,[System.Windows.Automation.Condition]::TrueCondition)) {
        $name=$node.Current.Name
        $r=$node.Current.BoundingRectangle
        if($name -match '^Click subtitle \d{3}$' -and $r.Height -gt 0 -and $r.Top -ge $bounds.Top+30 -and $r.Bottom -le $bounds.Bottom) {
            return @{name=$name;y=$r.Y}
        }
    }
    throw 'No complete subtitle row is visible'
}
function Assert-ScrollCue($before) {
    $after=Visible-ScrollCue
    if($after.name -ne $before.name -or [Math]::Abs($after.y-$before.y) -gt 1) {
        throw "Manual viewport moved: $($before.name) / $($before.y) -> $($after.name) / $($after.y)"
    }
}
Set-PlaybackPosition 80.2
Click-UI '播放'
Wait-PlaybackStarted
Click-UI '回到当前字幕'
Start-Sleep -Milliseconds 500
$selected=(Input-UI '字幕文字').GetCurrentPattern([System.Windows.Automation.ValuePattern]::Pattern).Current.Value
Wait-VisibleSubtitle $selected
$initial=Visible-ScrollCue
$list=(Find-UI '字幕列表').Current.BoundingRectangle
$x=[int]($list.X+$list.Width/2);$y=[int]($list.Y+$list.Height/2)
$handle=[SmokeNative]::TestWindow($p.Id)
if(-not [SmokeNative]::Wheel($handle,$p.Id,$x,$y,1200)) { throw 'Wheel message was not delivered to the native surface' }
Start-Sleep -Milliseconds 700
$before=Visible-ScrollCue
if([int]$before.name.Substring(15) -ge [int]$initial.name.Substring(15)-5){throw 'Wheel did not move away from the current cue'}
$position=Read-PlaybackPosition
Start-Sleep -Seconds 6
Assert-ScrollCue $before
if((Read-PlaybackPosition) -lt $position+4){throw 'Playback did not advance while browsing'}
Click-UI '暂停'
Start-Sleep -Milliseconds 400
Click-UI '播放'
Wait-PlaybackStarted
Start-Sleep -Seconds 6
Assert-ScrollCue $before
Capture-Navigation 'manual-scroll-held.png'
Click-UI '回到当前字幕'
Start-Sleep -Milliseconds 500
$selected=(Input-UI '字幕文字').GetCurrentPattern([System.Windows.Automation.ValuePattern]::Pattern).Current.Value
Wait-VisibleSubtitle $selected
Start-Sleep -Seconds 6
$later=(Input-UI '字幕文字').GetCurrentPattern([System.Windows.Automation.ValuePattern]::Pattern).Current.Value
if($later -eq $selected){throw 'Current cue did not advance after locate'}
Wait-VisibleSubtitle $later
Capture-Navigation 'manual-scroll-resumed.png'
Click-UI '暂停'
Write-Output 'PASS: real playback and wheel; viewport held beyond timeout and pause/resume; only locate restores following across later cues'
