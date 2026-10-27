"""Exercise the actual shell hooks with a fake netifd/UCI boundary, not real routes."""
import json
import os
from pathlib import Path
import subprocess
import tempfile
import unittest

ROOT = Path(__file__).resolve().parents[1]
FILES = ROOT / 'package/luci-app-xdvpn/root'


class Hooks(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.path = Path(self.temp.name)
        self.state = self.path / 'uci.json'
        self.state.write_text('{}')
        mock = self.path / 'uci'
        mock.write_text('''#!/usr/bin/env python3
import json, os, sys
p = os.environ['UCI_STATE']
d = json.load(open(p))
a = [x for x in sys.argv[1:] if x != '-q']
op, key = a[0], a[1]
if op == 'get':
    if key not in d: sys.exit(1)
    print(d[key])
elif op in ('set', 'add_list'):
    key, val = key.split('=', 1)
    d[key] = (d.get(key, '') + ' ' + val).strip() if op == 'add_list' else val
    json.dump(d, open(p, 'w'))
elif op != 'commit': sys.exit(2)
''')
        mock.chmod(0o755)
        stub = self.path / 'netifd.sh'
        stub.write_text('''
proto_init_update() { echo "link $*"; }
proto_add_ipv4_address() { echo "address $*"; }
proto_add_ipv4_route() { echo "route $*"; }
proto_send_update() { echo "send $*"; }
logger() { :; }
''')
        hook = (FILES / 'usr/libexec/xdvpn/vpnc-script').read_text()
        self.hook = self.path / 'hook'
        priority = self.path / 'priority'
        priority.write_text('#!/bin/sh\nexit "${GUARD_EXIT:-0}"\n')
        priority.chmod(0o755)
        hook = hook.replace('/usr/libexec/xdvpn/priority', str(priority))
        self.hook.write_text(hook.replace('/lib/netifd/netifd-proto.sh', str(stub)).replace('/etc/xdvpn/company-rules.txt', str(self.path / 'private-rules')))
        self.env = dict(os.environ, PATH=str(self.path) + ':' + os.environ['PATH'],
                        UCI_STATE=str(self.state), INTERFACE='xdvpn', TUNDEV='vpn-xdvpn',
                        reason='connect', INTERNAL_IP4_ADDRESS='10.42.0.2')

    def run_hook(self, config=None, **environment):
        self.state.write_text(json.dumps(config or {'network.xdvpn.xdvpn_mode': 'networks'}))
        return subprocess.run(['sh', str(self.hook)], env=dict(self.env, **environment),
                              text=True, capture_output=True)

    def test_only_explicit_networks_despite_server_default(self):
        r = self.run_hook({'network.xdvpn.xdvpn_mode': 'networks',
                           'network.xdvpn.xdvpn_networks': '10.8.1.0/24 172.20.0.2/32'},
                          CISCO_SPLIT_INC='1', CISCO_SPLIT_INC_0_ADDR='0.0.0.0',
                          INTERNAL_IP4_DNS='10.9.0.53')
        self.assertEqual(r.returncode, 0, r.stderr)
        self.assertIn('route 10.8.1.0 24', r.stdout)
        self.assertIn('route 172.20.0.2 32', r.stdout)
        self.assertIn('route 10.9.0.53 32', r.stdout)
        self.assertNotIn('route 0.0.0.0', r.stdout)
        self.assertIn('send xdvpn', r.stdout)

    def test_protection_failure_never_installs_vpn_routes(self):
        r = self.run_hook(XDVPN_PRIORITY='1', GUARD_EXIT='1')
        self.assertNotEqual(r.returncode, 0)
        self.assertNotIn('send xdvpn', r.stdout)
        self.assertEqual(self.run_hook(XDVPN_PRIORITY='1', GUARD_EXIT='0').returncode, 0)
        r = self.run_hook({'network.xdvpn.xdvpn_mode': 'global4'}, XDVPN_PRIORITY='1')
        self.assertEqual(r.returncode, 0)
        self.assertIn('send xdvpn', r.stdout)

    def test_live_policy_update_does_not_reset_interface_or_main_routes(self):
        r = self.run_hook({'network.xdvpn.xdvpn_mode': 'global4'}, reason='policy-update', XDVPN_PRIORITY='1')
        self.assertEqual(r.returncode, 0, r.stderr)
        self.assertNotIn('send xdvpn', r.stdout)
        self.assertNotIn('route 0.0.0.0', r.stdout)
        self.assertNotIn('route 128.0.0.0', r.stdout)

    def test_company_merges_presets_and_manual_never_push(self):
        for mode in ['company', 'server']:
            r = self.run_hook({'network.xdvpn.xdvpn_mode': mode, 'network.xdvpn.xdvpn_networks': '192.0.2.2'}, XDVPN_COMPANY_READY='1', XDVPN_COMPANY_NETWORKS='198.51.100.0/24', XDVPN_SERVER_ROUTES='0.0.0.0/0')
            self.assertEqual(r.returncode, 0, r.stderr)
            self.assertIn('route 192.0.2.2 32', r.stdout)
            self.assertIn('route 198.51.100.0 24', r.stdout)
            self.assertNotIn('route 0.0.0.0', r.stdout)
            self.assertNotEqual(self.run_hook({'network.xdvpn.xdvpn_mode': mode}).returncode, 0)

    def test_bare_ip_is_host_route(self):
        r = self.run_hook({'network.xdvpn.xdvpn_mode': 'networks', 'network.xdvpn.xdvpn_networks': '10.9.8.7'})
        self.assertEqual(r.returncode, 0)
        self.assertIn('route 10.9.8.7 32', r.stdout)

    def test_domain_routes_are_owned_by_netifd(self):
        r = self.run_hook(XDVPN_DOMAIN_IPS='10.2.3.4 10.2.3.5')
        self.assertEqual(r.returncode, 0)
        self.assertIn('route 10.2.3.4 32', r.stdout)
        self.assertIn('route 10.2.3.5 32', r.stdout)
        self.assertIn('send xdvpn', r.stdout)
        bad = self.run_hook(XDVPN_DOMAIN_IPS='10.2.3.4 invalid')
        self.assertNotEqual(bad.returncode, 0)
        self.assertNotIn('send xdvpn', bad.stdout)

    def test_global_preserves_default_uses_two_halves(self):
        r = self.run_hook({'network.xdvpn.xdvpn_mode': 'global4'})
        self.assertEqual(r.returncode, 0)
        self.assertIn('route 0.0.0.0 1', r.stdout)
        self.assertIn('route 128.0.0.0 1', r.stdout)
        self.assertNotIn('route 0.0.0.0 0', r.stdout)

    def test_invalid_input_never_publishes_partial_routes(self):
        for cidr in ['0.0.0.0/0', '10.2.0.0/33', '999.0.0.0/8', '10.0.0.0/;id']:
            with self.subTest(cidr=cidr):
                r = self.run_hook({'network.xdvpn.xdvpn_mode': 'networks',
                                   'network.xdvpn.xdvpn_networks': '10.8.0.0/16 ' + cidr})
                self.assertNotEqual(r.returncode, 0)
                self.assertNotIn('send xdvpn', r.stdout)

    def test_manual_does_not_load_presets_or_legacy_flag(self):
        r = self.run_hook({'network.xdvpn.xdvpn_mode': 'networks', 'network.xdvpn.xdvpn_private_rules':'1'}, XDVPN_COMPANY_READY='1', XDVPN_COMPANY_NETWORKS='198.51.100.0/24')
        self.assertEqual(r.returncode,0)
        self.assertNotIn('198.51.100',r.stdout)

    def test_disconnect_removes_interface_state(self):
        r = self.run_hook(reason='disconnect', INTERNAL_IP4_ADDRESS='')
        self.assertEqual(r.stdout, 'link vpn-xdvpn 0\nsend xdvpn\n')

    def test_foreign_interface_rejected(self):
        r = self.run_hook(INTERFACE='wan')
        self.assertNotEqual(r.returncode, 0)
        self.assertEqual(r.stdout, '')

    def test_invalid_address_rejected(self):
        self.assertNotEqual(self.run_hook(INTERNAL_IP4_ADDRESS='').returncode, 0)

    def test_reconnect_republishes(self):
        r = self.run_hook(reason='reconnect')
        self.assertEqual(r.returncode, 0)
        self.assertIn('send xdvpn', r.stdout)

    def defaults(self):
        config = self.path / 'network'
        config.touch()
        script = self.path / 'defaults'
        script.write_text((FILES / 'etc/uci-defaults/90-xdvpn').read_text()
                          .replace('/etc/config/network', str(config))
                          .replace('/etc/init.d/xdvpn-rules', 'true'))
        return subprocess.run(['sh', str(script)], env=self.env, text=True, capture_output=True)

    def test_install_disabled_and_idempotent(self):
        self.assertEqual(self.defaults().returncode, 0)
        d = json.loads(self.state.read_text())
        self.assertEqual(d['network.xdvpn.auto'], '0')
        self.assertEqual(d['network.xdvpn.peerdns'], '0')
        self.assertEqual(d['firewall.xdvpn.input'], 'REJECT')
        d['network.xdvpn.server'] = 'vpn.example.org'
        self.state.write_text(json.dumps(d))
        self.assertEqual(self.defaults().returncode, 0)
        self.assertEqual(json.loads(self.state.read_text()), d)

    def test_migrate_old_server_and_private_opt_in(self):
        self.assertEqual(self.defaults().returncode,0)
        baseline=json.loads(self.state.read_text())
        for mode,private,expected in [('server','0','company'),('networks','1','company'),('networks','0','networks'),('global4','1','global4')]:
            d=dict(baseline);d.update({'network.xdvpn.xdvpn_mode':mode,'network.xdvpn.xdvpn_private_rules':private})
            self.state.write_text(json.dumps(d))
            self.assertEqual(self.defaults().returncode,0)
            self.assertEqual(json.loads(self.state.read_text())['network.xdvpn.xdvpn_mode'],expected)

    def test_existing_pin_is_bound_without_replacing_trust(self):
        self.assertEqual(self.defaults().returncode, 0)
        d = json.loads(self.state.read_text())
        d.update({'network.xdvpn.server': 'vpn.example.org', 'network.xdvpn.port': '4433',
                  'network.xdvpn.xdvpn_certpin': 'a' * 64})
        self.state.write_text(json.dumps(d))
        self.assertEqual(self.defaults().returncode, 0)
        after = json.loads(self.state.read_text())
        self.assertEqual(after['network.xdvpn.xdvpn_cert_endpoint'], 'vpn.example.org:4433')
        self.assertEqual(after['network.xdvpn.xdvpn_certpin'], 'a' * 64)

    def test_collision_no_partial_changes(self):
        d = {'firewall.xdvpn_lan': 'forwarding'}
        self.state.write_text(json.dumps(d))
        self.assertNotEqual(self.defaults().returncode, 0)
        self.assertEqual(json.loads(self.state.read_text()), d)


if __name__ == '__main__':
    unittest.main()
