#!/bin/sh
[ -n "$INCLUDE_ONLY" ] || {
    . /lib/functions.sh
    . ../netifd-proto.sh
    init_proto "$@"
}
proto_xdvpn_hillstone_init_config() {
    proto_config_add_string server
    proto_config_add_int port
    proto_config_add_string interface
    proto_config_add_string username
    proto_config_add_string password
    proto_config_add_string xdvpn_cert_endpoint
    proto_config_add_string xdvpn_certpin
    # Policy keys stay in UCI but are read directly by the policy worker.
    # Registering them here makes config.change/network reload tear down the VPN.
    # Only transport and certificate changes belong in netifd's protocol schema.
    no_device=1
    available=1
}
proto_xdvpn_hillstone_setup() {
    local config="$1" server interface address
    [ "$config" = xdvpn ] || { proto_notify_error "$config" INVALID_INTERFACE; proto_block_restart "$config"; return 1; }
    json_get_vars server interface
    for address in $(resolveip -4 -t 5 "$server"); do
        proto_add_host_dependency "$config" "$address" "$interface"
    done
    proto_export INTERFACE="$config"
    proto_export GOMEMLIMIT=24MiB
    proto_run_command "$config" /usr/libexec/xdvpn/hillstone-run
}
proto_xdvpn_hillstone_teardown() {
    proto_kill_command "$1" TERM
}
[ -n "$INCLUDE_ONLY" ] || add_protocol xdvpn_hillstone
