# 贡献指南

仓库只追踪源码、可公开夹具、许可与来源登记、依赖锁文件和构建定义。用户数据、缓存与构建产物不入库。

1. 将源码、可公开夹具、构建定义和固定依赖登记放入对应目录；可重复使用的下载包、工具链、运行时和本地测试安装器放 `.local/` 或 `build/output/`，按版本与哈希复用且不追踪。一次性中间内容、日志、截图和测试工作目录放 `_temp/<任务>/`，不要强制添加忽略文件或提交生成产物。
2. 第三方内容先登记来源、精确版本、许可和修改范围。真实字幕和媒体不直接附在 Issue 或 PR 中。
3. 使用 UTF-8、LF 和对应语言锁文件。
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

上面的仓库检查只覆盖公开边界、常见凭据形态、编码、体积、JSON 和 Markdown 本地文件链接。业务与工作进程检查使用 [构建说明](build/README.md) 中的统一入口，检查结果只代表实际执行的范围。

## 模块归属与并行修改

| 任务 | 主要位置 | 边界 |
| --- | --- | --- |
| 启动与系统接入 | `apps/desktop/main.go`、`window_lifecycle.go` | 组装依赖、窗口与退出；不放业务算法 |
| 工作台与状态 | `apps/desktop/workbench_view.go`、`app_state.go`、各 `*_view.go` | 原生控件和交互；后台结果经 `Window.Update` 进入 |
| 导入、任务、播放器 | `import_view.go`、`tasks.go`、`player_state.go` | 分别管理导入事务、任务完成与取消、播放状态同步 |
| 编码规则 | `apps/desktop/internal/videoencoding/` | 无界面或进程依赖；编码 ID 与质量选项的唯一来源 |
| 媒体与子进程 | `internal/media/`、`internal/jobs/` | 流映射、真实探测、输出验证；进程生命周期和有界日志 |
| 字幕与工程 | `internal/subtitle/`、`internal/project/` | 字幕规则与格式适配；事务、修订、撤销和保存边界 |
| 翻译 | `internal/translator/` | `types.go` 定义契约，`translate.go` 编排，`batches.go` 调度，`events.go` 通知，`validation.go` 校验 |
| 工作进程 | `workers/` | 字幕格式、字体与 ASS、字幕检查的独立实现；不接管工程数据库 |

业务包不导入原生界面组件。窗口和 MCP 共用业务入口，避免各自复制规则。编码选项的显示名称留在界面，持久化 ID 和可选组合归编码模块。应用状态目前仍由主线程统一持有，修改公共状态字段、任务生命周期或跨模块契约时需要协调。

并行工作先划分文件和接口归属。每项工作检查调用方并运行相关回归；集成时统一执行仓库、核心和工作进程检查。不要因并行开发复制业务实现，也不要在各任务中同时更新依赖锁、生成资源或覆盖同一安装器。
