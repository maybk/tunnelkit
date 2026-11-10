# 图标与品牌资源 (`assets`)

管理应用图标与菜单栏/系统托盘图标的 SVG 矢量母版及 Go 运行时内嵌 PNG 资产。

## 目录结构

- `brand/`：浅色/深色应用图标、透明品牌图形与 macOS 菜单栏模板图标的 SVG 矢量母版。
- `icons/`：通过 `main.go` 内嵌至二进制的桌面窗口图标与三端托盘运行时 PNG 图标。

## 开发规范

- **安全区约束**：`icons/app-icon.png`（`256×256`）主体安全区为 `206×206`（对应 Apple HIG `824/1024`），`icons/tray-icon.png`（`64×64`）单色 Alpha 主体为 `52×52`，`icons/tray-icon-color.png`（`64×64`）全彩主体为 `56×56`。
- **派生同步**：修改 `brand/` 母版后，需同步更新 `icons/`、`frontend/public/favicon.svg` 与 `build/` 下的平台图标产物。

## 验证

```bash
go test -race ./internal/app/...
```
