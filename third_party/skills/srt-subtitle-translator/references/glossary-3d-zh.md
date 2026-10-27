# Glossary — 3D / motion graphics, Simplified Chinese

Default renderings for 3D, motion-graphics, and VFX tutorial subtitles with a Simplified
Chinese target. Read it whenever the source video is about Cinema 4D, Blender, Maya,
3ds Max, Houdini, Redshift, Octane, Arnold, or any adjacent tool.

## The alignment rule

**This skill's house default aligns shared 3D concepts to Cinema 4D's Simplified Chinese
wording.** Keep one rendering per concept across applications. This is a target-audience
convention, not a claim that other applications' localizations are universally wrong.
An explicit user glossary, the actual on-screen label, or a different target locale takes
precedence over this default.

This holds even when the video is a Blender or Maya tutorial: translate `Inset Faces` as
`内部挤压`, not Blender's own 内插面; translate Maya's `Extrude` as `挤压`, not 挤出.

**The one exception: a feature that genuinely exists only in that application.** Keep its
own name — 几何节点（Geometry Nodes）, 蜡笔（Grease Pencil）, XPresso, MoSpline, Pyro,
Houdini's SOP/DOP/VOP/Wrangle, Redshift's AOV. There is no C4D word to align to, so the
software's own term is the correct term.

## This is a table of defaults, not a find-and-replace list

The goal of this skill is a subtitle that sounds like it was written in Chinese. A
glossary applied mechanically defeats that as thoroughly as a bad translation does — it
just fails in a more consistent way.

**Context outranks the table. Always.** Before using a row, ask which of these the
speaker is doing:

**1. Pointing at the screen, or narrating an action?** When the viewer has to find a
thing in a menu, a manager, or a parameter field, use the UI wording — that is the string
on screen and matching it is the whole point. When the speaker is just describing what
they are doing, use how a Chinese creator would actually say it out loud.

- `It's under Create > Null` → menu path, the UI word
- `Create a null and drop everything inside` → 新建一个空对象，把东西都放进去 —
  nobody says 新建一个空白 in speech
- `Set the falloff to 50` → parameter field, 衰减
- `It falls off toward the edges` → narration: 边缘会逐渐变淡, not 边缘衰减

**2. Which sub-field is this?** Many English words in this domain map to several unrelated
Chinese words. Getting this wrong doesn't read as awkward — it reads as nonsense. See the
next section.

**3. Is the English word doing real work here?** Speakers pad with `basically`, `just`,
`some`, `a little bit of`. `Add a little bit of noise` is 加一点噪波 — not a sentence
about 噪波量的少量增加. The glossary supplies the noun; the sentence is still yours to
write.

Two habits that keep the table honest: read the row, then read your line aloud in your
head and ask whether a Chinese creator would say it that way; and when the video uses a
term in a sense this file didn't anticipate, translate the sense, not the row.

**Parseability is the entry fee.** A viewer outside the niche must be able to roughly
guess a term's meaning from its characters. 大型 fails for *blocking*; 基础形体 passes.
When community slang fails the test, ship the plain wording with the original bracketed
at first appearance (基础形体（Blocking）), and keep the verbatim string for anything the
viewer watches being typed on screen (命名对象就叫 Blocking).

## Avoid in this house glossary

Use these renderings for the listed technical senses in ordinary Simplified Chinese
narration. They do not override literal UI labels, user-locked terms, or unrelated senses.

| Never | Always | Source term | Why the drift happens |
|---|---|---|---|
| 内插、内插面 | **内部挤压** | Inner Extrude / Inset | Blender zh-CN uses 内插面; Traditional Chinese uses 内插 |
| 内插 | **插值** | Interpolation | 内插 is the Traditional Chinese / maths-textbook word |
| 视口 | **视窗** | Viewport | literal translation, common in game-engine docs |
| 挤出 | **挤压** | Extrude | Maya and Blender zh-CN both use 挤出 |
| 环境光遮蔽 | **环境吸收** | Ambient Occlusion / AO | the literal expansion of the English |
| 细分表面 | **细分曲面** | Subdivision Surface | literal rendering of "surface" |
| 摄影机 | **摄像机** | Camera | Traditional Chinese form |
| 关键画格、键帧 | **关键帧** | Keyframe | Traditional Chinese forms |
| 烘培 | **烘焙** | Bake | homophone typo, extremely common |
| 材料 | **材质** | Material | 材料 is physical stuff, not a shading asset |
| 大型 | **基础形体** | Blocking / Blockout | spoken shorthand ("拉大型"); outside the niche it reads as "large-scale" and parses as nothing |
| 图元 | **略去，只留对象名** | Primitive (a category word stacked onto a named object) | 这是 Unity 中文文档与图形学教材里 Primitive 的译法，C4D 观众不这么叫；三层名词还会把屏幕上真正要找的名字（直线样条）埋掉 |

