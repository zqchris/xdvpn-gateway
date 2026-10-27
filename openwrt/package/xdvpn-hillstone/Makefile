include $(TOPDIR)/rules.mk
PKG_NAME:=xdvpn-hillstone
PKG_VERSION:=0.7.3
PKG_RELEASE:=1
PKG_LICENSE:=GPL-3.0-only
PKG_BUILD_DEPENDS:=golang/host
PKG_BUILD_PARALLEL:=1
GO_PKG:=xdvpn/hillstone
GO_PKG_BUILD_PKG:=xdvpn/hillstone
include $(INCLUDE_DIR)/package.mk
include $(TOPDIR)/feeds/packages/lang/golang/golang-package.mk

define Package/xdvpn-hillstone
  SECTION:=net
  CATEGORY:=Network
  TITLE:=Lightweight Hillstone SCVPN client for XDVPN
  DEPENDS:=+kmod-tun +ip +resolveip +ca-bundle
endef

define Build/Prepare
	mkdir -p $(PKG_BUILD_DIR)
	$(CP) $(CURDIR)/../../core/hillstone/. $(PKG_BUILD_DIR)/
endef

define Package/xdvpn-hillstone/install
	$(INSTALL_DIR) $(1)/etc/init.d $(1)/usr/sbin $(1)/usr/libexec/xdvpn $(1)/lib/netifd/proto $(1)/usr/share/licenses/xdvpn-hillstone $(1)/lib/upgrade/keep.d
	$(INSTALL_BIN) $(GO_PKG_BUILD_BIN_DIR)/hillstone $(1)/usr/sbin/xdvpn-hillstone
	$(INSTALL_BIN) ./root/etc/init.d/xdvpn-policy $(1)/etc/init.d/xdvpn-policy
	$(INSTALL_BIN) ./root/usr/libexec/xdvpn/hillstone-run $(1)/usr/libexec/xdvpn/hillstone-run
	$(INSTALL_BIN) ./root/lib/netifd/proto/xdvpn_hillstone.sh $(1)/lib/netifd/proto/
	$(INSTALL_DATA) ./root/lib/upgrade/keep.d/xdvpn $(1)/lib/upgrade/keep.d/xdvpn
	$(INSTALL_DATA) $(CURDIR)/../../core/hillstone/LICENSE $(1)/usr/share/licenses/xdvpn-hillstone/LICENSE
endef
define Package/xdvpn-hillstone/prerm
#!/bin/sh
[ -n "$$IPKG_INSTROOT" ] && exit 0
if [ "$$(uci -q get network.xdvpn.xdvpn_owned)" = 1 ] && [ "$$(uci -q get network.xdvpn.proto)" = xdvpn_hillstone ]; then
    ifdown xdvpn
    if [ "$$1" != upgrade ]; then
        uci set network.xdvpn.auto=0
        uci commit network
    fi
fi
exit 0
endef
$(eval $(call GoBinPackage,xdvpn-hillstone))
$(eval $(call BuildPackage,xdvpn-hillstone))
