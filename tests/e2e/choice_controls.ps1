# Dot-sourced only by the isolated native smoke driver. All UIA objects belong
# to that test-owned process; no foreground, desktop switch or global input.
function Choice-Items {
    $root=[System.Windows.Automation.AutomationElement]::FromHandle(([SmokeNative]::TestWindow($p.Id)))
    $condition=[System.Windows.Automation.PropertyCondition]::new([System.Windows.Automation.AutomationElement]::ControlTypeProperty,[System.Windows.Automation.ControlType]::ListItem)
    return @($root.FindAll([System.Windows.Automation.TreeScope]::Descendants,$condition))
}
function Choose-EndItem($capture) {
    $items=Choice-Items
    if($items.Count -lt 9){throw "Expected a long choice list: $capture ($($items.Count))"}
    $last=$items[-1]
    $name=$last.Current.Name
    $first=@($items | Where-Object {-not $_.Current.IsOffscreen})[0].Current.BoundingRectangle
    $x=[int]($first.X+$first.Width/2);$y=[int]($first.Y+$first.Height/2)
    # Send wheel messages solely to this verified private HWND. UIA does not
    # expose a ScrollPattern for this native popup's nested scroll element.
    for($i=0;$i -lt 40;$i++) {
        if(-not [SmokeNative]::Wheel(([SmokeNative]::TestWindow($p.Id)),$p.Id,$x,$y,-1200)){throw 'Test surface is unavailable'}
        Start-Sleep -Milliseconds 100
        $last=@(Choice-Items | Where-Object {$_.Current.Name -eq $name})[0]
        if(-not $last.Current.IsOffscreen){break}
    }
    Start-Sleep -Milliseconds 350
    $last=@(Choice-Items | Where-Object {$_.Current.Name -eq $name})[0]
    if($last.Current.IsOffscreen){throw "Last option remains offscreen: $name"}
    & $ScreenshotTool $p.Id (Join-Path $workPath ($capture+'.png')) | Out-Null
    Select-Choice $name
    Start-Sleep -Milliseconds 350
    return $name
}
function Test-ChoiceControls {
    $add=(Find-UI '添加文件').Current.BoundingRectangle
    $clear=(Find-UI '清空工作台').Current.BoundingRectangle
    if([Math]::Abs($add.Height-$clear.Height) -gt 1){throw 'Clear and Add targets differ in height'}
    $root=[System.Windows.Automation.AutomationElement]::FromHandle(([SmokeNative]::TestWindow($p.Id)))
    foreach($node in $root.FindAll([System.Windows.Automation.TreeScope]::Descendants,[System.Windows.Automation.Condition]::TrueCondition)) {
        $pattern=$null
        if($node.Current.Name -eq '预设' -and $node.TryGetCurrentPattern([System.Windows.Automation.ExpandCollapsePattern]::Pattern,[ref]$pattern)){
            Pointer-ClickRow $node.Current.BoundingRectangle
            break
        }
    }
    Start-Sleep -Milliseconds 300
    $presets=Choice-Items
    if($presets.Count -lt 10){throw 'Preset list is incomplete'}
    if(@($presets | Where-Object {$_.Current.IsOffscreen}).Count){throw 'All presets must be visible without scrolling'}
    & $ScreenshotTool $p.Id (Join-Path $workPath 'all-presets.png') | Out-Null
    Pointer-ClickRow $presets[-1].Current.BoundingRectangle
    Start-Sleep -Milliseconds 350
    if(-not (Read-Preferences).stylePreset){throw 'Preset choice was not saved'}
    Click-UI '展开选项'
    $font=Choose-EndItem 'font-list-end'
    $saved=$false
    for($i=0;$i -lt 30;$i++) {
        if((Read-Preferences).lastStyle.fontName -eq $font){$saved=$true;break}
        Start-Sleep -Milliseconds 100
    }
    if(-not $saved){throw 'Font choice was not saved'}
    $bold=Find-UI '粗体'
    $bold.GetCurrentPattern([System.Windows.Automation.TogglePattern]::Pattern).Toggle()
    Start-Sleep -Milliseconds 350
    & $ScreenshotTool $p.Id (Join-Path $workPath 'selected-typeface.png')
    Click-UI '设置'
    Select-Tab '设置分类' '关于'
    & $ScreenshotTool $p.Id (Join-Path $workPath 'about.png')
    $automatic=Find-UI '自动检查并安装更新'
    $old=(Read-Preferences).autoUpdate
    # The label and switch share a field; target the actual toggle pattern.
    $root=[System.Windows.Automation.AutomationElement]::FromHandle(([SmokeNative]::TestWindow($p.Id)))
    foreach($item in $root.FindAll([System.Windows.Automation.TreeScope]::Descendants,[System.Windows.Automation.Condition]::TrueCondition)){
        $toggle=$null
        if($item.Current.Name -eq '自动检查并安装更新' -and $item.TryGetCurrentPattern([System.Windows.Automation.TogglePattern]::Pattern,[ref]$toggle)){$toggle.Toggle();break}
    }
    Start-Sleep -Milliseconds 350
    if((Read-Preferences).autoUpdate -eq $old){throw 'Automatic update switch was not saved'}
    Select-Tab '设置分类' '翻译服务'
    Click-UI '切换模型'
    $model=Choose-EndItem 'settings-model-list-end'
    if((Read-Preferences).providerModel -ne $model){throw 'Settings model choice was not saved'}
    Click-UI '关闭'
    Select-Tab '识别与翻译' 'AI 翻译'
    Click-UI '切换模型'
    $model=Choose-EndItem 'workbench-model-list-end'
    if((Read-Preferences).providerModel -ne $model){throw 'Workbench model choice was not saved'}
    Write-Output 'PASS: private native full preset list; font/model scrolling and selection; saved choices; equal toolbar targets; About update switch'
}
