# 全库画廊选稿

两页各三张可操作的 Baoyu 画板。画布名和日期是演示数据。画板从本地读取大都会艺术博物馆标记为公开领域的 26 张原始素材，图片页另加了五张裁幅和缩放样本。预览展示实际 JPG 文件像素。[大都会开放馆藏](https://www.metmuseum.org/hubs/open-access)

## 全部画布

- A · 封面网格。沿用截图中的等宽封面、名称和数量，可按最近更新、最近打开、创建时间和名称自动排序。
- B · 时间分组。把排序结果放进时间分组。原生版本已接入，侧栏与封面都可直接打开编辑画布。
- C · 侧栏速览。单击看封面与素材详情，可从详情直接打开画布。

## 全部图片

- A · 跨列瀑布。超宽图按比例跨列。1:6 竖幅与合适的邻图共用一列。其他图可以回填跨列留下的空位。间距 5 px。
- B · 齐边拼排。普通图按真实比例计算行高，6:1 和 12:1 图片有整行空间。细长图片有最小点击宽度。32 × 32 图的缩略图使用窄位，同一行可以继续排其他图。间距 4 px，末行未铺满时保留原分寸。
- C · 瀑布与详情。使用跨列瀑布，旁边显示实际像素、所在画布与馆藏来源。窄窗将详情放到图片区下方。

选稿时点 Play，可切换排序、搜索和预览。缩略图按可用空间排布。64 px 及以下的小图最多放大 2 倍，点开可切换适合窗口与原始像素。新版全部图片可在 Baoyu 选稿。

## 本机验证

三版各显示 31 张。1040 px 画板里核对了 4800 × 800、4800 × 400、640 × 3840、32 × 32、640 × 640 五张样本，图块没有重叠或越出图片墙。齐边版的 32 × 32 样本显示为 64 × 64，点击位宽 76 px。6:1 预览可看完整画幅，切换原始像素后内容为 4800 × 800 并可滚动。390 px 的 C 版仍显示全部样本，侧栏详情能读到 32 × 32。空隙主要在跨列高差或末行，三版都保留完整画幅。

[全部画布三版](http://127.0.0.1:4720/design/ZzPZoCG3YzdnqTK5YnUc9Q/#page-dd3fa3e23e76) · [全部图片三版](http://127.0.0.1:4720/design/ZzPZoCG3YzdnqTK5YnUc9Q/#page-25eab5aa91d8) · [画布预览图](../../../artifacts/gallery-variants-20261119/canvases.png) · [极端尺寸对照图](../../../artifacts/gallery-variants-20261119/images-stress.png)

预览沿用本机 Baoyu 服务（PID 72027）。从仓库根目录用该工具的 `artifact.mjs open designs/canvepp-empty-canvas` 重开，或用 `artifact.mjs stop designs/canvepp-empty-canvas` 关闭此画布的服务。[本机日志](../.artifact-server.log)

## 素材

| 馆藏 | 作者 | 下载像素 | 授权 |
| --- | --- | --- | --- |
| [大桥骤雨](https://www.metmuseum.org/art/collection/search/55433) | Utagawa Hiroshige | 429 × 625 | CC0 |
| [庄野白雨](https://www.metmuseum.org/art/collection/search/36967) | Utagawa Hiroshige | 599 × 405 | CC0 |
| [日本桥日雨](https://www.metmuseum.org/art/collection/search/36977) | Utagawa Hiroshige | 600 × 407 | CC0 |
| [神奈川冲浪里](https://www.metmuseum.org/art/collection/search/45434) | Katsushika Hokusai | 600 × 403 | CC0 |
| [马赛湾](https://www.metmuseum.org/art/collection/search/435872) | Paul Cézanne | 599 × 438 | CC0 |
| [有柏树的麦田](https://www.metmuseum.org/art/collection/search/436535) | Vincent van Gogh | 599 × 477 | CC0 |
| [大碗岛习作](https://www.metmuseum.org/art/collection/search/437658) | Georges Seurat | 599 × 407 | CC0 |
| [十月的一天](https://www.metmuseum.org/art/collection/search/437923) | Alfred Wahlberg | 600 × 382 | CC0 |
| [暮色山林](https://www.metmuseum.org/art/collection/search/39557) | Kuncan | 382 × 624 | CC0 |
| [玫瑰](https://www.metmuseum.org/art/collection/search/436534) | Vincent van Gogh | 494 × 624 | CC0 |
| [鸢尾花](https://www.metmuseum.org/art/collection/search/436528) | Vincent van Gogh | 599 × 475 | CC0 |
| [夹竹桃](https://www.metmuseum.org/art/collection/search/436530) | Vincent van Gogh | 599 × 492 | CC0 |
| [戴草帽的自画像](https://www.metmuseum.org/art/collection/search/436532) | Vincent van Gogh | 502 × 625 | CC0 |
| [年轻女子习作](https://www.metmuseum.org/art/collection/search/437879) | Johannes Vermeer | 558 × 625 | CC0 |
| [摇篮旁的女人](https://www.metmuseum.org/art/collection/search/437984) | Vincent van Gogh | 496 × 624 | CC0 |
| [阿尔勒的女子](https://www.metmuseum.org/art/collection/search/436529) | Vincent van Gogh | 496 × 624 | CC0 |
| [阿尔泰斯肖像](https://www.metmuseum.org/art/collection/search/436145) | Edgar Degas | 505 × 625 | CC0 |
| [花瓶旁的女子](https://www.metmuseum.org/art/collection/search/436121) | Edgar Degas | 600 × 488 | CC0 |
| [闹别扭](https://www.metmuseum.org/art/collection/search/436162) | Edgar Degas | 599 × 427 | CC0 |
| [舞蹈课](https://www.metmuseum.org/art/collection/search/438817) | Edgar Degas | 579 × 624 | CC0 |
| [舞台上的芭蕾排练](https://www.metmuseum.org/art/collection/search/436155) | Edgar Degas | 599 × 447 | CC0 |
| [古尔德小提琴](https://www.metmuseum.org/art/collection/search/503045) | Antonio Stradivari | 500 × 625 | CC0 |
| [掐丝珐琅瓶](https://www.metmuseum.org/art/collection/search/40606) | 作者未载 | 469 × 625 | CC0 |
| [布里欧修](https://www.metmuseum.org/art/collection/search/436946) | Edouard Manet | 599 × 486 | CC0 |
| [羽笔静物](https://www.metmuseum.org/art/collection/search/435904) | Pieter Claesz | 599 × 404 | CC0 |
| [凯风快晴](https://www.metmuseum.org/art/collection/search/36490) | Katsushika Hokusai | 599 × 413 | CC0 |
