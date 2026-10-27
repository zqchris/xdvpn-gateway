# XDVPN Hillstone core

单进程 Go SCVPN 客户端。没有桌面 UI、Python/Scapy、容器或通用代理核心。
Go 1.25+；目前发行 Linux amd64 / arm64，使用标准库，不需要 libc 动态链接。

## 已实现

- TLS 1.2+，认证前校验用户确认的精确叶证书 SHA256；没有无校验模式。
- 用户名密码认证、网络参数和密钥协商、UDP ESP / TUN 收发。
- AES-CBC 和 HMAC；当前服务器实测 AES-128-CBC + HMAC-MD5-96。
- 随机逐包 IV、ICV 校验、64 包抗重放窗口、序列号耗尽时退出。
- 明确的服务器终止报文会退出。SCVPN 协商后正常 TLS EOF 不代表 UDP 会话结束。
- 每 30 秒通过隧道查询服务器下发的 DNS，连续三次失败后清理并退出。
- 连接管理器按 5、10、20 秒逐步退避，最大 300 秒；稳定 120 秒后重置。
- 明确的认证拒绝、证书指纹不匹配或不支持的算法会阻止自动重试，等待用户修正后重新连接。
- netifd 管理地址、显式路由、防火墙区域；SIGTERM 清理。IPv6 不进入此隧道。

## 内存与 CPU

- 收发各复用固定缓冲区，加密/解密对象和 HMAC 输出缓存复用。
- 原型性能优化后 AES 加密基准为 0 B/op、0 allocs/op。
- netifd 设置 GOMEMLIMIT=24MiB，这是 Go 堆回收软目标，不是进程 RSS 硬上限。
- 工作线程阻塞在网络/TUN 读写，没有轮询忙等；空闲时只有低频健康探测。
- 实际 CPU 占用随流量增加，不承诺零开销或所有硬件线速；测量见 docs/HILLSTONE-VALIDATION.md。

## 命令

正常运行由 netifd 调用 `/usr/sbin/xdvpn-hillstone -netifd`。
配置和凭据从路由器 `network.xdvpn` 读取，密码不会出现在进程参数里。
接口名固定为 `vpn-xdvpn`，只支持一个受插件管理的实例。

诊断模式必须显式传入 `-duration` 和 `-test-route` 才建立数据隧道；默认只认证/协商后退出。
`-endpoint` 和 `-sha256` 可以覆盖服务器与证书指纹，不能覆盖账号或密码。

## 限制

不实现交互式 MFA、客户端证书认证、终端合规检查、压缩和原地 rekey。
服务器要求 rekey 时退出，由连接管理层重新建立会话；长周期 rekey 尚未实机验收。
不绕过服务端主机绑定、认证策略或断开指令。

## 许可与来源

本目录采用 GPL-3.0-only，完整许可证见 LICENSE。
SCVPN 格式、算法编号及密钥派生参考 LionNatsu/hilldust @542528d；
协议边界、ESP 加密实现、OpenWrt 集成与测试在本项目中实现。
独立 ESP 已知答案向量参考 NolanHo/hilldust 的公开 selftest.py。
LuCI 界面与其他项目代码的许可证见仓库根目录说明。
