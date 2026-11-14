# Dot-sourced by the private-desktop driver; only this test's process is used.
Click-UI '校对模式'
if(-not (Find-UI '校对字幕') -or (Find-UI '原始字幕') -or (Find-UI '优化字幕')){throw 'Proofreading tabs are incorrect'}
$translation=Join-Path $workPath 'translated.srt'
[IO.File]::WriteAllText($translation,"1`n00:00:00,100 --> 00:00:02,900`nTranslated subtitle`n",[Text.UTF8Encoding]::new($false))
Click-UI '导入字幕';Choose-Path $translation
for($i=0;$i -lt 100;$i++){if(Find-UI '选择字幕属性'){break};Start-Sleep -Milliseconds 100}
if(-not (Find-UI '选择字幕属性')){throw 'Import skipped language choice'}
Select-Tab '选择字幕属性' '译文'
Click-UI '导入字幕'
for($i=0;$i -lt 100;$i++){if(Find-UI 'Translated subtitle'){break};Start-Sleep -Milliseconds 100}
if(-not (Find-UI 'Translated subtitle') -or -not (Find-UI '测试字幕')){throw 'Both subtitle languages are not visible'}
Select-Value '原文'
(Input-UI '字幕文字').GetCurrentPattern([System.Windows.Automation.ValuePattern]::Pattern).SetValue('Corrected source')
Start-Sleep -Milliseconds 800
Select-Value '译文'
(Input-UI '字幕文字').GetCurrentPattern([System.Windows.Automation.ValuePattern]::Pattern).SetValue('Corrected translation')
Start-Sleep -Milliseconds 800
Click-UI '保存'
$sourceOutput=Join-Path $workPath 'reviewed-source.srt'
Choose-Path $sourceOutput
for($i=0;$i -lt 100;$i++){
    if((Test-Path $sourceOutput) -and [IO.File]::ReadAllText($translation).Contains('Corrected translation')){break}
    Start-Sleep -Milliseconds 100
}
if(-not (Test-Path $sourceOutput) -or -not [IO.File]::ReadAllText($sourceOutput).Contains('Corrected source')){throw 'Source save failed'}
if(-not [IO.File]::ReadAllText($translation).Contains('Corrected translation')){throw 'Translation save failed'}
& $ScreenshotTool $p.Id (Join-Path $workPath 'proofreading.png')
# A second imported source has no matching translation. Refuse both sample and
# full composition, and locate the actual paired row rather than its old number.
$longerSource=Join-Path $workPath 'two-source-cues.srt'
[IO.File]::WriteAllText($longerSource,"1`n00:00:00,100 --> 00:00:01,400`nCorrected source`n`n2`n00:00:01,500 --> 00:00:02,900`nSource without a translation`n",[Text.UTF8Encoding]::new($false))
Click-UI '导入字幕';Choose-Path $longerSource
for($i=0;$i -lt 100;$i++){if(Find-UI '选择字幕属性'){break};Start-Sleep -Milliseconds 100}
Select-Tab '选择字幕属性' '原文'
Click-UI '导入字幕'
for($i=0;$i -lt 100;$i++){if(Find-UI 'Source without a translation'){break};Start-Sleep -Milliseconds 100}
Click-AgentControl '合成视频'
foreach($label in @('导出 5 秒预览','开始合成')){
    $action=Find-UI $label
    if(-not $action -or $action.Current.IsEnabled){throw "Missing translation did not block composition: $label"}
}
$report=Read-AgentSnapshot 'compose-missing-translation'
if(-not ($report | ConvertTo-Json -Depth 40).Contains('#2')){throw 'Missing translation report omitted subtitle #2'}
Click-AgentControl '定位异常字幕'
if(-not (Find-UI 'Source without a translation')){throw 'Missing translation navigation lost the affected source'}
Click-UI '退出校对'
if(-not (Find-UI '原始字幕') -or -not (Find-UI '优化字幕')){throw 'Normal tabs did not return'}
Write-Output 'PASS: private proofreading import, bilingual edits and separate saves; missing translation blocks composition with subtitle #2 and correction navigation'
