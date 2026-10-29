# OneSub 品牌图标

`logo.png` 为维护者提供并指定用于 OneSub 的原始图标，1200 × 1200，RGBA。保持原图构图和透明度，不重绘。

SHA-256：`ad0d5219f8e70fb8138eea649d1f43de00f8ff5e3a884e9c2a64db5974eaa233`

MyGo 构建器从原图生成 Windows 多分辨率图标（16、24、32、48、64、128、256 px），应用与安装器共用；托盘按系统 DPI 生成 16、20、24、32、48 px 等实际像素尺寸，使用 `golang.org/x/image/draw.CatmullRom` 缩小原图并保留透明边缘。派生图标仅进入忽略的构建产物，不作为手工维护的源码副本。
