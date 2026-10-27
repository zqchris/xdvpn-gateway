# xdvpn-gateway（公司专线）

[XDVPN](https://github.com/zqchris/XDVPN) 的路由器版本。华硕 RT-AX86U（koolcenter 软件中心，hnd 平台）插件：路由器替全家拨通公司 OpenConnect 专线，任何环节出问题自动回到运营商直连。取代旧版 [xdvpn-router](https://github.com/zqchris/xdvpn-router)。

- 设计：[docs/DESIGN.md](docs/DESIGN.md)
- 沙盒演练结果：[docs/LAB-RESULTS.md](docs/LAB-RESULTS.md)
- 状态：0.1.0，本机沙盒 29 个场景、222 项检查全部通过；已装在作者家里的 RT-AX86U 上使用。

## OpenWrt 版本

新增独立的 [OpenWrt 插件](openwrt/README.md)：LuCI 界面、Go Hillstone SCVPN 核心及可选 OpenConnect 接入，提供手动白名单、私有公司预设和 IPv4 全局模式。

- 状态：0.7.3，x86_64 已验证 Hillstone 连接；ARM64 已构建并运行 Linux 测试，路由器真机仍需补测。
- 设计：[openwrt/docs/DESIGN.md](openwrt/docs/DESIGN.md)
- 验证与边界：[openwrt/docs/ACCEPTANCE.md](openwrt/docs/ACCEPTANCE.md)、[ARM / WAN 测试](openwrt/docs/PORTABILITY-VALIDATION.md)
- OpenWrt 版独立维护自有分流规则，不修改其他代理配置；仅在已测试的组合下验证共存，不代表全部 OpenClash/Passwall 模式兼容。
- 以下华硕构建、测试与安装说明仍适用于原版；OpenWrt 的依赖、IPK 构建与安装见独立文档。两者不是同一个核心，也不共享安装包。

## 目录

| 路径 | 内容 |
|---|---|
| `corpline/` | 守护程序（Rust，只依赖 libc，静态 aarch64 约 0.65 MB）：状态机、DNS 转发器、路由/防火墙、openconnect 管理 |
| `plugin/corpline/` | 软件中心插件：安装/卸载脚本、外壳脚本 `corpline_config.sh`、网页 `Module_corpline.asp`、图标 |
| `lab/` | 沙盒：Linux 虚拟机里用网络命名空间模拟家庭网络、运营商、公司 VPN（ocserv）与软件中心；`run.sh` 逐项故障演练 |
| `lab/web/` | 网页的本地预览与交互检查（模拟 dbus / 状态文件） |
| `docs/` | 设计文档与演练结果 |
| `openwrt/` | OpenWrt LuCI 插件、Hillstone 核心、IPK 构建与独立测试（核心 GPL-3.0-only，其余自有代码 MIT） |

## 构建

在装有 Rust 与 `aarch64-unknown-linux-musl` 目标的 Linux（例如沙盒虚拟机）上：

```sh
bash build.sh        # 生成 corpline.tar.gz（软件中心离线安装包）
```

## 沙盒测试

```sh
sudo bash lab/setup.sh             # 搭沙盒网络（只在虚拟机的网络命名空间里）
sudo bash lab/run.sh               # 全部场景，约 45 分钟；结果写入 lab/report/RESULTS.md
sudo bash lab/run.sh crash notify  # 只跑指定场景
sudo bash lab/mem_cn.sh <名单>     # 用真实国内服务名单测内存与解析
sudo bash lab/teardown.sh          # 拆掉沙盒
```

## 安装须知

安装后默认**不启用、不改任何网络设置**；在插件页填好公司账号再打开开关。需要 Entware 的 `openconnect`（`opkg install openconnect`）。关闭或卸载会撤掉全部改动，恢复到安装前。

家里某台设备要自己再开公司 VPN（例如 Mac 上的 XDVPN）时，请在「家里的设备」里把它设成「直连」：公司 VPN 域名对不同 DNS 会给出不同入口地址，不加的话这台设备的 VPN 可能被套进路由器的专线里，变慢或连不上。

**安全防护**默认开启：外网有人扫描路由器端口、反复连端口转发里的管理端口（像猜密码）、短时间大量连接，自动封锁 24 小时；公开恶意地址名单里的地址直接挡掉。家里设备连恶意地址或异常外连只提醒、不拦截。猜密码和家里设备异常会按「异常通知」推送。误封可在页面「正在封锁的地址」里解封；公司出口等常用地址加进「设置 › 安全防护」的白名单。防护只加拦截不加放行，不影响游戏和 BT。

只想让某台设备的个别程序直连、其余仍走公司线路时（例如 NAS 上的 BT 下载和 Plex），在「家里的设备」里保持「公司线路」，点这一行的「直连端口」，填程序用的本机端口或端口段（如 `32400, 3981, 3982-4081`）。从这些端口发出的连接（TCP 和 UDP）直连；程序主动访问的网站另在「网站单独设置」里设为直连。
