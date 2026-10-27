#!/bin/sh
# Root/Linux only: real nft/ip behavior inside a disposable namespace.
set -eu
source=${1:?usage: sh test-priority-kernel.sh /path/to/priority-status}
ns=xdcheck-$$
tmp=$(mktemp -d)
cleanup() { ip netns del "$ns" 2>/dev/null || true; rm -rf "$tmp"; }
trap cleanup EXIT INT TERM
ip netns add "$ns"
ip -n "$ns" link set lo up
ip -n "$ns" link add wan0 type dummy
ip -n "$ns" link add vpn-xdvpn type dummy
ip -n "$ns" link set wan0 up
ip -n "$ns" link set vpn-xdvpn up
ip -n "$ns" route add table 51833 default dev wan0
ip -n "$ns" route add table 51834 default dev vpn-xdvpn
ip -n "$ns" rule add priority 10010 fwmark 0x10000000/0x10000000 table 51833
ip -n "$ns" rule add priority 10020 fwmark 0x20000000/0x20000000 table 51834
: > "$tmp/pbr-owned"
sed "s|state=/var/run/xdvpn-priority|state=$tmp|" "$source" > "$tmp/check"
ip netns exec "$ns" nft add table inet fw4
ip netns exec "$ns" nft add chain inet fw4 xdvpn_select
for chain in mangle_prerouting dstnat mangle_output; do
 ip netns exec "$ns" nft add chain inet fw4 "$chain"
 ip netns exec "$ns" nft "add rule inet fw4 $chain jump xdvpn_select comment \"xdvpn-priority\""
done
ip netns exec "$ns" sh "$tmp/check" --check
ip netns exec "$ns" sh "$tmp/check" --check
echo 'PASS: three required chains without nat_output (repeated checks)'
ip netns exec "$ns" nft add chain inet fw4 nat_output
if ip netns exec "$ns" sh "$tmp/check" --check; then echo 'FAIL: existing nat_output without entry accepted'; exit 1; fi
ip netns exec "$ns" nft 'add rule inet fw4 nat_output jump xdvpn_select comment "xdvpn-priority"'
ip netns exec "$ns" sh "$tmp/check" --check
for chain in mangle_prerouting dstnat mangle_output; do
 ip netns exec "$ns" nft flush chain inet fw4 "$chain"
 if ip netns exec "$ns" sh "$tmp/check" --check; then echo 'FAIL: missing required entry accepted'; exit 1; fi
 ip netns exec "$ns" nft "add rule inet fw4 $chain jump xdvpn_select comment \"xdvpn-priority\""
done
echo 'PASS: optional chain is checked when present; all required entries remain mandatory'
