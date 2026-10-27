package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strings"
	"time"
)

type activityEvent struct {
	Time   string     `json:"time"`
	Kind   string     `json:"kind"`
	Text   string     `json:"text"`
	Access *accessRow `json:"access,omitempty"`
}

var unresolvedCount = regexp.MustCompile(`unresolved=([0-9]+)`)

func activityLine(line string) (activityEvent, bool) {
	e := activityEvent{Kind: "info"}
	if !strings.Contains(line, "xdvpn") && !strings.Contains(line, "SCVPN") && !strings.Contains(line, "Hillstone") {
		return e, false
	}
	if len(line) >= 24 {
		e.Time = line[:24]
		if stamp, err := time.Parse("Mon Jan _2 15:04:05 2006", e.Time); err == nil {
			e.Time = stamp.Format("2006-01-02 15:04:05")
		}
	}
	switch {
	case strings.Contains(line, "Interface 'xdvpn' is now up"):
		e.Kind = "connected"
		e.Text = "VPN 连接已建立"
	case strings.Contains(line, "Interface 'xdvpn' is now down"), strings.Contains(line, "Interface 'xdvpn' has lost the connection"):
		e.Kind = "disconnected"
		e.Text = "VPN 已断开"
	case strings.Contains(line, "policy refreshed without reconnect"):
		e.Kind = "rules"
		e.Text = "分流规则已刷新，连接保持不变"
	case strings.Contains(line, "SCVPN authenticated"):
		e.Text = "VPN 身份认证通过"
	case strings.Contains(line, "SCVPN health missed="):
		e.Kind = "warning"
		e.Text = "数据通道探测未收到响应，正在继续检查"
	case strings.Contains(line, "retry in "):
		e.Kind = "warning"
		e.Text = "连接结束，正在等待自动重试"
	case strings.Contains(line, "hillstone:"):
		e.Kind = "warning"
		e.Text = "VPN 核心报告错误；请检查连接、证书或配置"
	case strings.Contains(line, "Cannot restore VPN"), strings.Contains(line, "VPN policy audit failed"):
		e.Kind = "warning"
		e.Text = "分流规则恢复失败，需要检查"
	case strings.Contains(line, "SCVPN domains"):
		n := unresolvedCount.FindStringSubmatch(line)
		if len(n) < 2 || n[1] == "0" {
			return e, false
		}
		e.Kind = "dns"
		e.Text = "有 " + n[1] + " 个已填写域名未解析到 IPv4；父域名没有地址不代表子域名规则失效"
	default:
		return e, false
	}
	return e, true
}

// Bounded output and deadline; never expose raw system logs or credentials.
type cappedOutput struct {
	data []byte
	max  int
}

