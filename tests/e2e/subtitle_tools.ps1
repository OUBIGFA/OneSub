param([string]$Exe,[string]$Work,[string]$ScreenshotTool,[string]$Endpoint,[string]$NativeHelpersPath,[ValidateSet('dark','light')][string]$Appearance='dark')
$ErrorActionPreference='Stop'
Add-Type -AssemblyName UIAutomationClient
Add-Type -AssemblyName UIAutomationTypes
# Reuse the existing native pointer/menu driver. It checks the owning process
# and MyGo surface and activates a real menu item, not a synthesized command.
$nativeSource=[IO.File]::ReadAllText($NativeHelpersPath)
$nativeMatch=[regex]::Match($nativeSource,"(?s)Add-Type @'\r?\n(.*?)\r?\n'@")
if(-not $nativeMatch.Success){throw 'Native test driver declaration not found'}
Add-Type $nativeMatch.Groups[1].Value
$workPath=[IO.Path]::GetFullPath($Work)
if(Test-Path $workPath){throw 'Use a fresh test directory'}
New-Item -ItemType Directory $workPath | Out-Null
$source=Join-Path $workPath 'sample.srt'
$first='First step explains the complete workflow. Next step checks every subtitle carefully, then saves the final result.'
$second='这是显示时间不足的长字幕，拆分不能增加阅读时间，必须继续保留问题提示。'
$third='Check final inputs. Save every result.'
$body="1`n00:00:00,000 --> 00:00:16,000`n$first`n`n2`n00:00:17,000 --> 00:00:18,000`n$second`n`n3`n00:00:19,000 --> 00:00:23,000`n$third`n"
[IO.File]::WriteAllText($source,$body,[Text.UTF8Encoding]::new($false))
$initialHash=(Get-FileHash $source).Hash
$env:ONESUB_DATA_ROOT=Join-Path $workPath 'data'
$env:XDG_RUNTIME_DIR=Join-Path $workPath 'ipc'
$env:WEBVIEW2_USER_DATA_FOLDER=Join-Path $workPath 'webview'
if($env:XDG_RUNTIME_DIR.Length -gt 65){throw 'Use a shorter isolated IPC path'}
New-Item -ItemType Directory $env:ONESUB_DATA_ROOT,$env:XDG_RUNTIME_DIR | Out-Null
$prefs=@{version=5;uiLanguage='zh-Hans';appearance=$Appearance;closeToTray=$false;providerUrl=$Endpoint;providerModel='local-subtitle-tools';optimizeTimeline=$false;stripPunctuation=$false;replacePeriods=$false}
[IO.File]::WriteAllText((Join-Path $env:ONESUB_DATA_ROOT 'settings.json'),($prefs|ConvertTo-Json),[Text.UTF8Encoding]::new($false))
foreach($name in @('ONESUB_FFPROBE','ONESUB_FFMPEG','ONESUB_SUBTITLE_WORKER','ONESUB_STYLE_WORKER','ONESUB_SKILL_WORKER','ONESUB_SKILL_ROOT')){Remove-Item "Env:$name" -ErrorAction SilentlyContinue}
$p=Start-Process -FilePath ([IO.Path]::GetFullPath($Exe)) -ArgumentList ('"'+$source+'"') -PassThru -RedirectStandardError (Join-Path $workPath 'stderr.log')
function Root {$p.Refresh();if($p.HasExited -or $p.MainWindowHandle -eq 0){return $null};return [System.Windows.Automation.AutomationElement]::FromHandle($p.MainWindowHandle)}
function Find-UI($name){$root=Root;if(-not $root){return $null};return $root.FindFirst([System.Windows.Automation.TreeScope]::Descendants,[System.Windows.Automation.PropertyCondition]::new([System.Windows.Automation.AutomationElement]::NameProperty,$name))}
function Wait-UI($name){for($i=0;$i -lt 150;$i++){$item=Find-UI $name;if($item){return $item};Start-Sleep -Milliseconds 100};throw "Missing control: $name"}
function Click-UI($name){
 for($i=0;$i -lt 100;$i++){
  $root=Root
  if($root){foreach($item in $root.FindAll([System.Windows.Automation.TreeScope]::Descendants,[System.Windows.Automation.PropertyCondition]::new([System.Windows.Automation.AutomationElement]::NameProperty,$name))){
   if(-not $item.Current.IsEnabled){continue};$pattern=$null
   if($item.TryGetCurrentPattern([System.Windows.Automation.InvokePattern]::Pattern,[ref]$pattern)){$pattern.Invoke();Start-Sleep -Milliseconds 300;return}
   if($item.TryGetCurrentPattern([System.Windows.Automation.ExpandCollapsePattern]::Pattern,[ref]$pattern)){$pattern.Expand();Start-Sleep -Milliseconds 300;return}
   if($item.TryGetCurrentPattern([System.Windows.Automation.SelectionItemPattern]::Pattern,[ref]$pattern)){$pattern.Select();Start-Sleep -Milliseconds 300;return}
  }};Start-Sleep -Milliseconds 100
 };throw "Action unavailable: $name"
}
function Set-Text($name,$value){
 [void](Wait-UI $name)
 foreach($item in (Root).FindAll([System.Windows.Automation.TreeScope]::Descendants,[System.Windows.Automation.PropertyCondition]::new([System.Windows.Automation.AutomationElement]::NameProperty,$name))){
  $pattern=$null;if($item.TryGetCurrentPattern([System.Windows.Automation.ValuePattern]::Pattern,[ref]$pattern)){$pattern.SetValue($value);Start-Sleep -Milliseconds 200;return}
 };throw "Input unavailable: $name"
}
function Read-Text($name){
 [void](Wait-UI $name)
 foreach($item in (Root).FindAll([System.Windows.Automation.TreeScope]::Descendants,[System.Windows.Automation.PropertyCondition]::new([System.Windows.Automation.AutomationElement]::NameProperty,$name))){
  $pattern=$null;if($item.TryGetCurrentPattern([System.Windows.Automation.ValuePattern]::Pattern,[ref]$pattern)){return $pattern.Current.Value}
 };throw "Input unavailable: $name"
}
function Cue-Texts($path){return @([regex]::Matches([IO.File]::ReadAllText($path),'(?m)^\d+\r?\n[^\r\n]+ --> [^\r\n]+\r?\n([^\r\n]+)')|ForEach-Object{$_.Groups[1].Value})}
function Save-Subtitles {Click-UI '保存';Start-Sleep -Milliseconds 500}
function Undo-Save {Click-UI '撤销';Save-Subtitles}
function Assert-Sequence($path){$raw=[IO.File]::ReadAllText($path);$numbers=@([regex]::Matches($raw,'(?m)^\d+$')|ForEach-Object{[int]$_.Value});for($i=0;$i -lt $numbers.Count;$i++){if($numbers[$i] -ne $i+1){throw 'Non-contiguous numbering'}};return $numbers.Count}
function Split-One($text) {
 $row=Wait-UI $text;$r=$row.Current.BoundingRectangle
 if(-not [SmokeNative]::RightClick($p.MainWindowHandle,$p.Id,[int]($r.X+$r.Width/2),[int]($r.Y+$r.Height/2))){throw 'Test-owned row did not receive right click'}
 for($i=0;$i -lt 100;$i++){if([SmokeNative]::MenuItems($p.Id)){break};Start-Sleep -Milliseconds 100}
 if(-not [SmokeNative]::ChooseMenuItem($p.MainWindowHandle,$p.Id,'智能拆分')){throw ('Single split menu activation failed: '+[SmokeNative]::MenuActivationError)}
 Start-Sleep -Milliseconds 500
}
function Check-CompactCue($previousText,$text,$path,$name) {
 # Edit/save a real fixture, then use the real single-cue menu. A no-op must
 # preserve bytes and must not insert an undo step before the text edit.
 Set-Text '搜索' ''
 # Undo leaves this fixture's final cue selected. Verify that state instead
 # of invoking the row's non-actionable text leaf or clicking it again.
 if((Read-Text '字幕文字') -ne $previousText){throw 'Compact fixture row is not selected'}
 Set-Text '字幕文字' $text
 Save-Subtitles
 $savedHash=(Get-FileHash $path).Hash
 Split-One $text
 if((Read-Text '字幕文字') -ne $text){throw 'Compact cue text was split in the workbench'}
 [void](Wait-UI '状态：字幕无需拆分，已保留原条目。')
 if((Assert-Sequence $path) -ne 3 -or (Get-FileHash $path).Hash -ne $savedHash){throw 'Compact cue was fragmented or written by the split command'}
 if((Cue-Texts $path)[-1] -ne $text){throw 'Compact cue text or punctuation changed'}
 Capture $name
 Undo-Save
 if((Cue-Texts $path)[-1] -ne $previousText){throw 'No-op split inserted an undo step'}
}
function View-Long($count) {
 Click-UI '查看问题'
 for($i=0;$i -lt 100;$i++){if([SmokeNative]::MenuItems($p.Id)){break};Start-Sleep -Milliseconds 100}
 if(-not [SmokeNative]::ChooseMenuItem($p.MainWindowHandle,$p.Id,"查看过长字幕 · $count 条")){throw ('Reading category menu activation failed: '+[SmokeNative]::MenuActivationError)}
 Start-Sleep -Milliseconds 300
}
function Capture($name) {
 # Clear the UIA activation's synthetic hover/focus on a non-interactive
 # heading before taking a static image; never click an underlying modal.
 $label=if(Find-UI '替换为'){'查找替换'}else{'字幕编辑'}
 $root=Root
 if($root){
  $condition=[System.Windows.Automation.PropertyCondition]::new([System.Windows.Automation.AutomationElement]::NameProperty,$label)
  foreach($item in $root.FindAll([System.Windows.Automation.TreeScope]::Descendants,$condition)){
   if($item.Current.ControlType -ne [System.Windows.Automation.ControlType]::Text -or $item.Current.IsOffscreen){continue}
   $r=$item.Current.BoundingRectangle
   [void][SmokeNative]::PointerClick($p.MainWindowHandle,$p.Id,[int]($r.X+$r.Width/2),[int]($r.Y+$r.Height/2),$false)
   Start-Sleep -Milliseconds 400;break
  }
 }
 & $ScreenshotTool $p.Id (Join-Path $workPath $name) | Out-Host
 if($LASTEXITCODE -ne 0){throw 'Screenshot failed'}
}
try {
 [void](Wait-UI '智能拆分 · 2 条长字幕')
 $savedFace=(Wait-UI '保存').Current.BoundingRectangle
 $search=Find-UI '搜索';$replace=Find-UI '查找替换';$locate=Find-UI '回到当前字幕';$periods=Find-UI '句号转逗号';$trailing=Find-UI '移除句末标点'
 $sr=$search.Current.BoundingRectangle;$rr=$replace.Current.BoundingRectangle;$lr=$locate.Current.BoundingRectangle;$pr=$periods.Current.BoundingRectangle;$tr=$trailing.Current.BoundingRectangle
 if($rr.Left -lt $tr.Right -or $rr.Top -ge $tr.Bottom -or $pr.Left -le $lr.Right -or $tr.Left -lt $pr.Right){throw 'Subtitle toolbar grouping/order incorrect'}
 if($sr.Top -lt $rr.Bottom -and $sr.Bottom -gt $rr.Top -and $sr.Left -lt $rr.Right){throw 'Search does not follow the editing tools'}
 Capture 'before.png'
 Set-Text '搜索' 'Check final'
 View-Long 2
 [void](Wait-UI '字幕：过长 · 过快')
 Capture 'selected-issue.png'
 Click-UI '智能拆分 · 2 条长字幕'
 [void](Wait-UI '智能拆分 · 1 条长字幕')
 if((Get-FileHash $source).Hash -ne $initialHash){throw 'Split wrote the source before Save'}
 $save=Find-UI '保存'
 if(-not $save -or -not $save.Current.IsEnabled){throw 'Unsaved edits did not enable Save'}
 Capture 'split-source.png'
 # MyGo exposes the labeled button as one accessibility leaf. Inspect the
 # actual rendering inside that action rather than requiring a separate node.
 Add-Type -AssemblyName System.Drawing
 $saveBounds=$save.Current.BoundingRectangle;$rootBounds=(Root).Current.BoundingRectangle
 if([Math]::Abs($saveBounds.Width-$savedFace.Width) -gt 1){throw 'Save and Unsaved use different button widths'}
 $image=[Drawing.Bitmap]::new((Join-Path $workPath 'split-source.png'))
 try {
  $red=0
  for($y=[Math]::Max(0,[int]($saveBounds.Top-$rootBounds.Top-12));$y -lt [Math]::Min($image.Height,[int]($saveBounds.Bottom-$rootBounds.Top+12));$y++){
   for($x=[Math]::Max(0,[int]($saveBounds.Left-$rootBounds.Left-12));$x -lt [Math]::Min($image.Width,[int]($saveBounds.Right-$rootBounds.Left+12));$x++){
    $pixel=$image.GetPixel($x,$y);if($pixel.R -gt 130 -and $pixel.R -gt 1.4*$pixel.G -and $pixel.R -gt 1.4*$pixel.B){$red++}
   }
  }
  if($red -lt 10){throw 'Unsaved save action is not red in the desktop rendering'}
 } finally {$image.Dispose()}
 Save-Subtitles
 $splitTexts=Cue-Texts $source
 foreach($text in $splitTexts[0..($splitTexts.Count-3)]){if($text -match '[.。．]|[,，]$'){throw 'Smart split did not remove final punctuation before converting internal periods'}}
 if((Assert-Sequence $source) -le 3){throw 'Source was not split'}
 if(-not ([IO.File]::ReadAllText($source).Contains('00:00:16,000'))){throw 'Original end changed'}
 if((Cue-Texts $source)[-2] -ne $second){throw 'Unsafe short cue was modified'}
 Undo-Save
 if((Cue-Texts $source).Count -ne 3){throw 'Single undo did not restore source'}
 Split-One $third
 Save-Subtitles
 if((Assert-Sequence $source) -ne 4 -or (Cue-Texts $source)[0] -ne $first){throw 'Single split did not target only the readable cue'}
 Capture 'single-source.png'
 Undo-Save
 foreach($case in @(
  @{text='为什么公司几乎立即就道歉？';name='compact-question-source.png'},
  @{text='你能不能具体讲讲发生了什么？';name='compact-predicate-source.png'},
  @{text='Kylian Mbappé 是个极具统治力的球员';name='compact-name-source.png'}
 )){Check-CompactCue $third $case.text $source $case.name}
 # One occurrence, then whole-track replacement despite an active filter.
 foreach($all in @($false,$true)){
  Set-Text '搜索' 'Check final'
  Click-UI '查找替换';Set-Text '查找内容' 'step';Set-Text '替换为' 'stage'
  Click-UI '查找下一处'
  Capture $(if($all){'replace-all.png'}else{'replace-one.png'})
  Click-UI $(if($all){'全部替换'}else{'替换当前处'})
  Click-UI '关闭';Save-Subtitles
  $count=[regex]::Matches((Cue-Texts $source)[0],'stage').Count
  if($count -ne $(if($all){2}else{1})){throw 'Replacement occurrence count incorrect'}
  Undo-Save
 }
 Set-Text '搜索' ''
 Click-UI '句号转逗号';Save-Subtitles
 if(((Cue-Texts $source)-join '') -match '[.。．]'){throw 'Manual period conversion missed an ending'}
 Undo-Save
 Click-UI '移除句末标点';Save-Subtitles
 $texts=Cue-Texts $source
 if($texts[0].EndsWith('.') -or $texts[2] -ne 'Check final inputs. Save every result' -or -not $texts[0].Contains('workflow.')){throw 'Trailing cleanup changed internal punctuation'}
 Undo-Save
 $sourceHash=(Get-FileHash $source).Hash
 # A single real local provider call creates the independent working track.
 Click-UI 'AI 翻译';Click-UI '开始翻译字幕'
 $translated=Join-Path $workPath 'sample-zh.srt'
 for($i=0;$i -lt 300;$i++){if(Test-Path $translated){break};Start-Sleep -Milliseconds 100}
 if(-not (Test-Path $translated)){throw 'Translation output missing'}
 Click-UI '优化字幕'
 Click-UI '智能拆分 · 2 条长字幕'
 Save-Subtitles
 Capture 'split-translated.png'
 if((Assert-Sequence $translated) -le 3){throw 'Working track was not split'}
 $splitTexts=Cue-Texts $translated
 foreach($text in $splitTexts[0..($splitTexts.Count-3)]){if($text -match '[.。．]|[,，]$'){throw 'Working split punctuation was not cleaned'}}
 if((Get-FileHash $source).Hash -ne $sourceHash){throw 'Working split changed original file'}
 Undo-Save
 Split-One '最后检查所有输入，然后保存全部字幕结果。'
 Save-Subtitles
 if((Assert-Sequence $translated) -ne 4 -or (Get-FileHash $source).Hash -ne $sourceHash){throw 'Single working split did not preserve original track'}
 Capture 'single-translated.png'
 Undo-Save
 foreach($case in @(
  @{text='为什么公司几乎立即就道歉？';name='compact-question-translated.png'},
  @{text='你能不能具体讲讲发生了什么？';name='compact-predicate-translated.png'},
  @{text='Kylian Mbappé 是个极具统治力的球员';name='compact-name-translated.png'}
 )){Check-CompactCue '最后检查所有输入，然后保存全部字幕结果。' $case.text $translated $case.name}
 if((Get-FileHash $source).Hash -ne $sourceHash){throw 'Compact working-cue checks modified the original file'}
 Click-UI '查找替换';Set-Text '查找内容' '字幕';Set-Text '替换为' '台词';Click-UI '全部替换';Click-UI '关闭';Save-Subtitles
 if(-not ((Cue-Texts $translated)-join '').Contains('台词') -or (Get-FileHash $source).Hash -ne $sourceHash){throw 'Working replacement scope incorrect'}
 Undo-Save
 Click-UI '句号转逗号';Save-Subtitles
 if(((Cue-Texts $translated)-join '') -match '[.。．]'){throw 'Working punctuation not applied'}
 Undo-Save
 Click-UI '移除句末标点';Save-Subtitles
 foreach($text in (Cue-Texts $translated)){if($text.EndsWith('。')){throw 'Working trailing punctuation remained'}}
 if((Get-FileHash $source).Hash -ne $sourceHash){throw 'Working cleanup changed original file'}
 Capture 'finished.png'
 Write-Output 'PASS: packaged toolbar grouping, save status inside the action, original/working split with automatic punctuation cleanup, compact questions and mixed names unchanged without an undo step, unchanged files before Save, numbering, single/all replacement, manual punctuation with options off, one-step undo, independent tracks'
} catch {
 Capture 'failure.png'
 $root=Root;if($root){$root.FindAll([System.Windows.Automation.TreeScope]::Descendants,[System.Windows.Automation.Condition]::TrueCondition)|ForEach-Object{Write-Output ('UIA: '+$_.Current.Name)}}
 throw
} finally {
 $p.Refresh()
 if(-not $p.HasExited){[void]$p.CloseMainWindow();for($i=0;$i -lt 100;$i++){if($p.HasExited){break};if(Find-UI '不保存'){Click-UI '不保存'};Start-Sleep -Milliseconds 100;$p.Refresh()};if(-not $p.WaitForExit(10000)){$p.Kill();$p.WaitForExit();throw 'Isolated test window did not close normally'}}
}
