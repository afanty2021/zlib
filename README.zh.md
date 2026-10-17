<div align="center">

<img src="docs/logo.svg" alt="zlib" width="160"/>

# zlib

Z-Library 命令行工具，搜索、下载图书并发送到 Kindle，你和 Claude Code、Codex 等 AI Agent 都能用。

<a href="go.mod"><img src="https://img.shields.io/github/go-mod/go-version/heartleo/zlib" alt="Go version" height="20"/></a> <a href="https://github.com/heartleo/zlib/actions/workflows/ci.yml"><img src="https://img.shields.io/github/actions/workflow/status/heartleo/zlib/ci.yml" alt="CI" height="20"/></a> <a href="https://github.com/heartleo/zlib/releases/latest"><img src="https://img.shields.io/github/v/release/heartleo/zlib" alt="Release" height="20"/></a> <a href="https://github.com/heartleo/zlib/releases"><img src="https://img.shields.io/github/downloads/heartleo/zlib/total" alt="Downloads" height="20"/></a> <a href="LICENSE"><img src="https://img.shields.io/badge/license-MIT-green" alt="License" height="20"/></a>

<a href="https://trendshift.io/repositories/135168?utm_source=repository-badge&amp;utm_medium=badge&amp;utm_campaign=badge-repository-135168" target="_blank" rel="noopener noreferrer"><img src="https://trendshift.io/api/badge/repositories/135168" alt="heartleo%2Fzlib | Trendshift" width="250" height="55"/></a>

[English](README.md) | 简体中文

