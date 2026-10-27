#!/bin/sh
# Linux/root integration test. Uses only disposable namespaces and synthetic IPs.
# Run with the production priority script path as the first argument.
set -eu
priority=${1:?usage: sh test-routing-netns.sh /path/to/xdvpn/priority}
prefix="xdreg-$$"
r="$prefix-r"; c="$prefix-c"; s="$prefix-s"; w="$prefix-w"
tmp=$(mktemp -d)
cleanup() {
 for ns in "$c" "$s" "$w" "$r"; do ip netns del "$ns" 2>/dev/null || true; done
 rm -rf "$tmp"
}
trap cleanup EXIT INT TERM
for ns in "$r" "$c" "$s" "$w"; do ip netns add "$ns"; ip -n "$ns" link set lo up; done
ip -n "$r" link add lan0 type veth peer name peer0
ip -n "$r" link set peer0 netns "$c"
ip -n "$r" link add vpn-xdvpn type veth peer name peer0
ip -n "$r" link set peer0 netns "$s"
ip -n "$r" addr add 192.0.2.1/24 dev lan0
ip -n "$c" addr add 192.0.2.2/24 dev peer0
ip -n "$r" addr add 10.23.0.1/24 dev vpn-xdvpn
ip -n "$s" addr add 10.23.0.2/24 dev peer0
ip -n "$r" link set lan0 up
ip -n "$r" link set vpn-xdvpn up
ip -n "$c" link set peer0 up
ip -n "$s" link set peer0 up
ip -n "$c" route add default via 192.0.2.1
ip netns exec "$r" sysctl -qw net.ipv4.ip_forward=1 net.ipv4.conf.all.rp_filter=0 net.ipv4.conf.default.rp_filter=0
ip netns exec "$r" sysctl -qw net.ipv4.conf.lan0.rp_filter=0 net.ipv4.conf.vpn-xdvpn.rp_filter=0
ip -n "$r" route add table 51834 default via 10.23.0.2 dev vpn-xdvpn
ip -n "$r" rule add priority 10020 fwmark 0x20000000/0x20000000 table 51834
cat > "$tmp/rules" <<'RULES'
add table inet fw4
add set inet fw4 xdvpn_allow4 { type ipv4_addr; flags interval; }
add element inet fw4 xdvpn_allow4 { 10.23.0.0/24 }
add set inet fw4 xdvpn_domain4 { type ipv4_addr; }
add set inet fw4 xdvpn_deny4 { type ipv4_addr; }
add set inet fw4 xdvpn_block4 { type ipv4_addr; }
add set inet fw4 xdvpn_bypass4 { type ipv4_addr; }
add chain inet fw4 xdvpn_select
RULES
# Exercise the actual selector emitted by the production restore() function.
sed -n '/^flush chain inet fw4 xdvpn_select$/,/^RULES$/p' "$priority" | sed '$d' >> "$tmp/rules"
cat >> "$tmp/rules" <<'RULES'
add chain inet fw4 pre { type filter hook prerouting priority mangle; policy accept; }
add rule inet fw4 pre jump xdvpn_select
add chain inet fw4 nat { type nat hook postrouting priority srcnat; policy accept; }
add rule inet fw4 nat oifname "vpn-xdvpn" masquerade
RULES
ip netns exec "$r" nft -f "$tmp/rules"
# Before the fix: reply destination is the NAT tunnel IP inside allow4, so
# it gets marked for table 51834 again instead of returning to the LAN client.
ip netns exec "$c" ping -c 2 -W 2 10.23.0.2 >/dev/null
echo 'PASS: VPN replies return to LAN when allowlist overlaps the tunnel subnet'

# Main-router topology: separate LAN, WAN and VPN, with NAT on both exits.
ip -n "$r" link add wan0 type veth peer name peer0
ip -n "$r" link set peer0 netns "$w"
ip -n "$r" addr add 198.51.100.1/24 dev wan0
ip -n "$w" addr add 198.51.100.2/24 dev peer0
ip -n "$r" link set wan0 up
ip -n "$w" link set peer0 up
ip -n "$r" route add default via 198.51.100.2
ip -n "$r" route add table 51833 default via 198.51.100.2 dev wan0
ip -n "$r" rule add priority 10010 fwmark 0x10000000/0x10000000 table 51833
ip netns exec "$r" sysctl -qw net.ipv4.conf.wan0.rp_filter=0
for addr in 203.0.113.10 203.0.113.40; do ip -n "$s" addr add "$addr/32" dev lo; done
for addr in 203.0.113.20 203.0.113.30; do ip -n "$w" addr add "$addr/32" dev lo; done
cat > "$tmp/wan-rules" <<'RULES'
add rule inet fw4 nat oifname "wan0" masquerade
add element inet fw4 xdvpn_allow4 { 203.0.113.10, 203.0.113.30 }
add element inet fw4 xdvpn_domain4 { 203.0.113.40 }
add element inet fw4 xdvpn_block4 { 203.0.113.30 }
add chain inet fw4 out { type route hook output priority mangle; policy accept; }
add rule inet fw4 out jump xdvpn_select
RULES
ip netns exec "$r" nft -f "$tmp/wan-rules"
# Each target exists on exactly one exit. Wrong selection cannot pass by accident.
for source in "$c" "$r"; do
 for target in 203.0.113.10 203.0.113.20 203.0.113.30 203.0.113.40; do
  ip netns exec "$source" ping -c 1 -W 2 "$target" >/dev/null
 done
done
echo 'PASS: separate WAN/LAN, VPN IP/domain sets, WAN fallback, blacklist precedence and router-origin traffic'
