# 开发与交接

先读根目录 AGENTS.md 和 README.md。当前核心版本为 0.7.3-1，LuCI 构建版本为 0.7.3-1。
本文件只保留通用开发说明；部署地址、凭据、公司清单、现场日志及原始历史备份均不属于公开仓库。
部署权限不随此文档传递，操作实际设备前应确认当前任务的授权范围。

## 代码入口

- `core/hillstone/`：SCVPN 核心、证书验证、公司预设、保护规则与访问记录。
- `core/hillstone/openconnect_linux.go`：OpenConnect 共用策略辅助。
- `package/luci-app-xdvpn/root/usr/libexec/xdvpn/`：自有 nft/PBR 策略与刷新。
- `package/luci-app-xdvpn/htdocs/luci-static/resources/view/xdvpn/`：原生 JS/CSS 界面。
- `scripts/build-*.py`：开发 IPK 构建；`package/*/Makefile`：SDK 入口。
- `tests/`：界面、证书、脚本、打包测试；核心测试位于 Go 目录。

## 开发约束

保存分流规则通过现有核心热更新，不重连。只维护本插件的策略，不修改其他代理的配置或订阅。
私有规则由用户单独部署到 `/etc/xdvpn/company-rules.txt`，权限 0600，不能纳入源码或安装包。
真实浏览器验证应使用隔离会话，不读取个人浏览器配置或 cookies。

UI-only 部署需同时更新版本化 JS/CSS 入口及菜单 action，确保符号链接可读取，并清理实际的 LuCI dispatcher 缓存。
当前入口为 `main_v073.js` 和 `style_v073.css`。完整安装包升级可能停止 VPN 接口；不要通过重启整个网络来更新界面。

## 验证范围

README 提供构建和测试命令；Go 变更补充 Linux 目标的 `go vet`。
测试环境覆盖 x86_64、fw4、支持 nftset 的 dnsmasq 与 OpenClash redir-host。
ARM64 已在 Docker Desktop 的 ARM64 Linux 内核上运行静态测试程序，协议、加解密、证书及真实 TUN 测试通过；尚未在 ARM 路由器上连接公司服务器。
隔离 network namespace 已验证独立 WAN/LAN 的 NAT、公司 IP/域名地址集合、普通目标 WAN 回落、黑名单优先和路由器本机流量，执行生产 priority 脚本中的选择链。
此测试使用 veth 模拟 VPN 数据出口，不等同于完整 OpenWrt/netifd、DNS 填充、PPPoE 或真实 VPN 服务端联通验收。
隔离页面已检查手机/桌面宽度与明暗主题，但这不等于真实登录态的 LuCI 端到端验收。
OpenConnect 的服务端主动断开尚未解决；Hillstone 的稳定性结果不能套用到它。
其他已知边界参见 README 与验收摘要。

提交或发布前执行 [隐私检查](PUBLISHING.md)，不要从私人备份导回旧提交或原始现场文档。

## 分流优先级诊断

0.7.3 新增私有规则文件导入：写权限 RPC `import_company_rules` 将 JSON 内容交给核心 `-company-import`，固定路径存储，大小/格式校验，0600 临时文件后原子替换，完成后通知热刷新。不会将规则写入 UCI、公开 Git 或构建包。现场验证用原清单作幂等导入，不能用合成测试规则覆盖实际公司清单。新增核心通过文件原子替换热更新，运行中的 VPN 进程保留旧映像，下一次自然重连再使用新核心。

LuCI 0.7.3 在首次发现已连接或连接状态变为已连接时执行一次只读检查，另提供手动检查按钮，不新增后台轮询。
检查三个必需 fw4 入口首位，以及存在时的可选 nat_output 入口，同时检查准确的策略规则优先级及两张路由表默认出口。其他表的 prerouting/output/ingress 基础链或更早策略规则触发“需确认”，不是已证实冲突。
`priority audit` 复用同一检查，修复后再次验证，缺失入口不再静默跳过。现有 5 分钟兜底频率不变。
尚不检查 Fake-IP、DNS、tc/eBPF、硬件卸载及所有规则语义；“顺序正常”不等于端到端兼容保证。
现场已热部署并验证 RPC、HTTP 资源和规则检查，VPN 未重连；尚未进行真实浏览器登录态 UI 验收。

## 0.7.3 审查修复

原生 fw4 不提供 nat_output：只在它存在时检查；必需入口缺失或已有入口被插队仍报错。Hillstone 协议 schema 仅注册连接和证书参数，纯策略字段留在 UCI，由核心直接读取和信号热更新，不再参与 netifd 协议参数变更判断。

已有 netifd 缓存需通过网络服务重启或系统重启更新，不能仅替换协议脚本就宣称修复已生效。修复前的可选链误报、策略字段误注册均已用回归测试复现；修复后通过。完整连接连续性与定时周期的现场验收另行记录。