func (b *cappedOutput) Write(p []byte) (int, error) {
	n := len(p)
	if n >= b.max {
		b.data = append(b.data[:0], p[n-b.max:]...)
	} else {
		extra := len(b.data) + n - b.max
		if extra > 0 {
			b.data = b.data[extra:]
		}
		b.data = append(b.data, p...)
	}
	return n, nil
}
func uiCommand(name string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	b := &cappedOutput{max: 256 << 10}
	cmd.Stdout = b
	err := cmd.Run()
	return b.data, err
}
func emitUI(v any) error { return json.NewEncoder(os.Stdout).Encode(v) }
func activityInfo() error {
	b, e := uiCommand("logread", "-e", "xdvpn")
	if e != nil {
		return emitUI(map[string]any{"error": "暂时无法读取系统日志"})
	}
	events := []activityEvent{}
	for _, line := range strings.Split(string(b), "\n") {
		if event, ok := activityLine(line); ok {
			if event.Kind == "dns" && len(events) > 0 && events[len(events)-1].Kind == "dns" && events[len(events)-1].Text == event.Text {
				events[len(events)-1] = event
			} else {
				events = append(events, event)
			}
		}
	}
	if len(events) > 100 {
		events = events[len(events)-100:]
	}
	for i, j := 0, len(events)-1; i < j; i, j = i+1, j-1 {
		events[i], events[j] = events[j], events[i]
	}
	rows, available := readAccess()
	for i := range rows {
		r := rows[i]
		name := r.Group
		if r.Outer {
			name += "（TLS 外层域名）"
		}
		if len(r.Domains) == 0 {
			name = "IP " + strings.Join(r.IPs, "、")
		}
		events = append(events, activityEvent{Time: r.Last, Kind: "access", Text: name + " → Hillstone 公司 VPN", Access: &r})
	}
	sort.SliceStable(events, func(i, j int) bool { return events[i].Time > events[j].Time })
	note := "访问记录来自已送入 Hillstone 隧道的数据；不代表网站响应成功。每个子域名单独显示，同一域名，首次访问起 5 分钟内合并，之后新访问另记一笔，最多 100 组。仅存内存，VPN 重连后清空。QUIC 或无法识别时显示具体 IP；带 ECH 的 TLS 外层域名单独标注，不能视为真实访问域名。"
	if !available {
		note = "当前无 Hillstone 访问记录通道（未连接或使用其他核心）；以下为系统事件。"
	}
	return emitUI(map[string]any{"events": events, "note": note})
}
func setHasIP(v any, ip net.IP) bool {
	switch x := v.(type) {
	case string:
		if p := net.ParseIP(x); p != nil {
			return p.Equal(ip)
		}
		_, n, e := net.ParseCIDR(x)
		return e == nil && n.Contains(ip)
	case []any:
		for _, item := range x {
			if setHasIP(item, ip) {
				return true
			}
		}
	case map[string]any:
		if prefix, ok := x["prefix"].(map[string]any); ok {
			_, n, e := net.ParseCIDR(fmt.Sprint(prefix["addr"]) + "/" + fmt.Sprint(prefix["len"]))
			return e == nil && n.Contains(ip)
		}
		if r, ok := x["range"].([]any); ok && len(r) == 2 {
			a := net.ParseIP(fmt.Sprint(r[0])).To4()
			b := net.ParseIP(fmt.Sprint(r[1])).To4()
			p := ip.To4()
			if a != nil && b != nil && p != nil {
				return strings.Compare(string(a), string(p)) <= 0 && strings.Compare(string(p), string(b)) <= 0
			}
			return false
		}
		if elem, ok := x["elem"]; ok {
			return setHasIP(elem, ip)
		}
		if val, ok := x["val"]; ok {
			return setHasIP(val, ip)
		}
	}
	return false
}
func readIPSet(table, name string) (any, error) {
	b, e := uiCommand("nft", "-j", "list", "set", "inet", table, name)
	if e != nil {
		return nil, e
	}
	var obj struct {
		Nftables []struct {
			Set struct {
				Elem []any `json:"elem"`
			} `json:"set"`
		} `json:"nftables"`
	}
	if e = json.Unmarshal(b, &obj); e != nil {
		return nil, e
	}
	for _, v := range obj.Nftables {
		if v.Set.Elem != nil {
			return v.Set.Elem, nil
		}
	}
	return []any{}, nil
}
func domainInfo() error {
	var req struct {
		Domain string `json:"domain"`
	}
	if json.NewDecoder(io.LimitReader(os.Stdin, 1024)).Decode(&req) != nil {
		return emitUI(map[string]any{"error": "请输入一个域名"})
	}
	name := strings.ToLower(strings.TrimSuffix(strings.TrimSpace(req.Domain), "."))
	if !validDomain(name) {
		return emitUI(map[string]any{"error": "请输入域名，不带协议、路径、端口或通配符"})
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	ips, e := net.DefaultResolver.LookupIP(ctx, "ip4", name)
	if e != nil || len(ips) == 0 {
		return emitUI(map[string]any{"domain": name, "error": "未解析到 IPv4，当前无法判断；请检查 DNS 或具体子域名"})
	}
	b, e := uiCommand("ubus", "call", "network.interface.xdvpn", "status")
	var status struct {
		Up bool `json:"up"`
	}
	if e == nil {
		e = json.Unmarshal(b, &status)
	}
	if e != nil || !status.Up {
		return emitUI(map[string]any{"domain": name, "error": "VPN 当前未连接，不能确认公司出口；请先连接"})
	}
	modeBytes, _ := os.ReadFile("/var/run/xdvpn-priority/mode")
	protectBytes, _ := os.ReadFile("/var/run/xdvpn-priority/protect")
	mode := strings.TrimSpace(string(modeBytes))
	protect := strings.TrimSpace(string(protectBytes)) != "false"
	names := []string{"xdvpn_bypass4", "xdvpn_deny4", "xdvpn_allow4", "xdvpn_domain4", "xdvpn_block4"}
	sets := make([]any, len(names))
	for i, n := range names {
		sets[i], e = readIPSet("fw4", n)
		if e != nil {
			return emitUI(map[string]any{"error": "运行中的分流集合不完整，暂时无法可靠判断"})
		}
	}
	results := []map[string]string{}
	seen := map[string]bool{}
	for _, ip := range ips {
		if len(results) >= 32 {
			break
		}
		if ip.To4() == nil || seen[ip.String()] {
			continue
		}
		seen[ip.String()] = true
		decision, reason := "fallback", "未命中公司规则，交还原网络；后续是否代理由 OpenClash / Passwall 等决定"
		switch {
		case setHasIP(sets[4], ip):
			decision = "protected"
			reason = "命中其他黑名单，禁止通过公司 VPN，交还原网络"
		case setHasIP(sets[0], ip):
			reason = "本地网络、VPN 连接或服务器排除目标，交还原网络"
		case protect && setHasIP(sets[1], ip):
			decision = "protected"
			reason = "命中保护地址，禁止通过公司 VPN，交还原网络"
		case setHasIP(sets[2], ip) || setHasIP(sets[3], ip):
			decision = "vpn"
			reason = "命中当前 VPN 分流集合"
		}
		results = append(results, map[string]string{"ip": ip.String(), "decision": decision, "reason": reason})
	}
	return emitUI(map[string]any{"domain": name, "mode": mode, "results": results, "note": "这是本次 DNS 解析及当前规则判断，不是已发生连接的证明。共享 IP、客户端加密 DNS 和缓存可能影响实际去向。"})
}
