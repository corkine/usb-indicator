# USB Keyboard & Mouse Indicator

一个仅面向 Windows 的轻量小工具：目标键盘切到本机时显示绿色圆点，Logitech M585/M590 鼠标切到本机时显示蓝色圆点。两个状态彼此独立，窗口不抢焦点且鼠标可穿透。

程序带有单实例保护；重复双击时，后启动的进程会直接退出。

当前支持范围是一把可枚举的 HID/USB 键盘，以及一只通过蓝牙和 Logitech Unifying 在两台电脑间切换的 M585/M590。Bolt、普通 2.4G 接收器及多鼠标配置暂不在支持范围内。

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

首次启动会列出当前枚举到的键盘和受支持鼠标。选择设备并保存后，程序进入常驻监听。配置保存在：

```text
%LOCALAPPDATA%\usb-indicator\config.json
```

打开图形设置窗口，可以重新选择键盘和鼠标、设置圆点位置、大小和检测间隔：

```powershell
.\keyboard-indicator.exe --settings
```

支持八个预设位置，以及相对屏幕工作区左上角的自定义 X/Y 像素偏移；大小范围为 8–128px。检测间隔可选快速 100ms、平衡 200ms（默认）或低频 500ms。保存后正在运行的指示器会立即更新，不需要重启。`--select` 仍作为兼容别名保留。

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
- 设备通知的延迟无界且不确定，因此由可配置轮询提供程序侧的延迟上限；默认无论显示或隐藏状态均每 200ms 检查一次。设备通知只在及时到达时作为加速。
- 默认从所选设备路径提取 `VID_xxxx&PID_yyyy` 作为稳定匹配规则。
- M585/M590 蓝牙连接通过实际 HID 鼠标接口的到达与移除判断；Windows 保留的配对设备缓存不作为在线依据。
- Unifying 接收器始终保持 USB 枚举，因此通过 HID++ 连接状态查询判断当前是否有鼠标链路；查询结果根据检测间隔缓存 300ms、500ms 或 1s，避免频繁访问接收器。
- 绿点代表键盘，蓝点代表鼠标；仅显示当前连接到本机的设备对应圆点。
- 窗口使用 `TOPMOST / TOOLWINDOW / NOACTIVATE / TRANSPARENT` 扩展样式。

如果 USB Switcher 切换时设备管理器中的目标键盘始终不消失，这种基于设备枚举的判断方式将无法识别切换状态。
