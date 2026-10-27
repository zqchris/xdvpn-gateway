# XDVPN for OpenWrt

轻量的企业 VPN 插件，包含 LuCI 界面、Hillstone SCVPN 原生核心及可选的 OpenConnect 连接方式。
当前核心 **0.7.3-1**、LuCI 界面 **0.7.3-1**。核心已在 iStoreOS 24.10.6 x86_64 连接真实 Hillstone 服务，验证内网 HTTPS、模拟 LAN 转发与主动断开清理。
完整记录见 [Hillstone 验证](docs/HILLSTONE-VALIDATION.md)。

这是面向 OpenWrt 的独立实现，不替换上层目录中的华硕 Rust 守护程序或软件中心插件。

- 设计：[docs/DESIGN.md](docs/DESIGN.md)
- 验证：[docs/ACCEPTANCE.md](docs/ACCEPTANCE.md)、[ARM / WAN 测试](docs/PORTABILITY-VALIDATION.md)
- 开发入口：[AGENTS.md](AGENTS.md)、[交接说明](docs/CONTINUATION.md)

## 目录

| 路径 | 内容 |
|---|---|
| `core/hillstone/` | Go SCVPN 核心、证书验证、分流辅助与有界日志 |
| `package/` | LuCI、netifd、fw4 策略及 OpenWrt SDK 包定义 |
| `scripts/` | 开发 IPK 构建和协议诊断工具 |
| `tests/` | 界面、导入权限、生命周期与隔离网络测试 |
| `docs/` | 设计、验收、兼容性边界与发布说明 |

## 构建

需要 Python 3、Go 1.25+。在本 `openwrt/` 目录执行：

```sh
python3 scripts/build-ipk.py
python3 scripts/build-hillstone-ipk.py --arch x86_64
python3 scripts/build-hillstone-ipk.py --arch aarch64_generic
python3 scripts/build-hillstone-ipk.py --arch aarch64_cortex-a53
```

产物位于 `dist/`：LuCI 包为 `all`，核心包按架构标签区分。两个 ARM64 标签当前使用相同的通用 ARM64 核心，不代表针对具体路由器的优化或真机认证。以目标固件的 `opkg print-architecture` 为准。

这是开发 IPK 构建，不包含 GitHub Actions 自动发布，也不支持所有 OpenWrt 版本。SDK 构建需保留 `package/` 与 `core/` 的相对目录关系，安装匹配的 golang、packages、luci feeds；依赖从目标固件源安装，不混用系统库。

## 测试

需要 Node（界面测试），在本目录执行：

```sh
python3 -m unittest discover -s tests
node tests/test-luci.cjs
node tests/test-certificate-ui.cjs
(cd core/hillstone && go test -race ./...)
```

在具有 iproute2、nftables、ping 的 Linux 隔离测试环境以 root 执行：

```sh
sh tests/test-routing-netns.sh package/luci-app-xdvpn/root/usr/libexec/xdvpn/priority
```

该测试只创建临时 network namespace，模拟独立 WAN/LAN/VPN 出口，使用合成地址，退出时清理。它不等于完整 OpenWrt 固件或实际公司服务端验收，也不沿用华硕版 `lab/` 的测试数量和结论。

## 安装须知

开发 IPK 用于 `opkg` 固件；其他包管理器应使用对应 SDK 构建。
先查看 `opkg print-architecture`，选择匹配的核心包。ARM64 已交叉构建，尚未真机验收。

```sh
opkg install /tmp/xdvpn-hillstone_0.7.3-1_x86_64.ipk /tmp/luci-app-xdvpn_0.7.3-1_all.ipk
```

升级到 0.7.3 时，已有 netifd 可能仍缓存旧协议参数。请在允许短暂断网时重启网络服务或路由器，加载新的参数声明；安装脚本不会自动执行。加载前，修改策略仍可能触发重连。

首次安装新协议后，检查 `ubus call network get_proto_handlers` 中是否包含 `xdvpn_hillstone`。
若没有，需要在可短暂断网时重启网络服务；仅 reload 不一定注册新协议。
安装脚本本身不会主动重启网络或连接 VPN。

