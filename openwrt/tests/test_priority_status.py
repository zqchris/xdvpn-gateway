"""Exercise the shipped checker against kernel command snapshots."""
import json
import os
from pathlib import Path
import subprocess
import tempfile
import unittest
ROOT = Path(__file__).resolve().parents[1]

class PriorityStatus(unittest.TestCase):
    def check(self, scenario, check_only=False):
        with tempfile.TemporaryDirectory() as d:
            p=Path(d)
            (p/'pbr-owned').touch()
            script=(ROOT/'package/luci-app-xdvpn/root/usr/libexec/xdvpn/priority-status').read_text().replace('/var/run/xdvpn-priority', d)
            (p/'status').write_text(script)
            (p/'nft').write_text('''#!/bin/sh
if [ "$1" = list ] && [ "$2" = chain ]; then
 [ "$SCENARIO" != no_nat_output ] || [ "$5" != nat_output ] || exit 1
 exit 0
elif [ "$1" = list ]; then
 echo 'table inet fw4 {'
 echo ' type filter hook prerouting priority mangle; policy accept;'
 if [ "$SCENARIO" = foreign ]; then echo 'table inet proxy {'; echo 'type filter hook prerouting priority -200; policy accept;'; fi
else
 [ "$SCENARIO" != missing ] || exit 1
 [ "$SCENARIO" != no_nat_output ] || [ "$6" != nat_output ] || exit 1
 if [ "$SCENARIO" = nat_reordered ] && [ "$6" = nat_output ]; then echo 'jump proxy # handle 1'; fi
 [ "$SCENARIO" != reordered ] || echo 'jump proxy # handle 1'
 echo 'jump xdvpn_select comment "xdvpn-priority" # handle 2'
fi
''')
            (p/'ip').write_text('''#!/bin/sh
if [ "$2" = rule ]; then
 echo '0: from all lookup local'
 [ "$SCENARIO" != earlier ] || echo '10009: from all fwmark 0x162 lookup 354'
 echo '10010: from all fwmark 0x10000000/0x10000000 lookup 51833'
 [ "$SCENARIO" = wrongpref ] && pref=20020 || pref=10020
 echo "$pref: from all fwmark 0x20000000/0x20000000 lookup 51834"
elif [ "$5" = 51834 ]; then
 echo 'default dev vpn-xdvpn scope link'
else
 [ "$SCENARIO" = noexit ] || echo 'default via 192.0.2.1 dev wan'
fi
''')
            for name in ['nft','ip']: (p/name).chmod(0o755)
            if scenario=='inactive':(p/'pbr-owned').unlink()
            return subprocess.run(['sh',str(p/'status')]+(['--check'] if check_only else []),env=dict(os.environ,PATH=d+':'+os.environ['PATH'],SCENARIO=scenario),capture_output=True,text=True)
    def test_statuses(self):
        for scenario, expected in [('normal','ordered'),('inactive','inactive'),('foreign','review_required'),('earlier','review_required'),('missing','repair_needed'),('reordered','repair_needed'),('wrongpref','repair_needed'),('noexit','repair_needed')]:
            with self.subTest(scenario=scenario):
                r=self.check(scenario)
                self.assertEqual(r.returncode,0,r.stderr)
                self.assertEqual(json.loads(r.stdout)['status'],expected)
    def test_audit_checks_actual_order(self):
        self.assertEqual(self.check('normal',True).returncode,0)
        self.assertNotEqual(self.check('reordered',True).returncode,0)
        self.assertNotEqual(self.check('missing',True).returncode,0)

    def test_optional_nat_output(self):
        self.assertEqual(self.check('no_nat_output',True).returncode,0)
        self.assertEqual(json.loads(self.check('no_nat_output').stdout)['status'],'ordered')
        self.assertNotEqual(self.check('nat_reordered',True).returncode,0)
