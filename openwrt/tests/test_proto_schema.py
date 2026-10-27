"""Run the real protocol initializer, inspect the schema consumed by netifd."""
from pathlib import Path
import subprocess
import unittest
ROOT=Path(__file__).resolve().parents[1]
class ProtoSchema(unittest.TestCase):
    def test_only_transport_parameters_trigger_reload(self):
        script=ROOT/'package/xdvpn-hillstone/root/lib/netifd/proto/xdvpn_hillstone.sh'
        shell='INCLUDE_ONLY=1\nproto_config_add_string() { echo "$1"; }\nproto_config_add_int() { echo "$1"; }\nproto_config_add_boolean() { echo "$1"; }\nproto_config_add_array() { echo "$1"; }\n. "$1"\nproto_xdvpn_hillstone_init_config\n'
        fields=set(subprocess.check_output(['sh','-c',shell,'test',str(script)],text=True).split())
        for key in ['server','port','interface','username','password','xdvpn_certpin','xdvpn_cert_endpoint']:
            self.assertIn(key,fields)
        for key in ['xdvpn_mode','xdvpn_domains','xdvpn_networks','xdvpn_blacklist','xdvpn_private_rules','xdvpn_protect_anthropic']:
            self.assertNotIn(key,fields)
