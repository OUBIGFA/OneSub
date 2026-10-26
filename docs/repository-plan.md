# 仓库结构、公开边界与长期卫生

状态：仓库管理规范 · 2026-10-26

## 1. 原则

采用单仓库管理桌面应用、受控工作进程、协议、测试与文档。**当前只创建已经有内容的目录，不用空 `.gitkeep` 预支未来结构。** 新增模块按下述职责落位，不在根目录散放脚本、导出物或研究下载。

Git 跟踪的是可审查的开发输入，不是工作电脑的镜像。公开 Git、发布附件、私有本地数据是三个独立边界，不能用 Git LFS 把敏感文件“合法化”。

## 2. 当前交付结构

```text
OneSub/
  README.md
  AGENTS.md
  LICENSE
  CONTRIBUTING.md
  SECURITY.md
  .gitignore
  .gitattributes
  .editorconfig
  .github/
    workflows/repository-check.yml
    ISSUE_TEMPLATE/
    pull_request_template.md
  docs/
    product-plan.md
    architecture.md
    ui-design.md
    repository-plan.md
    roadmap.md
    security-and-release.md
    research/landscape.md
    research/evidence.json
    decisions/0001-product-and-reuse.md
  third_party/README.md
  tools/check_repo.py
  tests/repository/test_check_repo.py
```

当前没有应用 scaffold、语言依赖锁或二进制，避免将未验证的模板写成可运行产品。仓库维护检查不等于应用测试。

## 3. 后续结构与创建时机

```text
apps/desktop/                  # G1 首个 MyGo 可运行薄链路时建立
  main.go                      # 桌面进程入口
  go.mod, go.sum
  package.json, bun.lock       # 一个前端包管理器及其锁文件
  mygo.config.ts
  internal/
    appservice/                # 受限服务绑定
    project/                   # schema迁移、事务、修订、备份
    jobs/                      # 状态、资源、进程树、恢复
    asr/                       # OneAsr适配和模型清单消费
    subtitle/                  # 规范模型、libse、格式保护
    translator/                # 技能、候选、provider和缓存
    media/                     # probe、映射、代理、peaks、ComposePlan
    fonts/                     # FontPlan与字形覆盖
    security/                  # Keychain、资源令牌、脱敏
  src/
    workspace/, subtitles/, preview/, styles/, jobs/
    shared/ass/                # 登记来源后的SmartSub纯TS抽取
    generated/                 # MyGo绑定，生成、不入Git
workers/
  subtitle-core/               # .NET worker源码和NuGet锁
  skill-tools/                 # Python包装与冻结构建配置
contracts/                     # helper/项目/任务schema及示例
assets/                        # 自有图标、源码SVG、许可清楚的静态资源
build/                         # 构建配方、平台打包定义，不放产物
  output/                      # 忽略；每平台staging
third_party/
  manifests/                   # 精确依赖/模型/字体记录，G0建立
  licenses/                    # 实际引入组件的许可副本
  ports/concat/, ports/smartsub/ # PORTING说明、范围和哈希
  patches/oneasr/              # 有测试的有限patchset
  skills/srt-subtitle-translator/ # 获得明确授权后的完整受控快照
  sources/                     # 下载/解包上游源码，忽略
tests/                         # 与现有tests合并，不建立另一个测试根
  contracts/, regression/, e2e/
  fixtures/                    # 小型、可公开、可追溯夹具
  corpus-manifest.json          # 大语料来源/许可/哈希，不含媒体
docs/
  acceptance/                  # 脱敏可复现验收摘要
  build-and-release.md         # 真正具备构建入口时编写
.local/                        # 忽略；本机语料、模型、工具链、工程、日志
_temp/                         # 忽略；一次任务的临时文件
```

上图 `tests/`、`docs/` 是已有根目录的扩展。MyGo 模板真实生成路径若与规划有差异，先在 G1 小范围调整目录决策与忽略规则，不能在根目录再生成一套重复应用。多语言构建需要各自锁文件，但不引入多份 npm/yarn/pnpm 锁互相竞争。

不预建微服务、通用插件框架、共享 packages 大仓工具或多应用 monorepo 编排器；只有出现第二个真实复用消费者时再提取公共包。

## 4. 哪些推送，哪些保留本地

