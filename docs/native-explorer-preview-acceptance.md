# Windows 文件快速查看

1. Canvepp 在 Windows 运行时，资源管理器的文件列表中按空格预览选中文件。检查取消选择、选中不同的标签页、按住空格、修饰键、重命名、搜索框以及切到其他应用。快捷键页可关闭此功能，关闭应用时释放监听。
2. 预览是一扇随当前主题变化的窗口。空格或 Esc 关闭，回到原资源管理器。实际检查图片完整画幅、文本编码与长文。PDF 和 Office 文件使用系统已注册的预览处理程序。其他文件可查看文件信息。
3. 在独立 Windows 资料库验证真实资源管理器与本地文件。原生回归覆盖读取上限、错误和关闭后的状态。保留现有画布和图库内的空格行为。

## Windows 11 验收

使用独立资料库从资源管理器打开 JPG、TXT 和 PDF，确认三种文件的实际内容均可见。空格和 Esc 均可关窗并回到原列表。已检查重命名、搜索、无选中文件和 Ctrl + 空格的触发范围。验收实例已退出。

在 Windows 上首次运行时启用此功能，开关位于「设置 → 快捷键 → 文件资源管理器 → 空格预览文件」。PDF 和 Office 文档使用系统注册的预览处理程序，本次确认了 PDF 的正文。超出读取上限的图片会显示提示，长文本会展示开头。

本次实际操作覆盖单个资源管理器标签页，多标签页切换、空格长按和切到其他应用待补实机验收。本地通过原生回归和竞态检测，macOS 与 Windows 均通过 Go 静态检查。Windows 实测截图和结果见 [图片](../artifacts/windows-quick-view/image.png)、[文本](../artifacts/windows-quick-view/text.png)、[PDF](../artifacts/windows-quick-view/pdf.png) 与 [运行记录](../artifacts/windows-quick-view/windows-check.json)。