The table is scoped to those concepts; apply the context tests above before substituting.

## One English word, several Chinese words

Pick by what the speaker means, not by the first row. This is where mechanical glossary
use does the most damage.

**Null**
| Context | 中文 |
|---|---|
| Naming the menu item the viewer must click | the UI label — 空白 |
| Speech: a null used as a controller or parent | 空对象, or just 空 |
| Blender's *Empty* | 空物体 |
| A null value in XPresso, an expression, or script | 空值 |

**Noise** — 噪波 as a shader or procedural pattern; 噪点 as a render artefact; 噪声 or
杂音 for audio. `The render is still noisy` is 噪点还很多; `plug in a noise` is 接一个噪波.

**Light** — 灯光 for the object you create; 光线 for what travels and bounces; 光照 for
the resulting illumination. `The light bounces off the wall` → 光线打到墙上反弹, never
灯光反弹.

**Volume** — 体积 in 3D; 音量 for audio; C4D's Volume Builder is 体积生成.

**Field** — 域 for C4D Fields; 场 for a physical force field; 字段 for data.

**Scale** — 缩放 as the operation or parameter; 尺度 or 比例 when the speaker means how
big things are. `The scale of this scene is off` → 这个场景的尺度不对.

**Offset** — 偏移 as a parameter; 错开 when the speaker means staggering things in time.
`Offset the animation on each clone` → 让每个克隆体的动画错开.

**Frame** — 帧 as a unit of time; 画面 as the image; 构图 or 取景 when framing a shot.

**Render** — 渲染 as the verb; 渲染图 or 成品 for the finished image. `Compare it to the
final render` → 跟成品对比一下.

**Key** — 关键帧 in animation (`key it` → 打关键帧); 抠像 for keying/chroma key; 键 for
a keyboard key.

**Track** — 轨道 in a timeline; 跟踪 for motion tracking.

**Pass** — 通道 for a render pass; 遍 or 次 for an iteration. `Let's do another pass on
the lighting` → 布光再调一遍.

**Weight** — 权重 for vertex or bone weights; 重量 for mass in a simulation.

**Texture** — 纹理 is the image or procedural pattern itself; 贴图 is that texture wired
into a channel (法线贴图, 置换贴图). `Go grab a texture` → 找张纹理; `plug it into bump`
→ 接到凹凸贴图上.

**Displacement** — 置换 in materials; 位移 when the speaker means an object actually
moving.

**Clone** — 克隆 for the Cloner and the act; 克隆体 for the resulting copies. `The clones
are overlapping` → 克隆体重叠了.

**Primitive** — 说话人常把类别词叠在对象名后面，一句话里出现三个同指的词：`the line
spline primitive`、`the cube primitive`、`the null primitive`。中文不叠 —— 对象名自己就把
意思说完了：`Go to the line spline primitive` → 找到直线样条。类别词译出去，不是译进来，
因为观众要按名字找到屏幕上那个东西。若原文在区分基础几何体和可编辑对象，类别本身就是有效
信息，必须保留；图形学 API 中的点、线、三角形图元也不能按这条省略。

**Deformer vs Modifier** — Blender's *Modifier* is a stack of procedural operations
(Subdivision, Array, Boolean, Solidify); C4D's *Deformer* is only the shape-bending
subset. `Add a modifier` in a Blender video → 修改器, its own system. `Add a Bend
modifier` → the operation is 弯曲变形器. Judge by which one the viewer must find.

## Modelling and polygons

| English | 中文 |
|---|---|
| Extrude | 挤压 |
| Inner Extrude / Inset | 内部挤压 |
| Bevel / Chamfer | 倒角 |
| Knife / Line Cut | 切刀 / 线性切割 |
| Loop Cut | 循环切割 |
| Subdivision Surface | 细分曲面 |
| Optimize | 优化 |
| Weld / Merge | 焊接 / 合并 |
| Bridge | 桥接 |
| Close Polygon Hole | 封闭多边形孔洞 |
| Point / Edge / Polygon | 点 / 边 / 多边形 |
| Normal | 法线 |
| Phong | 平滑着色 |
| Symmetry | 对称 |
| Array | 阵列 |
| Boole | 布尔 |
| Connect | 连接 |
| Instance | 实例 |
| Metaball | 融球 |
| Spline | 样条 |
| Loft | 放样 |
| Sweep | 扫描 |
| Lathe | 旋转 |
| Vertex Map | 顶点贴图 |
| Selection | 选集 |
| Live Selection | 实时选择 |
| Retopology | 重新拓扑 |
| Remesh | 重构网格 |
| Blocking / Blockout (the stage, the mesh) | 基础形体（首次出现标注 Blocking） |
| Blocking (name typed on screen) | Blocking（保留原名） |
| Polygon Pen | 多边形笔 |
| Loop / Path Cut | 循环切割 |
| Line Cut | 线性切割 |
| Grab Brush | 抓取笔刷 |
| Stitch and Sew | 缝合 |
| Fill Selection | 填充选择 |
| Soft Selection | 柔和选择 |
| Enable Axis | 启用轴心 |
| Connect Objects plus Delete | 连接对象并删除 |
| Backface Culling | 背面剔除 |
| Caps (cylinder) / Caps tab | 封盖 / 封盖选项卡 |
| Height / Rotation Segments | 高度分段 / 旋转分段 |
| Support Loop | 支撑循环边 |
| Pole (3- or 5-edge vertex) | 极点 |

