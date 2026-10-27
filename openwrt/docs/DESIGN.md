# 实现路线

## 第一阶段：先跑通 x86

沿用 XDVPN 的 OpenConnect 连接思路，使用 OpenWrt 原生 netifd 接管进程生命周期和路由清理。
自定义连接事件脚本只发布明确选择的网段，防止服务器推送默认路由后意外接管全家流量。
OpenConnect 原生 `interface` 选项通过 netifd 为 VPN 网关创建 WAN 主机依赖，避免全局模式把 VPN 自己套进隧道。
需要实测多 WAN、VPN 网关域名变更和旁路由拓扑，不能仅凭这个接口假设永远不会套娃。

LuCI 写入系统 network 配置；rpcd ACL 的 UCI 权限粒度是整个 network 包，不是单个 section，故仅供管理员使用。
不增加自己的高频守护轮询；页面关闭后也没有状态轮询。TLS / DTLS 加解密的 CPU 成本仍由吞吐和硬件决定。
首版不盲目导入服务端 DNS 和分流域名，避免更改现有 DNS 服务。

## 第二阶段：心动域名 + 代理共存

XDial 的核心启发是让 DNS 和实际流量选择同一条线路；不移植整个 sing-box + Go 核心。

1. 获取实际公司域名、内网 DNS 和内网网段，确认 VPN 入口通过 WAN 解析及连接。
2. 公司域名通过 VPN 内网 DNS 查询；公司 DNS 自身有经隧道的路由。
3. 由 dnsmasq 按实际防火墙后端维护地址集合；fw4 使用 nftset，旧 fw3 使用 ipset。检查 dnsmasq 构建特性，不能默认为所有固件都支持。
4. 用独立路由表和带掩码的标记选择 VPN；启动前检查 mark、表号及优先级冲突，不覆盖其他软件的标记。
5. 与 Passwall/OpenClash 配置明确的公司流量直连/绕过透明代理规则；再由内核路由送进 VPN。**单独建立一个 nft 表或在自己的链里 return，不保证能绕过其他软件之后的 REDIRECT/TPROXY 链。**
6. OpenClash Fake-IP 下，公司域名须走真实解析并与 fake-ip-filter / DNS 策略配套；否则按真实 IP 建的 VPN 路由无法匹配客户端收到的假地址。
7. 检验插件重启、防火墙重载、DNS 缓存、TTL、连接跟踪及 IPv6 行为。保留对已有规则的精确归属和恢复记录。

共同开启时要覆盖 TCP、UDP、路由器自身与 LAN 转发，而不仅是 ping 成功。
不擅自关闭或重启其他代理；适配最终以用户的具体固件、插件版本及运行模式为准。

## 模式

| 模式 | 当前状态 | 说明 |
|---|---|---|
| 仅手动填写 | 已实现 | 只使用手动域名、IPv4 / 网段；其余交给原有网络 |
| 公司预设＋手动补充 | 已实现 | 本机 IT IP/域名清单加手动补充，不采用服务器全网路由；保护默认开启，可取消 |
| IPv4 全局 | 已实现，隔离验证 | 忽略内置 Anthropic 保护，但自定义黑名单仍优先；保留本地网络及 VPN 传输，阻断外部 IPv6 |
| 双栈全局 | 未实现 | 需要服务端 IPv6 能力 |

三种模式供 Hillstone 和 OpenConnect 共同使用。实机稳定性验证目前集中于 Hillstone；OpenConnect 的服务器主动断开问题仍未解决。

## 平台与打包

参考 [Are-u-ok](https://github.com/AUK9527/Are-u-ok) 的按固件系列和架构区分方式。

| 优先级 | 设备 | 常见软件包架构（必须现场确认） |
|---|---|---|
| 首测 | x86 虚拟机 | x86_64 |
| 后续 | NanoPi R2S / R4S | aarch64_generic |
| 后续 | GL.iNet MT3000 | aarch64_cortex-a53 |

LuCI/Shell 包自身为 `all`；架构相关的 OpenConnect 必须来自对应固件源。不是把 x86 的整套依赖改名后放到 ARM。
检查 `ubus call system board`、`/etc/openwrt_release`、包管理器架构列表、OpenConnect 版本、fw3/fw4 和 dnsmasq 特性。
首版以 24.10 接口为开发基线；22.03、23.05、25.12 及厂商固件是待测目标，不是已验证兼容清单。
opkg 发行 IPK，apk 发行对应 SDK 产物；不能仅更改文件扩展名。
