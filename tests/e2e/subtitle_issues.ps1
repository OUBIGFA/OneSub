# Runs only inside the existing private desktop recovery fixture. The caption
# contains one unsupported character already saved before translation resumes.
function Toggle-IssueFilter {
    $snapshot=Read-AgentSnapshot 'before-issue-filter'
    $target=Find-AgentControl $snapshot '定位异常字幕'
    if(-not $target){throw 'Native issue toggle is missing'}
    $raw=(& agent-desktop toggle $target.ref_id) -join "`n"
    $result=$raw | ConvertFrom-Json
    if($LASTEXITCODE -ne 0 -or -not $result.ok){throw "Native issue toggle failed: $raw"}
    Write-Output ('Issue toggle: '+($result.data | ConvertTo-Json -Compress -Depth 6))
    $null=Read-AgentSnapshot 'after-issue-filter'
}
$before=[IO.File]::ReadAllBytes($translated)
Set-PlaybackPosition 0.6
Start-Sleep -Seconds 1
Preview-Probe 'missing'
for($i=0;$i -lt 100;$i++){
    if(Find-UI '1 条异常字幕 · 1 项'){break}
    Start-Sleep -Milliseconds 100
}
if(-not (Find-UI '1 条异常字幕 · 1 项')){throw 'The affected subtitle/issue counts are missing'}
if((Read-Preferences).subtitleOutput.mode -ne 'translation'){throw 'Resume lost the selected translation output'}
Toggle-IssueFilter
for($i=0;$i -lt 50;$i++){
    if(Find-UI '译文 · 异常字符：U+4E7FD'){break}
    Start-Sleep -Milliseconds 100
}
if(-not (Find-UI '译文 · 异常字符：U+4E7FD')){throw 'The selected subtitle has no precise character diagnosis'}
for($i=0;$i -lt 50 -and (Find-UI 'Good morning');$i++){Start-Sleep -Milliseconds 100}
if(Find-UI 'Good morning'){throw 'The issue filter still contains unaffected subtitle rows'}
& $ScreenshotTool $p.Id (Join-Path $workPath 'subtitle-issue-located.png') | Out-Null
$field=Input-UI '字幕文字'
$field.GetCurrentPattern([System.Windows.Automation.ValuePattern]::Pattern).SetValue('你好，世界')
Toggle-IssueFilter
for($i=0;$i -lt 100;$i++){
    if((Find-UI 'Good morning') -and -not (Find-UI '1 条异常字幕 · 1 项')){break}
    Start-Sleep -Milliseconds 100
}
Start-Sleep -Seconds 2
Preview-Probe 'corrected'
if(Find-UI '1 条异常字幕 · 1 项'){throw 'Correcting the caption left a stale issue count'}
if([Convert]::ToBase64String([IO.File]::ReadAllBytes($translated)) -ne [Convert]::ToBase64String($before)){throw 'Correcting the draft silently overwrote the saved translation'}
& $ScreenshotTool $p.Id (Join-Path $workPath 'subtitle-issue-corrected.png') | Out-Null
Write-Output 'PASS: resumed translation preview updates despite one missing glyph; counts, exact cue location, filter and editing work; saved subtitles remain untouched'
