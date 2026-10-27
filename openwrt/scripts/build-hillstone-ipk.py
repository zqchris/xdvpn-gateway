#!/usr/bin/env python3
"""Build an x86_64 diagnostic release without downloading an OpenWrt SDK."""
import importlib.util, subprocess, hashlib, argparse
from pathlib import Path
parser=argparse.ArgumentParser()
parser.add_argument('--arch',choices=['x86_64','aarch64_generic','aarch64_cortex-a53'],default='x86_64')
args=parser.parse_args()
root=Path(__file__).resolve().parents[1]
spec=importlib.util.spec_from_file_location('ipk',root/'scripts/build-ipk.py')
m=importlib.util.module_from_spec(spec);spec.loader.exec_module(m)
dist=root/'dist';dist.mkdir(exist_ok=True)
binary=dist/('xdvpn-hillstone-'+args.arch)
import os
env={**os.environ,'CGO_ENABLED':'0','GOOS':'linux','GOARCH':('amd64' if args.arch=='x86_64' else 'arm64'),'GOTOOLCHAIN':'local'}
subprocess.run(['go','build','-trimpath','-ldflags=-s -w','-o',str(binary),'.'],cwd=root/'core/hillstone',env=env,check=True)
files=[('usr/sbin/xdvpn-hillstone',binary.read_bytes(),0o755),('lib/netifd/proto/xdvpn_hillstone.sh',(root/'package/xdvpn-hillstone/root/lib/netifd/proto/xdvpn_hillstone.sh').read_bytes(),0o755),('usr/share/licenses/xdvpn-hillstone/LICENSE',(root/'core/hillstone/LICENSE').read_bytes(),0o644)]
files.append(('lib/upgrade/keep.d/xdvpn',(root/'package/xdvpn-hillstone/root/lib/upgrade/keep.d/xdvpn').read_bytes(),0o644))
files.append(('usr/libexec/xdvpn/hillstone-run',(root/'package/xdvpn-hillstone/root/usr/libexec/xdvpn/hillstone-run').read_bytes(),0o755))
files.append(('etc/init.d/xdvpn-policy',(root/'package/xdvpn-hillstone/root/etc/init.d/xdvpn-policy').read_bytes(),0o755))
control=f"""Package: xdvpn-hillstone
Version: 0.7.3-1
Architecture: {args.arch}
Maintainer: XDVPN OpenWrt contributors
Section: net
Priority: optional
License: GPL-3.0-only
Depends: kmod-tun, ip, resolveip, ca-bundle
Installed-Size: {sum(len(b) for _,b,_ in files)}
Description: Lightweight Hillstone SCVPN client for XDVPN
""".encode()
out=m.archive([('debian-binary',b'2.0\n',0o644),('control.tar.gz',m.archive([('control',control,0o644),('prerm',(root/'package/xdvpn-hillstone/prerm').read_bytes(),0o755)]),0o644),('data.tar.gz',m.archive(files),0o644)])
target=dist/('xdvpn-hillstone_0.7.3-1_'+args.arch+'.ipk');target.write_bytes(out)
print(target.name,'bytes=',len(out),'sha256=',hashlib.sha256(out).hexdigest())