打开「服务 → XDVPN」，选择 Hillstone SCVPN，填写实际服务器、端口、账号密码，以及上网接口。证书无需手填指纹。
旁路由通常选择 `lan`，不要假定必须存在 `wan`。
首次建议选择「仅手动填写」，填写完整公司服务域名，保存并应用后点击连接。修改分流模式、域名/IP 或保护开关后保存并应用立即刷新；服务器、账号或核心变更仍需重新连接。
自签名证书会显示首次信任对话框；核实后确认，再保存并应用。已信任的旧版指纹会迁移并绑定到原服务器。
域名最多 64 个，每个最多采用 32 个 IPv4，总计最多 512 个去重地址；解析失败暂留旧结果，超过十分钟移除。此模式无断线阻断。

## 私有规则

在“公司预设 + 手动补充”模式下，可选择 UTF-8 文本文件，点击“导入并替换预设”。只发送到当前路由器，不上传公共仓库。服务端校验后以 0600 权限原子替换本地清单；格式错误保留原文件，手动补充项不变。导入成功后通知当前连接热刷新，按已保存的模式生效，不自动切换模式。页面尚未保存的模式更改仍需保存并应用。导入功能需配套 0.7.3 核心和 LuCI。

私有清单与代码、安装包分离，路由器固定路径为 `/etc/xdvpn/company-rules.txt`，每行一个 IPv4、CIDR 或域名（父域名包含子域名），可含空行或 `#` 注释；兼容中文逗号、分号、CRLF 和 BOM。IP 自动按 /32 处理，网段规范化并去重。最多 4096 个 IP/网段、256 个预设域名，文件最多 256 KiB。
设置权限 `0600`，选择「公司预设 + 手动补充」后生效。界面展示条目数量；内容不写入公开源码或安装包。更新文件后点击保存并应用即可重载，无需重连。手动模式完全不读取预设；旧手动模式主动启用私有清单的配置迁移为公司预设模式。
公司预设模式下文件缺失、为空或格式错误时拒绝提交路由，不回退到全局；不要加入覆盖管理 LAN 的网段。
升级保留该目录，但它会进入路由器系统备份，因此备份也需要私密保存。

公开构建不读这个文件，不嵌入公司网段。不要将私有文件、密码或完整 `uci show network` 输出提交到 Git。

## 界面与日志

顶部连接总览展示状态、核心、服务器和流量模式；「连接设置 / 日志与诊断」切换。连接账号设置置顶，未填完整时展开，已填完整时默认折叠，随后展示分流模式、保护开关和批量白名单。域名和 IP 分别按每行一项填写，兼容已有 UCI 列表。界面适配桌面与手机，并跟随系统明暗主题。

日志按需读取最多 100 条系统事件及 100 组 Hillstone 实际访问记录。访问记录包含本地日期时间、可识别的域名、目标 IP；按完整子域名分别显示，同一域名合并，首次访问起 5 分钟内合为一笔，之后访问另记。持续连接的最近时间最多每 10 秒更新一次，每组最多展示 8 个域名与 8 个 IP。记录仅在核心内存，重连清空，不写闪存、不新增轮询任务。

访问证据来自成功交给 Hillstone UDP 传输的出站数据，不代表网站响应成功。只解析有界的 TLS SNI / HTTP Host；ECH 外层域名单独标注，不能视为真实访问域名；QUIC、乱序或超出解析上限时显示具体 IP，不凭共享 IP 猜域名。最多 1024 个流元数据、每流 16 KiB 临时缓冲，未完成解析的有效缓冲总量限制 1 MiB；这是轻量观察记录，不是完整审计。OpenConnect 当前仅显示系统事件，未采集访问记录。

「域名去向」按需解析 IPv4，并读取实际运行的 nft 集合，区分命中 VPN、保护排除、交还原网络。它是当前规则判断，不是历史连接或出口成功证明；不发送网站 HTTP 请求。系统日志重启后可能清空，页面隐藏时停止状态查询。

## 当前功能与边界

连接总览提供“检查分流优先级”：检查 XDVPN 入口和策略路由，对其他独立入口或更早策略规则提示需确认。它不等于 DNS、Fake-IP/TUN 或实际访问兼容性保证；不新增后台轮询。

