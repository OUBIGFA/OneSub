param([string]$Exe,[string]$Work,[ValidateSet('media','subtitle')][string]$Entry='media',[ValidateSet('exact','language','typo','words','ambiguous','numeric-conflict')][string]$Scenario='language',[string]$ScreenshotTool,[switch]$Packaged)
$ErrorActionPreference='Stop'
Add-Type -AssemblyName UIAutomationClient
Add-Type -AssemblyName UIAutomationTypes
$workPath=[IO.Path]::GetFullPath($Work)
if(Test-Path $workPath){throw 'Use a fresh weighted-association directory'}
New-Item -ItemType Directory "$workPath/input","$workPath/data","$workPath/ipc" -Force | Out-Null
$env:ONESUB_DATA_ROOT="$workPath/data";$env:XDG_RUNTIME_DIR="$workPath/ipc";$env:WEBVIEW2_USER_DATA_FOLDER="$workPath/webview"
if($env:XDG_RUNTIME_DIR.Length -gt 65){throw 'Use a shorter isolated IPC path'}
[IO.File]::WriteAllText("$workPath/data/settings.json",'{"version":5,"uiLanguage":"zh-Hans","closeToTray":false,"appearance":"light"}',[Text.UTF8Encoding]::new($false))
if($Packaged){foreach($name in @('ONESUB_FFMPEG','ONESUB_FFPROBE','ONESUB_SUBTITLE_WORKER','ONESUB_STYLE_WORKER','ONESUB_SKILL_WORKER','ONESUB_SKILL_ROOT')){Remove-Item "Env:$name" -ErrorAction SilentlyContinue}}
$names=switch($Scenario){
 'exact' {@('Lesson 01 Introduction.mp4','Lesson 01 Introduction.srt')}
 'language' {@('Lesson 01 Introduction.mp4','Lesson 01 Introduction-zh.srt')}
 'typo' {@('Blender Material Editing.mp4','Blnder Material Editing.srt')}
 'words' {@('Introduction to Blender Materials.mp4','Blender Materials.srt')}
 'ambiguous' {@('Introduction to Blender.mp4','Introducton to Blender.srt','Introduction for Blender.srt')}
 'numeric-conflict' {@('Lesson 01 Introduction.mp4','Lesson 02 Introduction.srt')}
}
$video=Join-Path $workPath ('input/'+$names[0]);$subtitle=Join-Path $workPath ('input/'+$names[1])
& ffmpeg -v error -f lavfi -i 'color=c=green:size=640x360:rate=30:duration=10' -c:v libx264 -pix_fmt yuv420p $video
if($LASTEXITCODE -ne 0){throw 'Weighted video fixture creation failed'}
[IO.File]::WriteAllText($subtitle,"1`n00:00:00,100 --> 00:00:09,900`nWeighted source`n",[Text.UTF8Encoding]::new($false))
$hashes=@{};$hashes[$subtitle]=(Get-FileHash $subtitle -Algorithm SHA256).Hash
$second=$null
if($Scenario -eq 'ambiguous'){
 if($Entry -ne 'media'){throw 'The ambiguity scenario requires a media entry'}
 $second=Join-Path $workPath ('input/'+$names[2])
 [IO.File]::WriteAllText($second,"1`n00:00:00,100 --> 00:00:09,900`nExplicit candidate`n",[Text.UTF8Encoding]::new($false))
 $hashes[$second]=(Get-FileHash $second -Algorithm SHA256).Hash
}
$primary=if($Entry -eq 'media'){$video}else{$subtitle}
$p=Start-Process -FilePath ([IO.Path]::GetFullPath($Exe)) -ArgumentList @('"'+$primary+'"') -RedirectStandardError "$workPath/stderr.log" -PassThru
function Find-UI($name){
 $p.Refresh();if($p.HasExited -or $p.MainWindowHandle -eq 0){return $null}
 $root=[System.Windows.Automation.AutomationElement]::FromHandle($p.MainWindowHandle)
 return $root.FindFirst([System.Windows.Automation.TreeScope]::Descendants,[System.Windows.Automation.PropertyCondition]::new([System.Windows.Automation.AutomationElement]::NameProperty,$name))
}
function Wait-UI($name){
 for($i=0;$i -lt 150;$i++){
  try{$item=Find-UI $name;if($item){return $item}}catch [System.Windows.Automation.ElementNotAvailableException]{}
  Start-Sleep -Milliseconds 100
 }
 throw "Weighted association control unavailable: $name"
}
function Current-Manifest{
 $folder=Get-ChildItem -LiteralPath "$workPath/data/projects" -Directory | Sort-Object CreationTimeUtc -Descending | Select-Object -First 1
 return ([IO.File]::ReadAllText("$($folder.FullName)/project.json")|ConvertFrom-Json)
}
try{
 if($Scenario -eq 'ambiguous'){[void](Wait-UI '选择关联文件')}else{[void](Wait-UI '添加文件')}
 if($Scenario -eq 'numeric-conflict'){
  if($Entry -eq 'media'){
   [void](Wait-UI '00:00:10.000');[void](Wait-UI '未加载字幕')
   $manifest=Current-Manifest
   if($manifest.mediaPath -ne $video -or $manifest.subtitlePath -ne ''){throw 'Different chapter numbers were paired'}
  }else{
   [void](Wait-UI 'Weighted source');$manifest=Current-Manifest
   if($manifest.subtitlePath -ne $subtitle -or $manifest.mediaPath -ne ''){throw 'Different chapter numbers were paired'}
  }
 }else{
  # The modal exposes only its own UIA tree. Inspect the primary manifest
  # while choices are open; read the media controls after closing it.
  if($Scenario -eq 'ambiguous'){
   [void](Wait-UI '选择关联文件')
   $first=(Wait-UI $names[1]).Current.BoundingRectangle
   $next=(Wait-UI $names[2]).Current.BoundingRectangle
   [void](Wait-UI '匹配分 91');[void](Wait-UI '匹配分 89')
   if($first.Y -ge $next.Y -or (Current-Manifest).subtitlePath -ne ''){throw 'Close candidates were not sorted for a choice'}
   if($ScreenshotTool){& $ScreenshotTool $p.Id "$workPath/candidates.png" -wait 400 | Out-Host;if($LASTEXITCODE -ne 0){throw 'Candidate capture failed'}}
   (Wait-UI $names[2]).GetCurrentPattern([System.Windows.Automation.InvokePattern]::Pattern).Invoke()
   [void](Wait-UI 'Explicit candidate');$subtitle=$second
  }else{[void](Wait-UI 'Weighted source')}
  [void](Wait-UI '00:00:10.000')
  $manifest=Current-Manifest
  if($manifest.mediaPath -ne $video -or $manifest.subtitlePath -ne $subtitle -or (Find-UI '选择关联文件')){throw 'Expected weighted association was not loaded'}
  # This is the real media clock supplied by the player, not a filename check.
  (Wait-UI '播放').GetCurrentPattern([System.Windows.Automation.InvokePattern]::Pattern).Invoke()
  [void](Wait-UI '暂停');Start-Sleep -Milliseconds 700
  if((Find-UI '播放位置').GetCurrentPattern([System.Windows.Automation.RangeValuePattern]::Pattern).Current.Value -le 0){throw 'Weighted pair did not play'}
  (Find-UI '暂停').GetCurrentPattern([System.Windows.Automation.InvokePattern]::Pattern).Invoke()
  [void](Wait-UI '播放')
 }
 foreach($file in $hashes.Keys){if((Get-FileHash $file -Algorithm SHA256).Hash -ne $hashes[$file]){throw 'Association changed a source subtitle'}}
 $result=@{entry=$Entry;scenario=$Scenario;sourceUnchanged=$true;media=$manifest.mediaPath;subtitle=$manifest.subtitlePath;explicitChoice=($Scenario -eq 'ambiguous')}
 [IO.File]::WriteAllText("$workPath/result.json",($result|ConvertTo-Json),[Text.UTF8Encoding]::new($false))
 Write-Output ($result|ConvertTo-Json)
}finally{
 $p.Refresh()
 if(-not $p.HasExited){[void]$p.CloseMainWindow();if(-not $p.WaitForExit(10000)){$p.Kill();$p.WaitForExit();throw 'Weighted test-owned window did not close normally'}}
}
