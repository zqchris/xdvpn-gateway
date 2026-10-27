# Hillstone SCVPN 控制通道诊断

独立诊断程序，不属于发布 IPK。参考 hilldust（542528d）的公开消息格式，
用 Go 标准库实现有长度限制的编码/解析；不运行其系统路由脚本。

默认仅发送无凭据 CLIENT_INFO，检查服务器是否理解 SCVPN。它不是 VPN 客户端，
不会建立数据隧道、协商密钥、导入路由或更改 DNS。

`-sha256` 必填，严格匹配服务器叶证书 SHA256。此模式以指纹替代公共 CA 校验；
首次观测到的指纹本身不构成身份验证。向自签名服务器发送凭据前，应通过可信渠道
核验指纹，或由用户明确接受该证书。

显式指定 `-auth` 才会在路由器本地读取 `network.xdvpn.username/password`，
发送单次用户名密码认证。成功后发送 logout，失败不自动重试。
不输出账号、密码、cookie、完整返回报文。此测试不验证后续隧道可用性。

测试与构建（在仓库根目录）：

```sh
go test scripts/diagnostics/hillstone-probe/main.go scripts/diagnostics/hillstone-probe/main_test.go
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags='-s -w' -o dist/diagnostics/hillstone-probe scripts/diagnostics/hillstone-probe/main.go
```

调用时传入 `-endpoint HOST:PORT -sha256 APPROVED_SHA256`，默认不认证。

协议参考：https://github.com/LionNatsu/hilldust/blob/542528d/hillstone.py
