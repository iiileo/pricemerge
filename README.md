# PriceMerge

桌面工具：按品名更新基础 Excel 表中的 **出 / 接** 价格（大琪有效价），支持单条更新与批量导入。

技术栈：[Wails v3](https://v3.wails.io/) + Go + React。

## 功能

- **基础表**：自行选择模版 Excel（不再内置打包）
- **单条更新**：名称 + 出/接 + 价格
- **批量更新**：上传价格表 → 文件列表 → 查看预览 → 更新
- **结果撤回**：更新后可单条或全部撤回
- **名称映射**：其他表品名 → 基础表品名（可选）
- **导出**：写出更新后的基础表
- **操作日志**：本地记录；连点版本号 5 次开启调试模式

## 使用说明

1. 启动后先点 **选择**，打开基础表（需含「出 / 接」区块与「大琪」列）
2. 单条更新：输入名称、类型、价格后点 **更新**
3. 批量更新：选类型 → **上传文件** → **查看** 确认 → **更新**
4. 需要时加载名称映射表，再 **导出 Excel**

### 价格写入规则

右侧源数据布局：`品名 | 搬运工 | 大琪 | 差`

- 目标是改 **大琪有效价**，尽量不改搬运工
- 大琪有公式（`=搬运工-差`）时：只改「差」=`搬运工 - 新价`，保留公式
- 搬运工为空时按 `0` 处理

### 名称映射格式

Excel 两列：

| A 其他表名称 | B 基础表名称 |
| --- | --- |
| 软牡丹别名 | 软牡丹 |

首行含「名称」字样会当作表头跳过。

### 批量价格表

支持常见写法，例如：

- 两列：`名称 | 价格`
- 混写：`软牡丹133`、`软牡丹123-133`
- 同行多品、一格一品等

无价格的行不会提取；预览里会显示原价 / 新价 / 是否可匹配。

## 开发

### 环境

- Go 1.25+
- Node 22+
- [Wails CLI v3](https://v3.wails.io/)（本项目对应 `v3.0.0-beta.16`）
- [Task](https://taskfile.dev/)（可选，`wails3` 会调用）

```bash
go install github.com/wailsapp/wails/v3/cmd/wails3@v3.0.0-beta.16
```

### 常用命令

```bash
# 开发（热重载）
wails3 dev

# 当前平台编译
wails3 build

# 打包
wails3 package                         # → bin/PriceMerge.app
wails3 task darwin:package:dmg         # → bin/PriceMerge.dmg（可拖到「应用程序」安装）

# 测试
go test ./...
```

产物在 `bin/`。macOS 发布推荐使用 **DMG**：打开后把 `PriceMerge` 拖到 `Applications` 即可。

### 版本号

```bash
python3 scripts/set-version.py 1.2.3
```

会同步 `build/config.yml`、macOS plist、Windows `info.json`、前端版本显示。

## 发布

GitHub Actions：`.github/workflows/build-release.yml`

| 触发 | 说明 |
| --- | --- |
| 推送 tag `v*.*.*` | 例如 `git tag v1.0.0 && git push origin v1.0.0` |
| 手动 workflow_dispatch | 填写版本号 |

产物：

- `PriceMerge-{version}-darwin-arm64.dmg`（拖到「应用程序」安装）
- `PriceMerge-{version}-windows-amd64.exe`

并自动创建 GitHub Release。

## 目录结构

```
├── main.go / priceservice.go / workbook.go / extract.go
├── frontend/                 # React UI
├── build/                    # 图标、平台打包配置
├── scripts/set-version.py    # 版本同步
└── .github/workflows/        # CI 发布
```

## License

按仓库实际声明为准。