- Hillstone 自动检查证书：有效 CA 证书验证主机名及证书链；自签名证书首次确认后按完整证书指纹校验，绑定服务器地址和端口。
- 查看证书只进行 TLS 握手，不发送 VPN 账号密码。证书变更时需要重新核实。
- Hillstone 和 OpenConnect 共用三种分流模式：仅手动填写、公司预设＋手动补充、IPv4 全局。
- 仅手动填写：只接管填写的域名、IPv4 / 网段；其他目标交给原有代理或默认网关。域名包含自身和全部子域名，通过路由器 DNS 查询自动加入分流；保存并应用后事件通知核心立即加载，无需重连。
- 公司预设＋手动补充：读取路由器 `/etc/xdvpn/company-rules.txt` 中由 IT 维护的 IP/网段/域名，再叠加界面手动填写的域名和 IP；其余交还原网络。当前服务器下发的是全网路由，不用于内网判定。旧 `server` 模式迁移为 `company`，不会再使用服务器下发的默认路由。
- Anthropic / Claude 保护默认开启，手动和公司预设模式可主动取消；内置域名后缀并只读复用 OpenClash 相关 DOMAIN / DOMAIN-SUFFIX 规则。
- IPv4 全局：主动选择后忽略 Anthropic 保护，所有非本地 IPv4 目标走公司 VPN；保留管理网络和 VPN 连接本身，阻断外部 IPv6，避免绕行。尚不支持 IPv6 VPN。
- 公司目标在 fw4 透明代理入口优先处理；其他流量保留原流程，防火墙重载事件恢复规则，5 分钟只读兜底检查，异常才修复。不覆盖系统 DNS，默认不开机连接。
- 独立 VPN 防火墙区域，LAN → VPN 转发、NAT/MSS；netifd 管理生命周期。
- 数据健康检查，而非仅依据进程存活或 TLS 控制连接状态判断。
- OpenConnect 方式保留，但测试部署中仍有服务器主动断开的未解决问题。

当前 OpenClash 配置下已验证访问；**不代表全部 Passwall/OpenClash 模式均兼容**。
公司预设仍需管理员提供，不自动猜测全部公司服务。只新增 XDVPN 自有防火墙规则，不改 OpenClash 的配置、订阅和节点。
父域名按 DNS 后缀边界匹配，例如 example.com 包含 console.example.com，不包含 fakeexample.com。发现的 IPv4 最多 4096 个，保留一小时；共享 IP 可能一并分流。域名解析沿用路由器现有 DNS；仅添加自有 dnsmasq nftset 配置，不改 DNS 上游。自带 DoH/DoT、历史缓存、共享 CDN、未知新域名或二级代理无法仅凭 IP 集合完整识别，不能将当前测试表述为永久全覆盖保证。
IPv6 VPN、交互式 SSO/MFA 尚未实现。当前验证环境为 fw4 + OpenClash redir-host；Passwall / Fake-IP / 其他独立拦截基础链尚未验证。
开启域名保护时需要 dnsmasq 的 nftset 支持，缺失时拒绝连接；保护初始化失败或运行检查失败也会停止 VPN。
公司目标断线后仍可能回到原网络，未提供公司业务的断线阻断。Anthropic 禁止经 VPN 的出口保护独立于这一行为。

## 断开与卸载

先断开接口、关闭开机连接，再卸载；配置和私有清单保留。

```sh
ifdown xdvpn
uci set network.xdvpn.auto=0
uci commit network
opkg remove xdvpn-hillstone luci-app-xdvpn
```

彻底清理前确认网络和防火墙 section 的 `xdvpn_owned=1`，仅删除本插件拥有的配置；不要清空路由表或防火墙。

## 资料与许可

- [Hillstone 核心](core/hillstone/README.md)
- [断线调查与协议对照](docs/DISCONNECT-ANALYSIS.md)
- [验收记录](docs/ACCEPTANCE.md)
- [平台与共存设计](docs/DESIGN.md)
- [发布前隐私检查](docs/PUBLISHING.md)

`core/hillstone` 与其核心包采用 GPL-3.0-only；LuCI、其他自有脚本采用根目录 MIT 许可。
OpenConnect 及其他依赖遵循各自许可证。


## 自定义黑名单（0.6.0）

分流设置可折叠，Claude 保护范围下方提供“其他黑名单”，每行一个域名、IPv4 或 CIDR，最多 256 项。父域名包含子域名。自定义排除独立于 Claude 开关，在全部模式（包含全局）优先；交还原网络由 OpenClash / Passwall 继续处理，不改其他代理的配置。保存并应用即时刷新，移除条目会清理相应集合。域名排除依赖路由器 DNS；共享 IP 会一并排除，客户端加密 DNS 或缓存可能影响覆盖，已知 IP 可以直接补充。IPv6 仍不能进入公司隧道。

日志按具体域名搜索，不再把兄弟子域名归到父域名。仅 IP 的记录标题包含实际 IP。连接缓存扩充为有界的元数据缓存，减少繁忙时丢失 SNI；并明确区分普通 SNI 与 ECH 外层名称。
