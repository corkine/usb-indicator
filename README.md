# USB Keyboard Indicator

一个仅面向 Windows 的轻量小工具：USB Switcher 将目标键盘切到本机时，在主屏幕右上角显示一个不抢焦点、鼠标可穿透的绿色圆点；切走后圆点隐藏。

程序带有单实例保护；重复双击时，后启动的进程会直接退出。

## 使用

需要 Go 1.22 或更新版本：

```powershell
go build -ldflags "-H=windowsgui" -o keyboard-indicator.exe .
.\keyboard-indicator.exe
```

正式构建可以注入版本号，版本会显示在设置窗口中：

```powershell
go build -ldflags "-H=windowsgui -s -w -X main.version=v0.1.0" -o keyboard-indicator.exe .
```

首次启动会列出当前枚举到的键盘接口。选择目标 USB 键盘并保存后，程序进入常驻监听。配置保存在：

```text
%LOCALAPPDATA%\usb-indicator\config.json
```

打开图形设置窗口，可以重新选择键盘、设置圆点位置和大小：

```powershell
.\keyboard-indicator.exe --settings
```

支持八个预设位置，以及相对屏幕工作区左上角的自定义 X/Y 像素偏移；大小范围为 8–128px。保存后正在运行的指示器会立即更新，不需要重启。`--select` 仍作为兼容别名保留。

关闭正在运行的常驻指示器：

```powershell
.\keyboard-indicator.exe --disable
```

查看构建版本（需要保留控制台的诊断构建）：

```powershell
.\keyboard-indicator-console.exe --version
```

## CI 与发布

仅当推送 `v*` 标签时，GitHub Actions 才会运行测试、静态检查并构建带标签版本的 Windows amd64 程序。推送例如 `v0.1.0` 的标签后，会自动创建 GitHub Release，上传 EXE 和 SHA-256 校验文件。

## 工作方式

- 用 `RegisterDeviceNotification` 订阅键盘设备接口的到达/移除通知。
- 收到目标键盘的移除通知后立即隐藏圆点；随后通过 SetupAPI 延迟重检，校验最终状态。
- 收到设备到达通知后等待 Windows 完成枚举，再显示圆点。
- 设备通知不可靠时由轮询兜底：无论显示或隐藏状态均每 100ms 检查。
- 默认从所选设备路径提取 `VID_xxxx&PID_yyyy` 作为稳定匹配规则。
- 窗口使用 `TOPMOST / TOOLWINDOW / NOACTIVATE / TRANSPARENT` 扩展样式。

如果 USB Switcher 切换时设备管理器中的目标键盘始终不消失，这种基于设备枚举的判断方式将无法识别切换状态。
