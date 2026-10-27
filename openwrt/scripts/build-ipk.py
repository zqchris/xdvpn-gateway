#!/usr/bin/env python3
"""Build the architecture-independent development IPK, without a cross compiler."""
import gzip
import hashlib
import io
from pathlib import Path
import tarfile

ROOT = Path(__file__).resolve().parents[1]
PKG = ROOT / 'package/luci-app-xdvpn'


def archive(files):
    output = io.BytesIO()
    with tarfile.open(fileobj=output, mode='w', format=tarfile.GNU_FORMAT) as tar:
        directories = {str(parent) for name, _, _ in files
                       for parent in Path(name).parents if str(parent) != '.'}
        for name in sorted(directories, key=lambda p: (p.count('/'), p)):
            entry = tarfile.TarInfo('./' + name + '/')
            entry.type, entry.mode = tarfile.DIRTYPE, 0o755
            entry.uid = entry.gid = entry.mtime = 0
            tar.addfile(entry)
        for name, data, mode in sorted(files):
            entry = tarfile.TarInfo('./' + name)
            entry.size, entry.mode = len(data), mode
            entry.uid = entry.gid = entry.mtime = 0
            tar.addfile(entry, io.BytesIO(data))
    return gzip.compress(output.getvalue(), mtime=0)


def main():
    payload = []
    for folder, prefix in [('root', ''), ('htdocs', 'www/')]:
        for path in (PKG / folder).rglob('*'):
            if path.is_file():
                name = prefix + str(path.relative_to(PKG / folder))
                mode = 0o755 if path.read_bytes().startswith(b'#!/bin/sh') else 0o644
                payload.append((name, path.read_bytes(), mode))
    payload.append(('usr/share/licenses/luci-app-xdvpn/LICENSE', (ROOT / 'LICENSE').read_bytes(), 0o644))
    control = f'''Package: luci-app-xdvpn
Version: 0.7.3-1
Architecture: all
Maintainer: XDVPN OpenWrt contributors
Section: luci
Priority: optional
License: MIT
Depends: luci-base, openconnect, ca-bundle, xdvpn-hillstone
Installed-Size: {sum(len(data) for _, data, _ in payload)}
Description: XDVPN OpenConnect connection verification panel (experimental)
'''.encode()
    postinst = b'''#!/bin/sh
[ -n "$IPKG_INSTROOT" ] && exit 0
/bin/sh /etc/uci-defaults/90-xdvpn || exit 1
rm -f /etc/uci-defaults/90-xdvpn
rm -f /tmp/luci-indexcache /tmp/luci-indexcache.*.json
/etc/init.d/rpcd reload
exit 0
'''
    prerm = b'''#!/bin/sh
[ -n "$IPKG_INSTROOT" ] && exit 0
# Preserve settings on upgrades; stop before removing the event hook.
if [ "$(uci -q get network.xdvpn.xdvpn_owned)" = 1 ]; then
    ifdown xdvpn
    if [ "$1" != upgrade ]; then
        /usr/libexec/xdvpn/priority remove || exit 1
        uci set network.xdvpn.auto=0
        uci commit network
    fi
fi
exit 0
'''
    data = archive(payload)
    result = archive([
        ('debian-binary', b'2.0\n', 0o644),
        ('control.tar.gz', archive([('control', control, 0o644), ('postinst', postinst, 0o755), ('prerm', prerm, 0o755)]), 0o644),
        ('data.tar.gz', data, 0o644),
    ])
    dist = ROOT / 'dist'
    dist.mkdir(exist_ok=True)
    target = dist / 'luci-app-xdvpn_0.7.3-1_all.ipk'
    target.write_bytes(result)
    (dist / 'SHA256SUMS').write_text(hashlib.sha256(result).hexdigest() + '  ' + target.name + '\n')
    print(f'{target}: {len(result)} bytes (excludes OpenConnect and dependencies)')


if __name__ == '__main__':
    main()
