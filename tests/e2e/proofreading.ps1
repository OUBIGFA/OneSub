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
Click-UI '退出校对'
if(-not (Find-UI '原始字幕') -or -not (Find-UI '优化字幕')){throw 'Normal tabs did not return'}
Write-Output 'PASS: private desktop proofreading toggle, role choice, bilingual edit, separate saves and exit'
