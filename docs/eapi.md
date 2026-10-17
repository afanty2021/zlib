# EAPI 接口

[README](../README.zh.md) · [配置](configuration.md) · [排障](troubleshooting.md)

部分 Z-Library 域名会向非浏览器请求返回 DiamWall 重定向循环，但这些域名的
移动端 EAPI 仍可能可用。CLI 直接读取结构化 JSON，不处理 DiamWall 挑战。

## 快速使用

```bash
# 先检查候选域名的 EAPI 健康状态
zlib doctor

# 显式指定域名
zlib login --domain https://z-lib.gd

# 或写入 shell / ~/.config/zlib/.env 后省略 --domain
export ZLIB_DOMAIN=https://z-lib.gd
zlib login

# 后续命令直接使用 EAPI
zlib search "golang"
zlib search "golang" --json
```

你也可以导入自己导出的 Netscape Cookie 文件：

```bash
zlib login --cookies ./cookies.txt --domain https://z-lib.gd
```

会话只保存服务端返回或导入的 Cookie，不保存密码。Cookie 文件和
`~/.config/zlib/session.json` 都应按账号密码同等保护。

## 当前范围

| 功能 | 状态 | 端点 |
| --- | --- | --- |
| 登录 | 支持 | `POST /eapi/user/login`；可导入 Cookie 文件 |
| 静态和交互式搜索 | 支持 | `POST /eapi/book/search` |
| 推荐图书 | 支持 | `GET /eapi/user/book/recommended` |
| 热门图书 | 支持 | `GET /eapi/book/most-popular` |
| 域名诊断 | 支持 | `GET /eapi/info/domains` |
| profile 账号资料及下载配额 | 支持 | `GET /eapi/user/profile` |
| history | 支持 | `GET /eapi/user/book/downloaded` |
| 已收藏（`favorites`） | 支持 | `GET /eapi/user/book/saved` |
| 图书详情 | 支持 | `GET /eapi/book/{id}/{hash}` |
| 下载链接 | 支持 | `GET /eapi/book/{id}/{hash}/file` |
| 下载后 Kindle 投递 | 支持 | EAPI 下载后使用本地 SMTP |

网页全文搜索没有已确认的 EAPI 对应端点；`FullTextSearch` 会明确返回不支持错误。

EAPI 搜索和历史结果会保留数字 ID 与 hash。命令行引用写成 `id:hash`：

```bash
zlib download 115066162:e9f13b
zlib download 115066162:e9f13b --send-to-kindle
zlib history --download 115066162:e9f13b --send-to-kindle
```

TUI 选中图书时按需请求 `/eapi/book/{id}/{hash}`，补充详情卡片并按
`id:hash` 缓存。Android 的 `ZLibBook` 模型明确包含 `edition`、`pages` 和
`identifier`；分类及独立的 ISBN 10/13 字段不能仅凭 APK 模型保证存在。
客户端只展示响应中可用的分类，且只从有效 ISBN 推导对应的编号。

## 登录端点的单独管控

Z-Library 把 `/eapi/user/login` 与 EAPI 的其余部分分开管控。2026-09-04 对一个真实
账号实测：该端点返回 `400 {"success":0,"error":"Authorization failed"}`，而同一账号
用 `/rpc.php` 下发的 key 访问 `/eapi/user/profile`、`/eapi/book/search`、
`/eapi/user/book/downloaded` 全部返回 `success:1`。z-lib.gd、z-lib.gl、
z-library.ec、article.sk、zlib.bz 五个域名表现一致，因此不是域名问题。

被拒的不是密码：密码错误时该端点返回的是 `Incorrect email or password`。

EAPI 请求使用 `remix_userid`/`remix_userkey`。若登录端点拒绝账号，CLI 会返回
服务端错误；可导入浏览器导出的 Netscape Cookie 文件，然后继续使用 EAPI。

## 域名白名单

CLI 只接受 `1lib.sk`、`article.sk`、`articles.sk`、`bookabooki.fi`、
`bookabooki.tw`、`lexlib.fi`、`lexlib.tw`、`librella.fi`、`librella.tw`、
`z-lib.fm`、`z-lib.gd`、`z-lib.gl`、`z-lib.sk`、`z-library.ec`、`zlib.bz` 和
`zliba.ru`。其中 librella/lexlib/bookabooki（.fi/.tw）是桌面端 App 发给捐赠者
的个人域名层（2026-10-17 在桌面端配置中观察到），CLI 与桌面端使用相同的 remix
cookie 认证，因此同样接受这些域名。这些域名的子域名也可以使用。配置值必须是
HTTPS 源站，不能包含端口、路径、查询或凭据。

2026-09-04 实测：`z-lib.gd`、`z-lib.gl`、`z-library.ec`、`article.sk`、
`articles.sk`、`zlib.bz` 的 EAPI 可用；`1lib.sk`、`z-lib.fm`、`z-lib.sk` 被
DiamWall 拦截；`zliba.ru` 已变为跳转到 `zlib.bz` 的纯 redirector，登录会跟随
并改用最终域名。`z-library.gs` 因 TLS 握手失败已从白名单移除。

意大利、法国或西班牙可尝试 `z-library.ec`。意大利、法国、西班牙、英国、
德国、印度、丹麦和中国的政府或 ISP 可能屏蔽这些域名。浏览器无法加载时，法国
以外的部分地区可以尝试更换 DNS。也可以使用 VPN，或通过 Tor Browser 访问
onion 站点。

新登录优先使用 `--domain`；省略时读取 `ZLIB_DOMAIN`。
如果两者都没有设置，命令会打印设置方法并退出。EAPI 登录还会检查
`/eapi/info/domains`；检查失败时，命令会提示运行 `zlib doctor` 选择
健康域名。若该检查跳转到了另一台主机，登录会改用最终域名，并对它单独做一次
白名单校验。其他命令优先使用环境变量，没有环境变量时才读取 session 域名 ——
遗留的 `ZLIB_DOMAIN` 因此会让刚刚登录成功的域名失效，是"登录成功但后续命令
报错"的常见原因。

## 稳定性

EAPI 没有公开、稳定的官方授权契约。接口定义参考
[baroxyton/zlibrary-eapi-documentation](https://github.com/baroxyton/zlibrary-eapi-documentation)
记录的移动端接口，开发时通过实时健康端点和搜索响应验证。上游可能修改字段、
鉴权方式或关闭域名。使用前应先运行 `zlib doctor`，不要依赖某个域名
长期可用。

## Android API 调查

[APK 初步分析](android-api-research.md)记录了逆向方法；
[深入调查与实测](android-eapi-deep-dive.md)对照 Android 接口声明、服务端返回和
当前 CLI 实现，列出了可扩展的命令。调查结论按文中的日期和测试范围理解。