| 类别 | 去向 | 规则 |
| --- | --- | --- |
| 手写源码、协议、迁移 | Git | 修改须带对应测试；协议只有一个权威来源 |
| lockfile、构建/CI/打包配方 | Git | 精确版本；不提交手工伪造锁文件 |
| 生成的 MyGo 绑定 | 本地/CI 重新生成 | 生成器版本与输入入库；消费者检查前先生成 |
| 自制小字幕/文本夹具 | Git `tests/fixtures/` | 文件本身和许可证/来源说明一起提交 |
| 有授权的大音视频评测集 | 本地或受控资产存储 | Git 仅放 manifest；私人课程不公开 |
| 源视频、真实字幕、翻译提示数据、项目DB | 本地 | 不因已脱敏文件名就认为内容可公开 |
| 模型、字体二进制、FFmpeg与helpers | 签名发布资产/用户下载目录 | 下载来源、revision、哈希、许可进Git，文件不进Git |
| 需要修改/移植的第三方小模块 | Git 明确目录 | 保留版权、许可证、原SHA、符号与改动记录 |
| 完整SubtitleEdit/OneAsr源码下载 | `third_party/sources/` | 精确manifest拉取；不复制完整上游历史进主仓 |
| 受控技能包 | 授权后Git快照 | 保留规则/语言/术语/脚本/fixtures，禁止自行重许可 |
| 安装包、压缩包、SBOM、校验和 | Release附件 | 不在main堆积；对应源码和构建说明可取得 |
| 截图、抓取网页、试验日志 | `_temp/<任务>/` | 仅摘要进入长期文档，截图精选且无隐私才入docs/assets |
| Key、签名证书、账号、代理凭据 | OS凭据管理/CI Secrets | 永不进入Git、文档或Issue |

字体源码/说明可以登记；获准的小型二进制测试素材需通过明确清单申请例外，不能用 `git add -f` 绕过规则。目前不需要 Git LFS；以后确有公开大资产需求，再评估配额、来源与替代存储。

## 5. 依赖下载与许可材料

选用 **精确 manifest + 校验下载 + 有限 vendor**，不默认加 submodule：

1. 大型只构建依赖按 URL/revision/archive hash 拉到 `third_party/sources/`。
2. 在构建输出目录套用有序 patchset，不直接改下载缓存。
3. 小型移植模块保存在实际源码目录，并在 `third_party/ports/` 登记；路径移动也要同步映射。
4. 完整技能快照只在权利人授权和第三方材料检查后引入。
5. 构建前验证 manifest、哈希、补丁基线，缺文件时失败，不用浮动 latest 补齐。
6. 发行提供对应源码归档，不能只给未来可能失效的上游链接；强 copyleft 组件按实际分发方式履行源码义务。

`docs/research/evidence.json` 是阅读证据，不是依赖锁，不可让构建系统消费它。真正发行清单必须包含全部传递依赖、工具链和产物来源，尚未建立时诚实标记待冻结。

## 6. 目录卫生执行

- 每次新增类别先确认公开边界；本地目录在生成文件前加入忽略规则。
- 任意层级 `_temp/` 都被忽略；该规则在 `.gitignore` 最后，晚于所有 `!` 保留规则。
- 不全局忽略 `.srt/.vtt/.ass`，否则正式字幕回归测试也会丢失；真实用户字幕必须放本地目录。
- `bin/obj/target/dist/node_modules/.venv`、模型权重、安装包与数据库全局忽略；构建定义使用 `build/`，产物只放其 output 或指定本地目录。
- 当前单个跟踪文件限制 1 MiB；较大自有资源须说明理由并修改明确白名单，不随手上调全仓阈值。
- Windows路径大小写冲突、结尾空格/点、保留设备名、symlink/submodule需检查；初期不允许跟踪symlink或嵌套仓库。
- 跨平台文本 UTF-8/LF，脚本无机器绝对路径；`.gitattributes` 统一规则，合法二进制资源后续单独声明。
- 忽略不影响已跟踪文件；误跟踪先从索引移除。秘密一旦进入历史，先撤销凭据，再协调历史清理，普通删除提交不等于消除泄露。

## 7. 分支与发布

默认分支 `main`，此次只在该分支建立文档和仓库基线，不打产品版本 tag、不发布空安装包。后续新分支由维护者明确授权或通过贡献者自己的 fork，不自动创建额外长期分支。

提交格式 `type(scope): 摘要`。本地检查、暂存差异审阅后再提交；推送不使用 force。CI 最小只读权限，第三方 Action 固定 commit SHA，外部 PR 不接收发布密钥，不使用 `pull_request_target` 执行不可信代码。

首次 CI 验证后建议为 main 开启禁止强推/删除与必需状态检查；多人维护时加入审阅要求。分支保护的启用是独立仓库管理动作，以实际平台回执为准，不能仅写了规范就宣称已启用。

## 8. 每次提交前

```text
python tools/check_repo.py
python -m unittest discover -s tests/repository -v
git diff --cached --check
git diff --cached --stat
git diff --cached
git status --short
```

检查对象为暂存索引，最终必须人工浏览本次全部文件。仓库检查工具检测常见高置信凭据形态、禁止路径和大文件，但不是秘密不泄露的数学保证；应用代码引入后增加专门扫描、依赖审计和完整测试矩阵。

公开首推只包含规划、许可与卫生工具。本地输入文档、抓取缓存、账号信息和研究脚本不推送。
