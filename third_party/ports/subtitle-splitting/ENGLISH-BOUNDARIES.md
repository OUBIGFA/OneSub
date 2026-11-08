# 英语字幕断句规则与资料核查

核查日期：2026-11-08。范围为识别后本地原始字幕，不改变 ASR 权重，不翻译、压缩或改写文字。本记录是独立归纳，不复制外部词典、语料或实现。

## 依据和适用边界

| 来源 | 核查结果 | 本项目采用方式 |
|---|---|---|
| [Netflix 英语字幕规范](https://partnerhelp.netflixstudios.com/hc/en-us/articles/217350977-English-USA-Timed-Text-Style-Guide) | 两行排版优先考虑标点、连接词、介词；保护冠词／名词、形容词／名词、姓名、代词／动词、助动词／动词等关系 | 用于选择语言边界。行内换行与两个字幕事件不同，不能照搬为无时间约束的强制分条 |
| [Cambridge：连接词](https://dictionary.cambridge.org/grammar/british-grammar/subordinating-conjunctions) | 同一连接词可连接单词、词组或分句；还存在多词及成对结构 | 连接词不是无条件切点。结合后续词性；紧密并列组合保留 |
| [Cambridge：时间连接词](https://dictionary.cambridge.org/grammar/british-grammar/conjunctions-time) | 时间关系可由单词、多词结构和非限定从句表达 | 区分 `after we finish`／`before saving` 与 `after lunch` 等介词用法 |
| [Cambridge：语篇标记](https://dictionary.cambridge.org/grammar/british-grammar/discourse-markers-so-right-okay) | 口语 `so` 可引入新的话语部分或重启说明 | `so we`、`so because` 等有上下文依据时作为后条开头；不把 `so bright` 当分句 |
| [Cambridge：程度弱化表达](https://dictionary.cambridge.org/grammar/british-grammar/downtoners)、[a bit](https://dictionary.cambridge.org/grammar/british-grammar/a-) | `a bit`、`a little bit`、`a tiny bit`、`kind of` 等可修饰后续成分 | 与后续程度词／形容词保持完整 |
| [Cambridge：多词动词](https://dictionary.cambridge.org/grammar/british-grammar/phrasal-verbs-and-multi-word-verbs) | 动词、小品词及介词可组成语义单位；部分结构允许宾语插入 | 使用已有词性约束保护动词链和相邻动词／介词，有限多词结构补充；不宣称穷尽所有多词动词 |
| [ACL 2023：Unsupervised Subtitle Segmentation with Masked Language Models](https://aclanthology.org/2023.acl-short.67/) | 用掩码模型在候选位置预测标点可能性；保留原文并约束长度 | 是额外模型路线，不能视为一个连接词清单或已实测替代品 |
| [IWSLT 2025：AppTek](https://aclanthology.org/2025.iwslt-1.21/) | 同时考虑字幕切分、长度、阅读速度；译文长度控制是重要组成部分 | 参考多约束评估思路；不引入译文压缩到原始字幕 |
| [IWSLT 2026：FBK](https://aclanthology.org/2026.iwslt-1.7/) | 两阶段系统用更长句级上下文改进识别和翻译，同时维持时间同步 | 支持区分输入音频分块与语义句界；不是现成的无改写字幕中间件 |
| [IWSLT 2026：HW-TSC](https://aclanthology.org/2026.iwslt-1.10/) | 结合识别、强制对齐、翻译及对超限译文的模型改写 | 与当前本地、保留原文的需求不同，不直接替换 |

论文部分核查公开摘要及可检索的方法段落，没有在本项目样本上复现其论文指标。出版年份是原论文年份，不把近期抓取日期当作新发表日期。BBC 官方页面本次未成功取得；未把同名镜像网站作为官方依据。

## 项目比较

2026-11-08 通过项目官方文档及 GitHub API 核查维护状态。以下项目只做方案比较，本轮未引入其代码、模型或运行时。

| 项目 | 作用／状态 | 判断 |
|---|---|---|
| [spaCy](https://spacy.io/usage/linguistic-features/) | 依存句法和基础名词短语；官方仓库未归档，最近推送 2026-10-27 | 比词性标签更能表达句法关系，但仍需字幕时间规划及本地模型／运行时集成；本轮未实测其优于现有路径，不直接新增依赖 |
| [pySBD](https://github.com/nipunsadvilkar/pySBD) | 完整句子的边界消歧；未归档，最近推送 2024-08-20 | 适合缩写、句号等问题，不负责长句内部的定时字幕划分 |
| [stable-ts](https://github.com/jianfch/stable-ts) | 按标点、间隔、长度重组并保护锁定边界；2026-05-30 已归档 | 可参考分阶段处理与边界保护，不描述为仍活跃的最新方案 |
| [M2L3](https://github.com/cncloudli/M2L3) | 多层 LLM 字幕系统，另有本地规则和约 5,700 项词组集合；未归档，最近推送 2026-08-18 | 未验证其效果，不复制其大词表；完整流程不等于当前纯本地、保留原文的需求 |

## 当前实现与回归矩阵

| 类别 | 例子 | 行为 |
|---|---|---|
| 完整句／标点 | 句号、问号、逗号、分号 | 句末优先，长句先用分句标点；短句不因含连接词继续拆 |
| 并列分句／动作 | `and we`、`or download`、`but you`、`yet we` | 只在后续分句／动作有依据时优先；连接词归后条 |
| 因果／条件／让步 | `because`、`unless`、`although`、`though`、`whether` | 使用后续词性证据；不从连接结构内部拆开 |
| 时间关系 | `before`、`after`、`since`、`until`、`once`、`while` | 有分句／动作证据才获得优先级；时间名词补语不是分句 |
| 多词关系 | `as soon as`、`as long as`、`even if`、`provided that`、`in order to`、`so that` | 整体判断、整体保护，不能从末尾的 `as/if/that/to` 另起 |
| 承接／转折／重启 | `so`、`however`、`therefore`、`otherwise`、`meanwhile`、`then` | 有后续分句依据时优先；不把所有口头词一律当切点 |
| 紧密并列 | `black and white`、`black or white`、`both ... and ...`、`either ... or ...` | 相邻名词／形容词／数词的组合保留；不承诺任意长度的成对结构全部不拆 |
| 程度及弱化 | `so bright`、`so much brighter`、`a bit too bright`、`kind of bright` | 修饰语与后续成分保留，避免仅修词表仍拆坏程度短语 |
| 名词短语／姓名 | 冠词、限定词、形容词、连续名词、`Dr. Jane Smith` | 保留既有词性与字面量保护 |
| 否定／动词链 | `should not have been changed`、`wouldn't have worked` | 否定、助动词、谓语及小品词连接受保护 |
| 数量／字面量 | 拼写数词、单位、小数、网址、文件名 | 数值内部不切；保留既有字面量检查 |
| 多词介词／动词 | `because of`、`in front of`、`look forward to` | 保持结构；不是强制分句名单 |
| 音频分块 | 未完句跨两个大型识别片段 | 先分出已完成句，再接续未完句，避免无关上下文耗尽容量 |
| 时间与显示 | 词时间、短头短尾、静音、字幕前导空白 | 真词时间不估算；最短时间和内容门槛仍生效；发布结果去前导空白 |

长句中的安全分句切点作为结构保留，不再因为两段合计恰好低于长度上限就抹掉切点。仍过长时使用原短语规划。所有步骤都保留词组保护。

自动输出及手动拆分的原文／译文各行去掉前导空白；内部空格、标点和词时间不变。清理只发生在切点确定之后；同步更新对应词证据的首词空白，不破坏精确文字覆盖。不可变导入证据仍保留原切片，未拆分的手动导入条目不被批量修改。

## 限制

这是带词性和上下文检查的确定性规则，不是完整依存句法解析器。相对从句、宾语从句、嵌套引语、长距离照应、ASR 错词和话语重启仍有歧义；不把 `that/who/which` 全部设为无条件切点。长无标点句可能继续采用普通短语边界，保护约束冲突时宁可保留较长条目。回归矩阵与真实样本只能证明已测试行为，不能证明对所有英语结构无误。
