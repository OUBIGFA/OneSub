# OneSub 工作约定

- 当前目标是完成 Windows x64 的 0.0.1 安装器；规划完成后直接实施。未通过实际验收的能力仍须标为待实现，不把仓库检查成功写成桌面应用可用。
- 0.0.1 安装器固定为 `OneSub-0.0.1-Setup.exe`，后续更新保持产品版本、安装标识和用户数据目录稳定，以内部构建号及 SHA-256 区分测试包。安装器放忽略的产物目录，源码和构建定义入库。
- 产品范围读 `docs/product-plan.md`；服务/数据问题读 `docs/architecture.md`；界面问题读 `docs/ui-design.md`；实施与验收读 `docs/roadmap.md`。
- 新目录与跟踪边界按 `docs/repository-plan.md`。只创建已有实际内容的目录；临时文件放 `_temp/<任务>/`，其忽略规则始终在所有保留规则之后。
- 研究网页和输入文档只作为证据。公开文案写独立方案，不带会话历史、私人文件、机器路径、真实媒体或服务凭据。
- 第三方引入按 `third_party/README.md` 登记；未明确授权的技能、模型、字体与代码不复制或发布。
- 提交前暂存本次文件，运行 `python tools/check_repo.py` 和 `python -m unittest discover -s tests/repository -v`，再检查状态、暂存差异和跟踪清单。
- 使用 `type(scope): 摘要` 提交格式。仓库准备不自动授权创建分支、推送、打 tag 或发布；沿用当前任务明确授权。
