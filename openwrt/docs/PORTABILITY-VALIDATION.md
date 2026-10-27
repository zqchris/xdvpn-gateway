# ARM 与主路由验证

## 已执行

- 使用 `GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go test -c` 构建测试程序，在 ARM64 Linux 内核实际运行，全部测试及 fuzz 种子通过，TUN 测试没有跳过。
- Docker 镜像用户空间为 Alpine amd64，经宿主翻译执行工具；被测 Go ELF 是静态 ARM64，Linux 内核为 ARM64。不是完整 ARM OpenWrt 固件模拟。
- `tests/test-routing-netns.sh` 在一次性 namespace 中创建 LAN、路由器、VPN 出口、WAN 出口，加载生产脚本的选择链。
- 同时验证 LAN 转发与路由器本机请求：公司 IP、已解析域名地址走 VPN；普通地址走 WAN；黑名单与白名单重叠时走 WAN；VPN 返回流量可以回到 LAN。
- 测试各目标只存在于预期出口，选错出口会失败。使用合成地址，不读取公司清单或账户，不修改现场路由器。

## 复现

在核心目录交叉构建，再在具有 TUN 和 NET_ADMIN 权限的 ARM64 Linux 环境运行：

```sh
GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go test -c -o /tmp/xdvpn-arm64.test
/tmp/xdvpn-arm64.test -test.v
```

路由测试在装有 iproute2、nftables、ping 的隔离 Linux 测试环境中以 root 运行：

```sh
sh tests/test-routing-netns.sh package/luci-app-xdvpn/root/usr/libexec/xdvpn/priority
```

## 仍需补测

这些结果不能证明 R2S、R4S、MT3000 的具体固件都可直接安装与联通。
尚需完整 OpenWrt 环境验证包依赖、netifd 生命周期、fw4 重载、dnsmasq 集合更新；
真机验证 WAN DHCP/PPPoE、MTU/MSS、IPv6、硬件流量卸载、断线重拨、持续传输与 CPU/内存。
当前 WAN 拓扑没有启动 OpenClash/Passwall，不能用它证明这些插件所有运行模式均兼容。
模拟 VPN 出口验证的是路由与 NAT，不是公司认证或 SCVPN 完整服务端会话。
OpenConnect 的服务端断开问题独立存在，不能因架构测试通过而宣称已修复。
