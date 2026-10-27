import importlib.util
import io
import json
import re
from pathlib import Path
import tarfile
import unittest

spec = importlib.util.spec_from_file_location('build_ipk', Path(__file__).resolve().parents[1] / 'scripts/build-ipk.py')
builder = importlib.util.module_from_spec(spec)
spec.loader.exec_module(builder)


class Package(unittest.TestCase):
    def test_versioned_menu_resources_exist_and_match_source(self):
        version = re.search(r'PKG_VERSION:=(.+)', (builder.PKG / 'Makefile').read_text()).group(1).strip().replace('.', '')
        menu = json.loads((builder.PKG / 'root/usr/share/luci/menu.d/luci-app-xdvpn.json').read_text())
        entry = menu['admin/services/xdvpn']['action']['path']
        self.assertEqual(entry, 'xdvpn/main_v' + version)
        resources = builder.PKG / 'htdocs/luci-static/resources'
        script = resources / ('view/' + entry + '.js')
        canonical = resources / 'view/xdvpn/main.js'
        self.assertEqual(script.read_bytes(), canonical.read_bytes())
        css = 'view/xdvpn/style_v' + version + '.css'
        self.assertIn(css, script.read_text())
        self.assertEqual((resources / css).read_bytes(), (resources / 'view/xdvpn/style.css').read_bytes())

    def test_parent_directories_precede_files_for_opkg(self):
        result = builder.archive([('usr/libexec/xdvpn/hook', b'#!/bin/sh\n', 0o755)])
        seen = set()
        with tarfile.open(fileobj=io.BytesIO(result)) as tar:
            for member in tar:
                path = Path(member.name)
                self.assertEqual(member.uid, 0)
                if str(path.parent) != '.':
                    self.assertIn(str(path.parent), seen)
                if member.isdir():
                    self.assertEqual(member.mode, 0o755)
                    seen.add(str(path))
                else:
                    self.assertEqual(tar.extractfile(member).read(), b'#!/bin/sh\n')


class ImportACL(unittest.TestCase):
    def test_private_import_requires_write_permission(self):
        acl = json.loads((builder.PKG / 'root/usr/share/rpcd/acl.d/luci-app-xdvpn.json').read_text())['luci-app-xdvpn']
        self.assertIn('import_company_rules', acl['write']['ubus']['xdvpn'])
        self.assertNotIn('import_company_rules', acl['read']['ubus']['xdvpn'])


if __name__ == '__main__':
    unittest.main()