[安装](#安装) · [快速开始](#快速开始) · [终端](#终端) · [常用命令](#常用命令) · [Agent 接入](#agent-接入) · [文档](#文档)

</div>

![终端搜书演示](docs/demo-search.gif)

## 功能

🔍 **搜索** — 按年份、语言和格式筛选，结果可排序\
📚 **浏览** — 收藏、推荐、热门图书和下载历史\
📥 **下载** — 查看书籍详情，选择下载目录\
📖 **发送到 Kindle** — 把文件投递到你的 [Kindle 邮箱](docs/kindle.md)\
🕒 **用量查看** — 查看每日下载配额\
🎨 **主题** — 用 `/theme` 切换配色\
🌐 **网络** — 用 `zlib doctor` 找到可用镜像，也支持代理\
🤖 **AI Agent 集成** — 通过 [Agent 插件](#agent-接入)集成 Claude Code 和 Codex

## 安装

macOS / Linux，使用 Homebrew：

```bash
brew install heartleo/tap/zlib
```

macOS / Linux，使用安装脚本（安装到 `/usr/local/bin`，可能需要 `sudo`）：

```bash
curl -fsSL https://raw.githubusercontent.com/heartleo/zlib/main/install.sh | sh
```

Windows，使用 winget：

```powershell
winget install --id heartleo.zlib --exact
```

也可以从 [Releases](https://github.com/heartleo/zlib/releases/latest) 下载程序，
放入 `PATH` 目录。Go 安装、源码编译和更新方式见[安装指南](docs/installation.md)。

## 快速开始

使用前需要一个 Z-Library 账号。先检测当前网络可用的域名：

```sh
zlib doctor
```

选择结果中的域名，将下方示例 URL 替换为检测结果中的地址。
运行后在表单中输入邮箱和密码：

```sh
zlib login --domain https://z-lib.gd
zlib tui
```

按 `/`，选择 `/search`，输入书名、作者或 ISBN，按 `Ctrl+S` 搜索。
也可以直接输入 `/search dune` 后按 `Enter`。选中图书后按 `Enter`，确认后开始下载。

默认下载到当前目录，也可以在下载前选择其他目录。
登录状态保存在本地，后续直接运行 `zlib tui` 即可。

没有可用域名或登录失败时，参阅[排障指南](docs/troubleshooting.md)。
已有浏览器会话也可以通过[导入 Cookie](docs/troubleshooting.md#login-returns-authorization-failed)登录。

## 终端

`zlib tui` 打开一个常驻的终端界面。每个页面底部都会列出可用按键，
第一次使用记住下面这些就够了：

| 按键 | 作用 |
| --- | --- |
| `/` | 打开命令列表：搜索、收藏、历史、账号、设置、主题 |
| `↑/↓` 或 `j/k` | 选择图书 |
| `←/→` | 上一页或下一页结果 |
| `Enter` | 下载选中的图书（先确认） |
| `Ctrl+F` | 修改当前搜索的查询词和过滤条件 |
| `Esc` | 返回 |
| `/quit` | 退出 |

设置在 `/config` 下（网络、下载、Kindle）。全部页面和按键见[终端界面说明](docs/tui.md)。

## 常用命令

带查询词时输出表格，加上 `--json` 则输出供脚本使用的 JSON：

```sh
zlib search "dune" --ext epub
zlib search "dune" --ext epub --json --count 10
```

| 操作 | 命令 |
| --- | --- |
| 浏览收藏 | `zlib favorites` |
| 浏览推荐 | `zlib recommended` |
| 浏览热门图书 | `zlib most-popular` |
| 浏览下载历史 | `zlib history` |
| 查看账号与配额 | `zlib profile` |
| 配额用尽时轮转账号 | `zlib accounts add`、`zlib accounts switch` |
| 发送到 [Kindle](docs/kindle.md) | `zlib kindle send ./book.epub` |

下载时，将 `id:hash` 替换为结果中书籍的完整 `id`：

```sh
zlib download "id:hash"
```

运行 `zlib <command> --help` 查看参数。
过滤条件、分页和下载选项见[使用指南](docs/usage.md)。

## Agent 接入

安装插件即可。没有 `zlib` 时，Agent 首次使用会自行安装。
登录请自己运行 `zlib login`，密码不要发到对话里。

### Claude Code

在 Claude Code 中执行：

```text
/plugin marketplace add heartleo/zlib
/plugin install zlib@zlib-plugin-cc
```

### Codex

在终端执行：

```sh
codex plugin marketplace add heartleo/zlib
codex plugin add zlib@zlib
```


> 用 zlib 搜索 The Pragmatic Programmer 的 EPUB 版本，先列出结果，再决定下载。

## 多账号轮转

每个 Z-Library 账号有独立的每日下载配额。配置多个账号后，`download`
（以及所有其他下载路径——搜索回车下载、`history --download`）会自动轮转：
服务器返回每日限额错误时，CLI 把该账号标记为用尽，换下一个还有额度的
账号重试。轮转顺序是当前账号优先，其余按存储顺序；每次下载每个账号
最多尝试一次。

账号保存在 `~/.config/zlib/accounts.json`，当前账号镜像到 `session.json`，
因此单账号场景的行为与从前完全一致。

```bash
zlib accounts add main --user-id 123 --user-key abc --domain https://zlib.bz
zlib accounts add alt                    # 交互式登录，参数与 `zlib login` 相同
zlib accounts import backup              # 把当前 session.json 的登录收纳为账号
zlib accounts list                       # 表格，含各账号的缓存配额
zlib accounts switch backup              # 手动切换
zlib accounts remove backup              # 删除一个账号
zlib accounts clear                      # 删除全部（当前登录保留）
```

`--user-id`/`--user-key` 直接录入 `remix_userid`/`remix_userkey` 凭证对——
与 Z-Library 桌面端 App 存在其 `config.json` 里的完全相同——不需要发起
任何网络请求。

配额记账不产生额外 API 调用：下载成功把缓存剩余数减一，限额错误置零；
所有账号看起来都用尽时，CLI 会向服务器重查一次（跨天配额重置也靠这个
发现）。`zlib profile` 会顺带刷新当前账号的缓存。所有账号真正用尽时，
下载报错并列出各账号的剩余数量。

注意：

- 某个账号会话过期不会触发轮转；请 remove 后重新 add 该账号完成重新登录。
- 设置了 `ZLIB_DOMAIN` 时，它仍然覆盖所有账号自己的域名。

## 文档

| 指南 | 内容 |
| --- | --- |
| [安装指南](docs/installation.md) | 安装方式、源码编译、`zlib update` |
| [使用指南](docs/usage.md) | 搜索、下载、收藏、历史 |
| [终端界面](docs/tui.md) | 命令和快捷键 |
| [配置指南](docs/configuration.md) | 域名、代理、目录、配置路径 |
| [主题指南](docs/themes.md) | 配色列表和预览操作 |
| [Kindle](docs/kindle.md) | SMTP 配置与发送 |
| [排障指南](docs/troubleshooting.md) | 登录和网络问题 |

遇到问题请在 [Issues](https://github.com/heartleo/zlib/issues) 附上
`zlib version`、操作系统、所用命令和报错。请勿附带密码、Cookie 或会话文件。
安全漏洞请按 [SECURITY.md](SECURITY.md) 私下报告，不要公开提 issue。

## Star History

<a href="https://www.star-history.com/?repos=heartleo%2Fzlib&type=date&logscale=&legend=top-left">
 <picture>
   <source media="(prefers-color-scheme: dark)" srcset="https://api.star-history.com/chart?repos=heartleo/zlib&type=date&theme=dark&logscale&legend=top-left" />
   <source media="(prefers-color-scheme: light)" srcset="https://api.star-history.com/chart?repos=heartleo/zlib&type=date&logscale&legend=top-left" />
   <img alt="Star History Chart" src="https://api.star-history.com/chart?repos=heartleo/zlib&type=date&logscale&legend=top-left" />
 </picture>
</a>
