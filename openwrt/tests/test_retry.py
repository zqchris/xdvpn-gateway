"""Retry adapter exercises real shell control flow, without a real VPN account."""
import os
from pathlib import Path
import subprocess
import tempfile
import unittest
ROOT=Path(__file__).resolve().parents[1]
class Retry(unittest.TestCase):
    def test_transient_backoff_and_auth_block(self):
        with tempfile.TemporaryDirectory() as tmp:
            p=Path(tmp)
            (p/'proto').write_text('proto_notify_error() { echo notify "$*"; }\nproto_block_restart() { echo block "$*"; }\n')
            (p/'core').write_text('#!/bin/sh\nn=$(cat "$TEST_DIR/count" 2>/dev/null || echo 0)\nn=$((n+1)); echo "$n" > "$TEST_DIR/count"\n[ "$n" -lt 3 ] && exit 1\nexit 2\n')
            (p/'sleep').write_text('#!/bin/sh\necho "sleep $1"\n')
            (p/'logger').write_text('#!/bin/sh\nexit 0\n')
            for name in ('core','sleep','logger'):(p/name).chmod(0o755)
            script=(ROOT/'package/xdvpn-hillstone/root/usr/libexec/xdvpn/hillstone-run').read_text().replace('/lib/netifd/netifd-proto.sh',str(p/'proto')).replace('/usr/sbin/xdvpn-hillstone',str(p/'core'))
            (p/'run').write_text(script)
            r=subprocess.run(['sh',str(p/'run')],env={**os.environ,'TEST_DIR':tmp,'PATH':tmp+':'+os.environ['PATH']},capture_output=True,text=True,timeout=5)
            self.assertEqual(r.returncode,1,r.stderr)
            self.assertIn('sleep 5',r.stdout)
            self.assertIn('sleep 10',r.stdout)
            self.assertIn('block xdvpn',r.stdout)
            self.assertEqual((p/'count').read_text().strip(),'3')
