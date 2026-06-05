# jczhl Filyme Launcher

一个 Windows 专用的 Flyme / Niagara 风格 A-Z 字母启动器。

界面默认只显示一列字母表，鼠标移到字母上时向左展开对应文件列表，点击即可打开文件、文件夹或快捷方式。

## 特性

- Windows-only
- Go + Wails + Vue + pnpm
- 右侧 A-Z 字母表
- 只显示有内容的字母
- 鼠标聚焦字母时向左展开气泡列表
- 点击列表项打开，不会误触滑动打开
- 气泡最多显示约 4 项，超过后内部滚动
- 只扫描所选目录当前层，不递归子目录
- 支持桌面目录中的：
  - 文件夹
  - 普通文件
  - `.lnk` 快捷方式
  - `.exe` 程序
  - URL / 脚本等 Windows 可打开文件
- 懒加载 Windows 关联图标
- Acrylic / 毛玻璃透明窗口
- 未聚焦时窗口宽高只包住字母表
- 可拖动窗口
- 自动保存窗口位置，下次启动恢复
- 配置与缓存保存到 `~/.jczhl-filyme`

## 数据目录

程序会在用户目录下创建：

```text
~/.jczhl-filyme
```

主要文件：

```text
config.json        # 保存所选目录和窗口位置
items.json         # 最近一次扫描结果
icons/             # Windows 图标 PNG 缓存
```

## 开发环境

需要：

- Windows
- Go
- Node.js
- pnpm
- Wails CLI

安装 Wails CLI：

```powershell
go install github.com/wailsapp/wails/v2/cmd/wails@latest
```

## 安装依赖

```powershell
pnpm install
```

## 开发运行

```powershell
pnpm wails:dev
```

## 构建

```powershell
pnpm wails:build
```

构建产物：

```text
build/bin/jczhl-filyme-launcher.exe
```

## 使用方式

1. 首次运行会要求选择一个目录。
2. 建议选择桌面目录或你整理快捷方式的目录。
3. 后续启动会直接读取保存的目录，不再弹出选择。
4. 鼠标移动到字母上，左侧展示该字母下的文件列表。
5. 点击列表项打开。
6. 点击字母表上方的 `↻` 刷新。

## GitHub Actions

已提供 `.github/workflows/release.yml`。

触发方式：

- 推送到 `main` / `master`
- 手动触发 `workflow_dispatch`

Action 会：

1. 安装 Go / Node / pnpm / Wails
2. 构建 Windows exe
3. 使用当前提交 hash 前 8 位作为 tag
4. 创建或更新 GitHub Release
5. 上传 `build/bin/jczhl-filyme-launcher.exe`

示例 tag：

```text
a1b2c3d4
```

## 备注

本项目只支持 Windows。图标提取和文件打开逻辑依赖 Windows / PowerShell / WebView2。