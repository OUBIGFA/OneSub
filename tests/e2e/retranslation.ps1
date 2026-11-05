param([string]$Exe,[string]$Work,[string]$ScreenshotTool,[string]$Endpoint,[switch]$Packaged)
$ErrorActionPreference='Stop'
Add-Type -AssemblyName UIAutomationClient
Add-Type -AssemblyName UIAutomationTypes
$workPath=[IO.Path]::GetFullPath($Work)
if(Test-Path $workPath){throw 'Use a fresh test directory'}
New-Item -ItemType Directory $workPath | Out-Null
$source=Join-Path $workPath 'sample.srt'
$body="1`n00:00:00,000 --> 00:00:04,000`nFirst source.`n`n2`n00:00:05,000 --> 00:00:09,000`nSecond source.`n`n3`n00:00:10,000 --> 00:00:14,000`nThird source.`n"
[IO.File]::WriteAllText($source,$body,[Text.UTF8Encoding]::new($false))
$initialHash=(Get-FileHash $source).Hash
$env:ONESUB_DATA_ROOT=Join-Path $workPath 'data'
$env:XDG_RUNTIME_DIR=Join-Path $workPath 'ipc'
$env:WEBVIEW2_USER_DATA_FOLDER=Join-Path $workPath 'webview'
if($env:XDG_RUNTIME_DIR.Length -gt 65){throw 'Use a shorter isolated IPC path'}
New-Item -ItemType Directory $env:ONESUB_DATA_ROOT,$env:XDG_RUNTIME_DIR | Out-Null
$prefs=@{version=5;uiLanguage='zh-Hans';appearance='light';closeToTray=$false;providerUrl=$Endpoint;providerModel='local-retranslation-test';optimizeTimeline=$false;stripPunctuation=$false;replacePeriods=$false}
[IO.File]::WriteAllText((Join-Path $env:ONESUB_DATA_ROOT 'settings.json'),($prefs|ConvertTo-Json),[Text.UTF8Encoding]::new($false))
$root=(Get-Location).Path
$overrides=@{ONESUB_SUBTITLE_WORKER='build/output/subtitle-core/OneSub.SubtitleCore.exe';ONESUB_STYLE_WORKER='build/output/style-core/OneSub.StyleCore.exe';ONESUB_SKILL_WORKER='build/output/skill-tools/OneSub.SkillTools.exe';ONESUB_SKILL_ROOT='third_party/skills/srt-subtitle-translator'}
foreach($name in $overrides.Keys){if($Packaged){Remove-Item "Env:$name" -ErrorAction SilentlyContinue}else{Set-Item "Env:$name" (Join-Path $root $overrides[$name])}}
$p=Start-Process -FilePath ([IO.Path]::GetFullPath($Exe)) -ArgumentList ('"'+$source+'"') -PassThru -RedirectStandardError (Join-Path $workPath 'stderr.log')
function Root {$p.Refresh();if($p.HasExited -or $p.MainWindowHandle -eq 0){return $null};return [System.Windows.Automation.AutomationElement]::FromHandle($p.MainWindowHandle)}
function Items($name){$r=Root;if(-not $r){return @()};return $r.FindAll([System.Windows.Automation.TreeScope]::Descendants,[System.Windows.Automation.PropertyCondition]::new([System.Windows.Automation.AutomationElement]::NameProperty,$name))}
function Find-UI($name){foreach($item in (Items $name)){return $item};return $null}
function Wait-UI($name){for($i=0;$i -lt 150;$i++){$item=Find-UI $name;if($item){return $item};Start-Sleep -Milliseconds 100};throw "Missing control: $name"}
function Click-UI($name){
 for($i=0;$i -lt 100;$i++){
  foreach($item in (Items $name)){
   if(-not $item.Current.IsEnabled){continue};$pattern=$null
   if($item.TryGetCurrentPattern([System.Windows.Automation.InvokePattern]::Pattern,[ref]$pattern)){$pattern.Invoke();Start-Sleep -Milliseconds 300;return}
   if($item.TryGetCurrentPattern([System.Windows.Automation.TogglePattern]::Pattern,[ref]$pattern)){$pattern.Toggle();Start-Sleep -Milliseconds 300;return}
   if($item.TryGetCurrentPattern([System.Windows.Automation.SelectionItemPattern]::Pattern,[ref]$pattern)){$pattern.Select();Start-Sleep -Milliseconds 300;return}
  };Start-Sleep -Milliseconds 100
 };throw "Action unavailable: $name"
}
function Read-Text($name){
 [void](Wait-UI $name)
 foreach($item in (Items $name)){$pattern=$null;if($item.TryGetCurrentPattern([System.Windows.Automation.ValuePattern]::Pattern,[ref]$pattern)){return $pattern.Current.Value}}
 throw "Input unavailable: $name"
}
function Wait-Text($name,$value){for($i=0;$i -lt 150;$i++){if((Read-Text $name) -eq $value){return};Start-Sleep -Milliseconds 100};throw "Unexpected text in $name"}
function Capture($name){& $ScreenshotTool $p.Id (Join-Path $workPath $name) | Out-Host;if($LASTEXITCODE -ne 0){throw 'Screenshot failed'}}
function Cue-Texts($path){return @([regex]::Matches([IO.File]::ReadAllText($path),'(?m)^\d+\r?\n[^\r\n]+ --> [^\r\n]+\r?\n([^\r\n]+)')|ForEach-Object{$_.Groups[1].Value})}
try {
 [void](Wait-UI '选择字幕')
 if(Find-UI '勾选字幕 #1'){throw 'Unchecked list shows checkboxes'}
 Click-UI '全选';[void](Wait-UI '重译 · 3 条')
 Click-UI '取消勾选';[void](Wait-UI '选择字幕')
 if(Find-UI '勾选字幕 #1'){throw 'Clear did not hide checkboxes'}
 Click-UI 'AI 翻译';Click-UI '开始翻译字幕'
 $translated=Join-Path $workPath 'sample-zh.srt'
 for($i=0;$i -lt 300;$i++){if(Test-Path $translated){break};Start-Sleep -Milliseconds 100}
 if(-not (Test-Path $translated)){throw 'Baseline translation not published'}
 [void](Wait-UI '原来的第一条译文')
 $savedHash=(Get-FileHash $translated).Hash
 Click-UI '选择字幕';Click-UI '勾选字幕 #3'
 [void](Wait-UI '重译 · 2 条')
 Capture 'selected.png'
 Click-UI '重译 · 2 条'
 if((Read-Text '原译文') -ne '原来的第一条译文'){throw 'Wrong initial comparison'}
 Click-UI '翻译全部';Click-UI '关闭';Click-UI '查看重译结果'
 Wait-Text '新译文' '新的第一条译文'
 Capture 'results.png'
 Click-UI '重译此条';Wait-Text '新译文' '再次翻译的第一条译文'
 Click-UI '全部再译'
 for($i=0;$i -lt 150;$i++){if((Read-Text '检查结果与状态') -match '401'){break};Start-Sleep -Milliseconds 100}
 if((Read-Text '检查结果与状态') -notmatch '401' -or (Read-Text '新译文') -ne '再次翻译的第一条译文'){throw 'Failure lost previous result'}
 if((Get-FileHash $translated).Hash -ne $savedHash){throw 'Generating results saved the subtitle file'}
 Click-UI '关闭';Click-UI '取消勾选'
 if(Find-UI '勾选字幕 #1'){throw 'Clear after review did not hide checkboxes'}
 Click-UI '查看重译结果'
 Wait-Text '新译文' '再次翻译的第一条译文'
 Click-UI '替换此条';[void](Wait-UI '已替换')
 if((Read-Text '原译文') -ne '原来的第一条译文'){throw 'Accept erased before/after comparison'}
 Capture 'applied.png'
 Click-UI '关闭'
 if((Get-FileHash $translated).Hash -ne $savedHash){throw 'Acceptance wrote a file without Save'}
 Click-UI '保存'
 $texts=Cue-Texts $translated
 if($texts[0] -ne '再次翻译的第一条译文' -or $texts[1] -ne '原来的第二条译文' -or $texts[2] -ne '原来的第三条译文'){throw 'Replace this touched other subtitles'}
 Click-UI '撤销';Click-UI '保存'
 if((Cue-Texts $translated)[0] -ne '原来的第一条译文'){throw 'Undo failed'}
 Click-UI '查看重译结果'
 if((Read-Text '新译文') -ne '再次翻译的第一条译文' -or (Read-Text '检查结果与状态') -notmatch '字幕已修改'){throw 'Stale result not retained'}
 foreach($name in @('替换此条','替换全部','重译此条','全部再译')){if((Wait-UI $name).Current.IsEnabled){throw "Stale action enabled: $name"}}
 Capture 'stale.png'
 if((Get-FileHash $source).Hash -ne $initialHash){throw 'Original source file changed'}
 Click-UI '关闭'
 Write-Output 'PASS: native retranslation interaction and file checks'
} catch {
 if(Root){Capture 'failure.png';(Root).FindAll([System.Windows.Automation.TreeScope]::Descendants,[System.Windows.Automation.Condition]::TrueCondition)|ForEach-Object{Write-Output $_.Current.Name}}
 throw
} finally {
 # Only this isolated test-owned application is stopped.
 if(-not $p.HasExited){Stop-Process -Id $p.Id -Force}
}
