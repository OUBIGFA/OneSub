# 贡献指南

先阅读 [产品范围](docs/product-plan.md)与[仓库规划](docs/repository-plan.md)。当前接受规划、证据和仓库维护改进；功能开发按[阶段验收](docs/roadmap.md)进行。

1. 将源码、可公开夹具、长期文档和构建定义放入对应目录；私人输入、下载、日志和截图放 `_temp/<任务>/` 或 `.local/`。
2. 第三方内容先登记来源、精确版本、许可和修改范围。真实字幕和媒体不直接附在 Issue 或 PR 中。
3. 使用 UTF-8、LF 和对应语言锁文件。不要强制添加忽略文件或提交生成产物。
4. 暂存本次文件，然后执行：

   ```text
   python tools/check_repo.py
   python -m unittest discover -s tests/repository -v
   git diff --cached --check
   git diff --cached
   git status --short
   ```

检查器读取暂存索引；未暂存文件不属于该次检查对象。回归测试建立真实小型 Git 仓库，测试产物保存在被忽略的 `_temp/` 下；本地清理时核对路径后移入回收站。

提交信息使用 `type(scope): 摘要`。PR 描述具体问题、最终行为、验证方法和未覆盖范围。产品改动按风险增加真实行为测试；纯文案不需要为文字本身另写测试。

现有检查仅覆盖仓库公开边界、常见凭据形态、编码、体积、JSON 和 Markdown 本地文件链接；不验证网页可用性、Markdown 标题锚点或媒体/应用功能。公开前仍需人工检查。