## Character anatomy (topology & rigging tutorials)

| English | 中文 |
|---|---|
| Deltoid | 三角肌 |
| Pectoral (muscle) | 胸大肌 |
| Clavicle | 锁骨 |
| Acromion | 肩峰 |
| Scapula / Shoulder Blade | 肩胛骨 |
| Trapezius | 斜方肌 |
| Latissimus Dorsi | 背阔肌 |
| Ribcage | 胸廓 |
| Pubis / Pubic Bone | 耻骨 |
| Armpit | 腋窝 |
| Nasolabial Fold | 法令纹 |

## MoGraph and Fields

| English | 中文 |
|---|---|
| MoGraph | 运动图形 |
| Cloner | 克隆 |
| Matrix | 矩阵 |
| Fracture | 破碎 |
| Tracer | 追踪对象 |
| Effector | 效果器 |
| Plain / Random / Shader Effector | 简易 / 随机 / 着色 效果器 |
| Step / Delay / Formula Effector | 步幅 / 延迟 / 公式 效果器 |
| Time / Inheritance / Target Effector | 时间 / 继承 / 目标 效果器 |
| Push Apart / Sound / Volume Effector | 推离 / 声音 / 体积 效果器 |
| Linear / Radial / Grid Array (clone mode) | 线性 / 放射 / 网格排列 |
| Object mode (Cloner) | 对象模式 |
| Iterate / Blend | 迭代 / 混合 |
| Field | 域 |
| Linear / Spherical / Box / Cylinder Field | 线性 / 球体 / 立方体 / 圆柱体 域 |
| Random / Shader / Sound / Formula Field | 随机 / 着色器 / 声音 / 公式 域 |
| Group Field | 组域 |
| Falloff / Decay | 衰减 |
| Remapping | 重映射 |
| Inner Offset | 内部偏移 |
| Strength | 强度 |

## Deformers

| English | 中文 |
|---|---|
| Deformer | 变形器 |
| Bend / Bulge / Shear | 弯曲 / 膨胀 / 倾斜 |
| Taper / Twist | 锥化 / 扭曲 |
| Squash & Stretch | 挤压 & 伸展 |
| Displacer | 置换 |
| Jiggle | 抖动 |
| Smoothing | 平滑 |
| Shrink Wrap | 收缩包裹 |
| Spherify | 球化 |
| Spline Wrap | 样条约束 |
| Mesh Deformer | 网格变形器 |
| Collision | 碰撞 |
| Correction | 修正 |
| Wind | 风力 |

## Materials and shading

| English | 中文 |
|---|---|
| Material | 材质 |
| Shader | 着色器 |
| Color | 颜色 |
| Luminance | 发光 |
| Transparency / Refraction | 透明 / 折射 |
| Reflectance / Reflection | 反射 |
| Roughness / Metalness | 粗糙度 / 金属度 |
| Bump / Normal Map | 凹凸 / 法线贴图 |
| Specular | 高光 |
| Fresnel | 菲涅耳 |
| Gradient | 渐变 |
| Fusion / Layer | 融合 / 层 |
| Tiling | 平铺 |
| Projection | 投射 |
| UV / UVW | UV / UVW |
| UV Unwrap | UV 展开 |
| Node / Node Editor | 节点 / 节点编辑器 |

## Lighting and rendering

| English | 中文 |
|---|---|
| Lighting | 布光 |
| Area / Spot / Infinite Light | 区域光 / 聚光灯 / 无限光 |
| Sun / Sky | 太阳 / 天空 |
| Shadow / Soft Shadow | 阴影 / 柔和阴影 |
| Intensity / Exposure | 强度 / 曝光 |
| Global Illumination (GI) | 全局光照 |
| Ambient Occlusion (AO) | 环境吸收 |
| Caustics | 焦散 |
| Depth of Field | 景深 |
| Motion Blur | 运动模糊 |
| Render Settings | 渲染设置 |
| Picture Viewer | 图片查看器 |
| Sampling / Samples | 采样 |
| Denoise | 降噪 |
| Bounce | 反弹 |
| Path / Ray Tracing | 路径追踪 / 光线追踪 |
| Anti-aliasing | 抗锯齿 |
| Resolution / Frame Rate | 分辨率 / 帧率 |
| Multi-Pass | 多通道 |
| Tone Mapping | 色调映射 |
| Render Region | 渲染区域 |

