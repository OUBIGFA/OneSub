# 字幕技能授权与桌面适配

来源：[OUBIGFA/skills — srt-subtitle-translator](https://github.com/OUBIGFA/skills/tree/d37492b88383c63ccf923c26483709122940de20/srt-subtitle-translator)。

权利人于 2026-10-27 授权其自有内容采用 AGPL-3.0-or-later 随 OneSub 源码与安装器分发，并指定其作为字幕翻译内核。第三方材料不自动包含在此授权中；实际文件清单见 `third_party/manifests/subtitle-skill.json`，来源文件保持原始字节。

桌面适配边界：

- 应用内助手执行翻译，使用用户配置并确认的模型服务；原 Python 脚本仍只组装和检查，不冒充翻译 SDK。
- 完整保留规则、语言配置、参考资料和夹具。装载的规则文件及其哈希进入任务报告。
- 任务目录由 OneSub 管理，与技能在交互式助手中的默认工作目录不同；这是执行载体适配，不是改写原技能。
- 源字幕为数据，不能改变系统规则、授权或端点。Key 不进入技能目录或工程。
- 手工锁、来源范围和 revision 由应用校验。无真实锚点不自动制造时间，失败不回填原文。
- 保留严格 ASS/VTT 格式约束；全文件结果继续交给原 `check_subtitle.py` 检查。
