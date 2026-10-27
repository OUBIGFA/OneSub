# SmartSub 样式适配与补丁

基线：`buxuku/SmartSub@f7ce2045aea53177a60236092487897da3c8cd06`，MIT。文件闭包见 [清单](../../manifests/smartsub-style.json)。

## 字体族别名优先级

文件：`main/helpers/fontResolver.ts`；符号：`registerFaces`、`discoverInstalledFonts`。

复现：Windows 安装 Microsoft YaHei / YaHei Light 或 Arial / Arial Black 时，后扫描的变体含同名 preferredFamily、subfamily=Regular，会覆盖精确字体族别名。选择 YaHei 最终解析成 YaHei Light，粗体变体缺失。

修改：别名按精确 family、fullName/PostScript、preferredFamily 排优先级，同级优先 Regular；清理目录索引时同时清理优先级。不修改 ASS builder、样式参数或预设，不重写字体解析体系。

适配器调用原 `loadFontData` 取得常规及粗体／斜体 variants，缓存经 SHA-256 校验的字节，并把同一组字体交给 JASSUB 和 FFmpeg。缓存位于用户数据目录，不随安装器再分发系统字体。

验证：真实 Windows 字体文件的 family 与 Bold 检查，重复缓存一致性；Go 包内样式与软／硬字幕合成回归；暂停帧粗体显示由桌面截图核对。