## Animation

| English | 中文 |
|---|---|
| Keyframe | 关键帧 |
| Timeline | 时间线 |
| Dope Sheet | 摄影表 |
| F-Curve | 函数曲线 |
| Interpolation | 插值 |
| Linear / Spline / Step (interpolation) | 线性 / 样条 / 步幅 |
| Ease In / Ease Out | 缓入 / 缓出 |
| Tangent | 切线 |
| Loop / Cycle | 循环 |
| Rig / Joint / Bone | 绑定 / 关节 / 骨骼 |
| Weight Painting | 权重绘制 |
| Constraint | 约束 |
| Align to Spline | 沿样条对齐 |
| Vibrate | 震动 |
| Onion Skin | 洋葱皮 |
| Playback Range | 播放范围 |

## Simulation and dynamics

| English | 中文 |
|---|---|
| Simulation | 模拟 |
| Dynamics | 动力学 |
| Rigid Body / Soft Body | 刚体 / 柔体 |
| Collider | 碰撞体 |
| Cloth | 布料 |
| Rope | 绳索 |
| Particle / Emitter | 粒子 / 发射器 |
| Force / Gravity | 力 / 重力 |
| Turbulence | 湍流 |
| Attractor | 吸引 |
| Friction / Mass | 摩擦 / 质量 |
| Substeps | 子步 |
| Cache | 缓存 |
| Bake | 烘焙 |
| Volume Builder | 体积生成 |
| VDB | VDB |

## Interface and transforms

| English | 中文 |
|---|---|
| Viewport | 视窗 |
| Attribute Manager | 属性管理器 |
| Object Manager | 对象管理器 |
| Material Manager | 材质管理器 |
| Coordinate Manager | 坐标管理器 |
| Asset Browser | 资产浏览器 |
| Tag | 标签 |
| Preset | 预设 |
| Take | 场次 |
| Scene / Project Settings | 场景 / 工程设置 |
| Hierarchy / Parent / Child | 层级 / 父级 / 子级 |
| Group | 群组 |
| Snap / Grid | 捕捉 / 网格 |
| Move / Scale / Rotate | 移动 / 缩放 / 旋转 |
| Position / Rotation | 位置 / 旋转 |
| Pivot / Axis | 轴心 / 轴 |
| World / Object coordinates | 世界坐标 / 对象坐标 |
| Freeze Transformation | 冻结变换 |
| Reset | 重置 |
| Example (in a tutorial) | 案例 |

## Stays in the original form

Product and vendor names, file formats, and established acronyms. Translating these makes
them unsearchable, which is the opposite of what a tutorial viewer needs.

- Applications and renderers: `Cinema 4D`, `Blender`, `Maya`, `Houdini`, `Redshift`,
  `Octane`, `Arnold`, `V-Ray`, `Unreal`, `After Effects`, `Nuke`
- Acronyms: `UV`, `HDRI`, `GI`, `AO`, `AOV`, `IK`, `FK`, `LUT`, `VDB`, `ACES`, `PBR`
- Formats: `OBJ`, `FBX`, `Alembic`, `USD`, `EXR`, `PNG`
- Application-exclusive identifiers: `XPresso`, `MoSpline`, `Pyro`, `Eevee`, `Cycles`,
  `SOP` / `DOP` / `VOP`, `Wrangle`, `Hypershade`. Features with established localized
  names such as Geometry Nodes / 几何节点 and Grease Pencil / 蜡笔 follow the UI/context
  rule rather than being locked to English in every sentence.

Abbreviations the speaker actually says stay as spoken: `C4D`, `AE`, `RS`, `PS`. Do not
expand `C4D` into `Cinema 4D` in a subtitle — it costs characters and the viewer already
knows it.

On first appearance of a concept the viewer may not connect to the English audio, use
`中文（English）` — `内部挤压（Inner Extrude）` — then the short form for the rest of the
file. Do this for the two or three terms the video is *about*, not for every row above.

## Beyond this list

This table covers 3D and motion graphics only. For a cooking, finance, or medical video,
build the equivalent list for that domain before translating, following the same two
principles: pick the wording the target audience already uses rather than the literal one,
and treat the list as defaults that the sentence's context can override.
