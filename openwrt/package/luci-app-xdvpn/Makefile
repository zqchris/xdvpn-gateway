include $(TOPDIR)/rules.mk

LUCI_TITLE:=XDVPN OpenConnect connection panel
LUCI_DEPENDS:=+luci-base +openconnect +ca-bundle +xdvpn-hillstone
LUCI_PKGARCH:=all
PKG_VERSION:=0.7.3
PKG_RELEASE:=1
PKG_LICENSE:=MIT

define Package/luci-app-xdvpn/prerm
#!/bin/sh
[ -n "$${IPKG_INSTROOT}" ] && exit 0
if [ "$$(uci -q get network.xdvpn.xdvpn_owned)" = 1 ]; then
    ifdown xdvpn
    if [ "$$1" != upgrade ]; then
        /usr/libexec/xdvpn/priority remove || exit 1
        uci set network.xdvpn.auto=0
        uci commit network
    fi
fi
exit 0
endef

include $(TOPDIR)/feeds/luci/luci.mk

# call BuildPackage - OpenWrt buildroot signature
