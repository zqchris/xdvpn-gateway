# 上游参考

以下为开发时核对过的参考版本；上游克隆和调查材料保存在仓库外，不随源码或安装包发布。

| 仓库 | 本地版本 | 用途 |
|---|---|---|
| [zqchris/XDVPN](https://github.com/zqchris/XDVPN) | `7baab7a042649b057e2b0f1037aac78c9bff957d` | OpenConnect 登录、显式分流、生命周期设计 |
| [kafeifei/XDial](https://github.com/kafeifei/XDial) | `a52390d24f7dc29a6e2f682a4e62c1ccdd5ffe85` | DNS 与出口一致的架构；不直接移植完整 Go / sing-box 核心 |
| [zqchris/xdvpn-gateway](https://github.com/zqchris/xdvpn-gateway) | `25d74e259fcbb824f925770386b6efa45b9b787b` | 华硕版现有网关与故障处理参考，硬件/平台接口不同 |
| [zqchris/xdvpn-router](https://github.com/zqchris/xdvpn-router) | `6504bf120ba614d2bf66076cb150d519b0453346` | 旧版策略路由方案参考，作者已用 gateway 替代 |

当前新增 OpenWrt 代码独立实现，没有复制华硕插件或 macOS GUI 源码，没有分发上游二进制。
AnyConnect 方式使用系统 OpenConnect；Hillstone SCVPN 则使用本项目的 Go 核心。
SCVPN 协议参考 LionNatsu/hilldust @542528d，独立 ESP 测试向量参考 NolanHo/hilldust。
该核心及核心包采用 GPL-3.0-only，具体说明见 [核心文档](../core/hillstone/README.md)；LuCI 和其他自有脚本采用 MIT。

官方接口核对：

- [OpenWrt 24.10 OpenConnect netifd adapter](https://github.com/openwrt/packages/blob/openwrt-24.10/net/openconnect/files/openconnect.sh)
- [OpenWrt vpnc-script](https://github.com/openwrt/packages/blob/master/net/vpnc-scripts/files/vpnc-script)
- [LuCI OpenConnect 协议界面](https://github.com/openwrt/luci/blob/master/protocols/luci-proto-openconnect/htdocs/luci-static/resources/protocol/openconnect.js)
- [OpenWrt 24.10 IPK 格式](https://github.com/openwrt/openwrt/blob/openwrt-24.10/scripts/ipkg-build)
- [Are-u-ok 平台划分](https://github.com/AUK9527/Are-u-ok)
