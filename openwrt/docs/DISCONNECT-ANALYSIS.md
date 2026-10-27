# OpenConnect 断连调查摘要

在一套测试部署中，OpenConnect 9.12/GnuTLS 3.8.5 可完成认证及业务 HTTPS，
但约半分钟后收到 `Received server disconnect`。部分会话随后在退出阶段崩溃。
服务器主动终止与客户端退出崩溃是两个需要分别定位的问题。

## 已做对照

- 绕过正式插件的分流脚本，独立 TUN 仅配置一个测试目的地址。
- 默认参数、关闭压缩、客户端 User-Agent 调整、旧式认证流程均未消除断连。
- 各轮断连前业务请求成功；关闭压缩也不能重复避免退出崩溃。
- 早期的 DPD、持续业务请求及平台标识对照同样未解决。
- 独立 AnyConnect 协议核心对照也收到服务器终止帧，不能认定换 UI 或核心就能解决。
- 系统 9.12/GnuTLS 关闭 vhost-net（`NOVHOST=1`）后，一轮在约 39 秒收到终止报文、正常以状态 1 退出；没有解决断连。
- 独立构建 9.21/OpenSSL 3.5.9，同样关闭 vhost-net，一轮在约 51 秒收到终止报文、以状态 1 退出。此前每 10 秒内网 HTTPS 成功。此轮同时更换版本与 TLS 库，不能单独归因于版本或库。
- 新旧客户端都成功使用 AnyConnect/CSTP。服务器未提供 DTLS 地址，数据走 TLS；这可能影响吞吐，不能直接解释收到显式终止报文。

## 客户端退出崩溃线索

既有崩溃转储显示 musl 分配器检查失败，返回地址与 `free_vring()` 释放 used ring 的路径吻合。
上游 9.12 与 9.21 的 `vhost.c` 都为 used ring 分配 `4 + 8 * ring_size` 字节，
但事件字段 `avail_event` 位于该区域末尾之外，还需要 2 字节。这是可疑的越界来源，
尚未通过同一构建的修补前后重复测试证明它就是本部署崩溃的根因。
关闭 vhost-net 的两轮未崩溃，仅是支持线索，不能表述为已修复。

没有将禁用加速或实验构建部署为正式版本；禁用加速对吞吐和 CPU 的影响也未量化。
不要将退出清理问题与服务器主动终止合并为一个根因。

服务器没有提供客户端可解码的具体终止原因。尚不能认定某项账号策略、服务端故障或网络条件是根因。
下一项关键证据是由管理员私下核对测试时间及会话标识对应的服务端日志。
不通过无限重连、忽略终止帧或关闭证书校验将故障包装成成功。

临时测试使用有限时长并在结束后清理；没有把未验证参数写入正式配置。
不要将本部署的失败泛化为所有 OpenConnect 服务不可用，也不要把 Hillstone 的验证结果套用到 OpenConnect。

## 公开参考

- [OpenConnect 手册](https://www.infradead.org/openconnect/manual.html)
- [OpenConnect 变更记录](https://www.infradead.org/openconnect/changelog.html)
- [ocserv 手册](https://www.infradead.org/ocserv/manual.html)

原始日志、精确会话时间、IP 和证书信息均不随本仓库发布。
