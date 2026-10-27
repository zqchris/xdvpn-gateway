'use strict';
'require view';
'require form';
'require uci';
'require rpc';
'require poll';
'require ui';

var priorityStatus = rpc.declare({ object: 'xdvpn', method: 'priority_status', expect: { '': {} } });
var getStatus = rpc.declare({ object: 'network.interface.xdvpn', method: 'status', expect: { '': {} } });
var certificate = rpc.declare({ object: 'xdvpn', method: 'certificate', expect: { '': {} } });
var activity = rpc.declare({ object: 'xdvpn', method: 'activity', expect: { '': {} } });
var companyRulesInfo = rpc.declare({ object: 'xdvpn', method: 'company_rules', expect: { '': {} } });
var importCompanyRules = rpc.declare({ object: 'xdvpn', method: 'import_company_rules', params: ['content'], expect: { '': {} } });
var inspectDomain = rpc.declare({ object: 'xdvpn', method: 'domain', params: ['domain'], expect: { '': {} } });
var up = rpc.declare({ object: 'network.interface.xdvpn', method: 'up' });
var down = rpc.declare({ object: 'network.interface.xdvpn', method: 'down' });

var commitNetwork = rpc.declare({ object: 'uci', method: 'commit', params: ['config'] });
var reloadRules = rpc.declare({ object: 'xdvpn', method: 'reload_rules', expect: { '': {} } });
return view.extend({
    handleSaveApply: function(ev) {
        var page = this;
        return this.handleSave(ev).then(function() { return commitNetwork('network'); }).then(function(result) {
            if (typeof result === 'number' && result !== 0) throw new Error('保存失败：' + result);
            return reloadRules();
        }).then(function(result) {
            if (page.refreshOverview) page.refreshOverview();
            ui.addNotification(null, E('p', {}, result.status === 'offline' ? '配置已保存；VPN 未连接，连接后生效。' : '已保存并通知核心立即刷新分流规则，无需重连。服务器、账号或连接核心变更仍需重新连接。'));
        }).catch(function(err) { ui.addNotification(null, E('p', {}, '保存并刷新失败：' + err.message)); });
    },
    load: function() { return uci.load('network'); },
    render: function() {
        var page = this;
        var m = new form.Map('network', '', '');
        var s = m.section(form.NamedSection, 'xdvpn', 'interface', '分流设置');
        s.addremove = false;
        function lines(value) { return String(value || '').split(/\r?\n/).map(function(v){return v.trim();}).filter(Boolean); }
        function multiline(option, key) {
            option.cfgvalue = function(section) {
                var v=uci.get('network', section, key);
                return (Array.isArray(v)?v:String(v||'').split(/\s+/)).filter(Boolean).join('\n');
            };
            option.write = function(section, value) {
                var unique=Array.from(new Set(lines(value).map(function(v){return v.toLowerCase().replace(/\.$/,'');})));
                if(unique.length) uci.set('network', section, key, unique); else uci.unset('network', section, key);
            };
        }
        var o;
        o = s.option(form.ListValue, 'xdvpn_mode', '流量模式');
        o.value('networks', '仅手动白名单 · 其他交还原网络');
        o.value('company', '公司预设 + 手动补充');
        o.value('global4', '全局 · 全部 IPv4 走公司');
        o.description = '白名单外的流量交还原网络。公司预设使用本机 IT 清单并叠加下方补充项，不采用服务器全网路由；全局忽略 Claude 保护，但仍遵守其他黑名单，并阻断外部 IPv6。';
        o = s.option(form.Flag, 'xdvpn_protect_anthropic', '禁止 Anthropic / Claude 走公司 VPN');
        o.depends('xdvpn_mode', 'networks'); o.depends('xdvpn_mode', 'company');
        o.default = '1'; o.rmempty = false; o.retain = true;
        o.description = '默认开启，排除优先。内置 Claude / Anthropic 域名并合并 OpenClash 相关规则；全局不启用，列表暂不可编辑。';
        o = s.option(form.TextValue, 'xdvpn_domains', '公司域名白名单');
        o.rows = 6;
        o.placeholder = 'corp.example.org\nexample.com\napps.example.org';
        multiline(o, 'xdvpn_domains');
        o.depends('xdvpn_mode', 'networks'); o.depends('xdvpn_mode', 'company'); o.retain = true;
        o.validate = function(section, value) {
            var names = lines(value);
            if (names.length > 64) return '最多填写 64 个域名';
            for (var i = 0; i < names.length; i++) {
                var name = names[i].replace(/\.$/, '');
                if (!(name.length <= 253 && name.indexOf('.') > 0 && !/^[0-9.]+$/.test(name) && name.split('.').every(function(label) {
                    return /^[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?$/.test(label);
                }))) return '第 ' + (i + 1) + ' 行不是有效域名；不要带协议、路径或通配符';
            }
            return true;
        };
        var validateDomain = o.validate;
        o.description = '每行一个域名，支持批量粘贴；父域名包含所有子域名。保存并应用立即更新，最多 64 项。客户端需使用路由器 DNS，共享 IP 可能一并分流。';
        o = s.option(form.TextValue, 'xdvpn_networks', '补充 IP / 网段');
        o.rows = 4; o.placeholder = '每行一个 IPv4 或 CIDR，可留空';
        multiline(o, 'xdvpn_networks');
        o.depends('xdvpn_mode', 'networks'); o.depends('xdvpn_mode', 'company'); o.retain = true;
        o.validate = function(section, value) {
            var items = lines(value);
            if (items.length > 512) return '最多填写 512 个 IP / 网段';
            for (var i=0; i<items.length; i++) {
                var parts=items[i].split('/'), octets=parts[0].split('.');
                if (parts.length>2 || octets.length!==4 || !octets.every(function(n){return /^\d{1,3}$/.test(n)&&+n<=255;}) ||
                    (parts.length===2 && !/^(?:[1-9]|[12][0-9]|3[0-2])$/.test(parts[1]))) return '第 '+(i+1)+' 行不是有效 IPv4 / 网段（/1 到 /32）';
            }
            return true;
        };
        var validateNetwork = o.validate;
        o.description = '填写单个 IPv4 或 CIDR 网段；单个 IP 自动按 /32 处理。手动与公司预设模式均使用这些条目。';
        o = s.option(form.DummyValue, '_company_rules', '公司预设清单');
        o.depends('xdvpn_mode', 'company');
        o.renderWidget = function() {
            var info=E('span',{},'正在检查本机预设…');
            companyRulesInfo().then(function(r){info.textContent=r.valid?'已就绪：'+r.ip_count+' 个 IP / 网段，'+r.domain_count+' 个域名。':(r.error||'暂未读取到预设状态');}).catch(function(){info.textContent='无法读取预设状态，请检查本机文件。';});
            var file = E('input', { type: 'file', accept: '.txt,text/plain', 'aria-label': '选择公司规则文件', style: 'max-width:100%' });
            var button = E('button', { type: 'button', 'class': 'cbi-button', click: function() {
                var selected = file.files && file.files[0];
                if (!selected || selected.size > 262144) {
                    info.textContent = '请选择不超过 256 KiB 的 UTF-8 文本文件。'; return;
                }
                button.disabled = true;
                info.textContent = '正在校验并导入…';
                var reader = new FileReader();
                reader.onerror = function() { info.textContent = '文件读取失败，请重新选择。'; button.disabled = false; };
                reader.onload = function() {
                    importCompanyRules(reader.result).then(function(result) {
                        if (!result.valid) throw new Error(result.error || '导入失败');
                        info.textContent = '已导入：' + result.ip_count + ' 个 IP / 网段，' + result.domain_count + ' 个域名。' +
                            (result.refresh === 'refresh_requested' ? '已通知当前连接刷新；按已保存的流量模式生效。' :
                            result.refresh === 'refresh_failed' ? '文件已保存，但刷新通知失败，请点击保存并应用重试。' : '连接 VPN 后按已保存的流量模式生效。');
                        file.value = '';
                    }).catch(function(err) { info.textContent = err.message || '导入失败，请重试。'; })
                      .then(function() { button.disabled = false; });
                };
                reader.readAsText(selected, 'UTF-8');
            } }, '导入并替换预设');
            return E('div', {}, [E('p', {}, info), file, button,
                E('p', { 'class': 'xd-muted' }, '每行一个 IP、网段或域名。导入会替换原预设，不改变手动补充项；格式错误保留原文件。文件只发送到当前路由器，并会随路由器系统备份保存。')]);
        };
        o.description = '读取路由器 /etc/xdvpn/company-rules.txt，支持 IP、网段和域名。与公开源码和安装包分离；更新文件后保存并应用立即重载。清单缺失、为空或无效时拒绝启用公司预设，不回退为全局。';
        o = s.option(form.Flag, 'auto', '路由器启动时连接'); o.default = '0'; o.rmempty = false;

        s = m.section(form.NamedSection, 'xdvpn', 'interface', '其他黑名单');
        s.addremove = false;
        o = s.option(form.TextValue, 'xdvpn_blacklist', '禁止走公司 VPN');
        o.rows = 6; o.placeholder = '每行一个域名、IPv4 或网段';
        multiline(o, 'xdvpn_blacklist');
        o.validate = function(section,value) {
            var items=lines(value); if(items.length>256)return '最多填写 256 项';
            for(var i=0;i<items.length;i++)if(validateDomain(section,items[i])!==true&&validateNetwork(section,items[i])!==true)return '第 '+(i+1)+' 行不是有效域名、IPv4 或网段';
            return true;
        };
        o.description = '独立于 Claude 保护开关，在全部模式（含全局）优先排除，交还 OpenClash / Passwall 或原网络。父域名包含子域名；每行一项，保存并应用立即生效。域名依赖路由器 DNS，共享 IP 可能一并排除；已知 IP 可直接填写。';

        s = m.section(form.NamedSection, 'xdvpn', 'interface', '连接与账号');
        s.addremove = false;
        o = s.option(form.ListValue, 'proto', '连接核心');
        o.value('xdvpn_hillstone', 'Hillstone SCVPN');
        o.value('openconnect', 'OpenConnect'); o.rmempty = false;
        var protocolOption = o;
        function defaultPort(proto) { return proto === 'xdvpn_hillstone' ? '4433' : '8443'; }
        function selectedPort(section) { return defaultPort(protocolOption.formvalue(section) || uci.get('network',section,'proto')); }
        o = s.option(form.Value, 'server', 'VPN 服务器');
        o.datatype = 'host'; o.rmempty = false;
        o.description = '填写主机名或 IPv4 地址，不带 https:// 或路径。';
        var serverOption = o;
        o = s.option(form.Value, 'port', '端口'); o.datatype = 'port'; o.placeholder = defaultPort(uci.get('network','xdvpn','proto'));
        o.description = '切换核心时自动填入默认端口：Hillstone 4433，OpenConnect 8443；仍可手动修改。';
        var portOption = o;
        portOption.cfgvalue = function(section) { return uci.get('network',section,'port') || defaultPort(uci.get('network',section,'proto')); };
        protocolOption.onchange = function(ev,section,value) {
            var port = defaultPort(value);
            portOption.placeholder = port;
            var widget = portOption.getUIElement(section);
            if(widget) widget.setValue(port);
        };
        function syncEndpoint(section) {
            var host = serverOption.formvalue(section);
            var port = portOption.formvalue(section) || selectedPort(section);
            uci.set('network', section, 'uri', 'https://' + host + ':' + port);
        }
        serverOption.write = function(section, value) {
            uci.set('network', section, 'server', value);
            syncEndpoint(section);
        };
        portOption.write = function(section, value) {
            uci.set('network', section, 'port', value || selectedPort(section));
            syncEndpoint(section);
        };
        portOption.remove = function(section) {
            uci.set('network', section, 'port', selectedPort(section));
            syncEndpoint(section);
        };
        o = s.option(form.Button, '_certificate', '服务器证书');
        o.depends('proto', 'xdvpn_hillstone');
        o.inputtitle = '查看 / 确认信任'; o.inputstyle = 'action';
        o.description = '先保存并应用服务器地址。证书自动获取，不发送账号密码；有效 CA 证书自动验证，自签名证书仅需首次确认。';
        o.onclick = function() { return inspectCertificate(false); };
        o = s.option(form.Value, 'username', '用户名'); o.rmempty = false;
        o = s.option(form.Value, 'password', '密码'); o.password = true; o.rmempty = false;
        o.description = '保存在路由器 root 可读的网络配置中；导出系统备份会包含密码。';
        o = s.option(form.ListValue, 'vpn_protocol', '协议');
        o.depends('proto', 'openconnect');
        ['anyconnect', 'nc', 'gp', 'pulse', 'f5', 'fortinet', 'array'].forEach(function(p) { o.value(p, p); });
        s.option(form.Value, 'authgroup', '认证组（可选）').depends('proto', 'openconnect');
        s.option(form.Value, 'usergroup', '登录路径 / 用户组（可选）').depends('proto', 'openconnect');
        o = s.option(form.ListValue, 'interface', '连接 VPN 所用的上网接口');
        o.rmempty = false;
        uci.sections('network', 'interface', function(iface) {
            if (iface['.name'] !== 'xdvpn' && iface['.name'] !== 'loopback')
                o.value(iface['.name'], iface['.name']);
        });
        o.description = '选择路由器当前用来上网的接口。旁路由通常为 lan，主路由通常为 wan；保留已配置的值即可。';
        o.validate = function(section, value) {
            return value !== 'xdvpn' && value !== 'loopback' && uci.get('network', value, '.type') === 'interface'
                ? true : '请选择列表中的现有上网接口';
        };
        o = s.option(form.Flag, 'no_dtls', '禁用 DTLS（兼容排障）'); o.default = '0'; o.depends('proto', 'openconnect');
        o = s.option(form.Value, 'mtu', 'MTU（可选）'); o.datatype = 'range(576,1500)'; o.placeholder = '1406'; o.depends('proto', 'openconnect');

        function inspectCertificate(connect) {
            return certificate().then(function(info) {
                if (info.error) throw new Error(info.error);
                if (!/^[a-f0-9]{64}$/.test(info.sha256 || '')) throw new Error('证书信息无效');
                var endpoint = serverOption.formvalue('xdvpn') + ':' + (portOption.formvalue('xdvpn') || selectedPort('xdvpn'));
                if (info.endpoint !== endpoint) throw new Error('请先保存并应用服务器地址和端口，再检查证书');
                var pinned = uci.get('network', 'xdvpn', 'xdvpn_certpin');
                var bound = uci.get('network', 'xdvpn', 'xdvpn_cert_endpoint') === endpoint;
                if (connect && ((pinned && bound && info.saved_pin_matches) || (!pinned && info.ca_valid))) return up();
                var changed = pinned && (!bound || !info.saved_pin_matches);
                ui.showModal('确认服务器身份', [
                    E('p', {}, '服务器：' + info.endpoint),
                    E('p', {}, changed ? '证书或服务器已变化。请向管理员核实后再重新信任。' :
                        (info.ca_valid ? '证书已通过 CA 和服务器名称验证。' : '此证书无法通过 CA 验证。请向管理员核实服务器身份后确认信任。')),
                    E('p', {}, '证书主体：' + info.subject),
                    E('p', {}, '有效期至：' + info.expires),
                    E('details', {}, [E('summary', {}, '查看 SHA256 指纹'), E('p', { style: 'overflow-wrap:anywhere' }, info.sha256)]),
                    E('div', { 'class': 'right' }, [
                        E('button', { 'class': 'cbi-button', click: ui.hideModal }, '取消'), ' ',
                        E('button', { 'class': 'cbi-button cbi-button-positive', click: function() {
                            if (info.ca_valid) uci.unset('network', 'xdvpn', 'xdvpn_certpin');
                            else uci.set('network', 'xdvpn', 'xdvpn_certpin', info.sha256);
                            uci.set('network', 'xdvpn', 'xdvpn_cert_endpoint', info.endpoint);
                            return m.save().then(function() {
                                ui.hideModal();
                                ui.addNotification(null, E('p', {}, '信任设置已暂存，请点击“保存并应用”，然后连接。'));
                            }).catch(function(err) { ui.addNotification(null, E('p', {}, '保存失败：' + err.message)); });
                        } }, info.ca_valid ? '使用 CA 验证' : '已核实，信任此证书')
                    ])
                ]);
            }).catch(function(err) { ui.addNotification(null, E('p', {}, '证书检查失败：' + err.message)); });
        }
        function connectVPN() {
            return uci.get('network', 'xdvpn', 'proto') === 'xdvpn_hillstone' ? inspectCertificate(true) : up();
        }
        var priorityDetail = E('p', { 'class': 'xd-status-detail', role: 'status' }, '尚未检查分流优先级');
        function checkPriority() {
            priorityDetail.textContent = '正在检查分流优先级…';
            return priorityStatus().then(function(info) {
                var messages = {
                    inactive: '分流尚未启用；连接 VPN 后检查。',
                    ordered: 'XDVPN 入口与策略路由顺序正常；DNS、Fake-IP/TUN 及实际访问仍需验证。',
                    review_required: 'XDVPN 自有顺序正常；发现其他策略路由或独立入口，兼容性需要确认，不能保证全部流量优先。',
                    repair_needed: '分流优先级异常：入口、路由或顺序缺失。请检查日志；VPN 已连接不代表分流正常。'
                };
                priorityDetail.textContent = messages[info.status] || '检查结果未知，请重试。';
            }).catch(function() { priorityDetail.textContent = '优先级检查失败，请刷新重试。'; });
        }
        var priorityWasUp = false;
        var statusBadge = E('span', { 'class': 'xd-status-badge loading' }, '读取中');
        var statusDetail = E('p', { 'class': 'xd-status-detail' }, '正在读取连接状态…');
        function setStatus(state, label, detail) {
            if (statusBadge.className !== 'xd-status-badge ' + state) statusBadge.className = 'xd-status-badge ' + state;
            if (statusBadge.textContent !== label) statusBadge.textContent = label;
            if (statusDetail.textContent !== detail) statusDetail.textContent = detail;
        }
        function action(fn) {
            return function() {
                return fn().then(function(result) {
                    if (typeof result === 'number' && result !== 0) throw new Error('RPC 返回 ' + result);
                }).catch(function(err) { ui.addNotification(null, E('p', {}, '操作失败：' + err.message)); });
            };
        }
        poll.add(function() {
            if (typeof document !== 'undefined' && document.hidden) return Promise.resolve();
            if (page.refreshOverview) page.refreshOverview();
            return getStatus().then(function(data) {
                if (data.proto && ['openconnect', 'xdvpn_hillstone'].indexOf(data.proto) < 0) {
                    setStatus('error', '需要处理', 'VPN 协议尚未被网络服务加载；首次安装后需要重启网络服务（会短暂中断网络）。');
                    return;
                }
                if (data.up && !priorityWasUp) checkPriority();
                priorityWasUp = !!data.up;
                if (!data.up) priorityDetail.textContent = 'VPN 未连接，分流优先级尚未生效。';
                var uptime = data.uptime || 0;
                var duration = uptime >= 3600 ? Math.floor(uptime / 3600) + ' 小时 ' + Math.floor((uptime % 3600) / 60) + ' 分钟' : Math.floor(uptime / 60) + ' 分钟';
                setStatus(data.up ? 'online' : (data.pending ? 'pending' : 'offline'), data.up ? '已连接' : (data.pending ? '连接中' : '未连接'),
                    data.up ? '连接已持续 ' + duration : (data.pending ? '正在建立安全连接，请稍候。' : 'VPN 当前已断开，可检查设置后重新连接。'));
                if (data.errors && data.errors.length) statusDetail.textContent += ' · ' + JSON.stringify(data.errors);
            }).catch(function() { setStatus('error', '接口异常', '接口尚未加载，请按安装说明重新加载网络配置。'); });
        }, 15);
        var logList = E('div', { 'class': 'xd-log-list', 'aria-live':'polite' }, '打开日志后读取最近的重要事件。');
        var logSummary = E('p', { 'class':'xd-muted' }, '访问记录保存在有限大小的内存里；点击刷新立即读取。');
        var cachedEvents=[];
        var logSearch=E('input',{type:'search',placeholder:'搜索域名或 IP','aria-label':'搜索日志',input:function(){drawLogs();}});
        var filter=E('select', {'aria-label':'筛选日志', change:function(){drawLogs();}}, [
            E('option',{value:'all'},'全部重要事件'),E('option',{value:'access'},'VPN 访问'),E('option',{value:'connection'},'连接与断开'),E('option',{value:'rules'},'规则刷新'),E('option',{value:'warning'},'异常与 DNS')
        ]);
        function replace(node, children) { while(node.firstChild) node.removeChild(node.firstChild); children.forEach(function(c){node.appendChild(c);}); }
        function drawLogs() {
            var choice=filter.value||'all'; var search=(logSearch.value||'').trim().toLowerCase();
            var items=cachedEvents.filter(function(v){return !search||JSON.stringify(v).toLowerCase().indexOf(search)>=0;}).filter(function(v){return choice==='all'||(choice==='access'&&v.kind==='access')||(choice==='connection'&&['connected','disconnected'].indexOf(v.kind)>=0)||(choice==='rules'&&v.kind==='rules')||(choice==='warning'&&['warning','dns'].indexOf(v.kind)>=0);});
            replace(logList,items.length?items.map(function(v){return E('div',{'class':'xd-event'},[E('span',{'class':'xd-event-dot '+v.kind,'aria-hidden':'true'},''),E('div',{},[E('div',{'class':'xd-event-title'},v.text),E('time',{'class':'xd-muted'},v.time),v.access&&v.access.outer?E('p',{'class':'xd-muted'},'检测到 ECH 扩展：此处是 TLS 外层域名，可能是兼容性占位或掩护域名，不能确认实际访问域名。'):E('span',{},''),v.access?E('p',{'class':'xd-muted'},'目标 IP：'+v.access.ips.join('、')+' · 首次 '+v.access.first+' · 最近 '+v.access.last+(v.access.domains&&v.access.domains.length?' · 域名：'+v.access.domains.join('、'):'')):E('span',{},'')])]);}):[E('p',{'class':'xd-empty'},'暂无符合条件的重要事件。系统日志重启后可能清空。')]);
        }
        var refreshButton=E('button',{'class':'cbi-button',click:function(){return loadLogs();}},'刷新日志');
        function loadLogs() {
            refreshButton.disabled=true; logSummary.textContent='正在读取…';
            return activity().then(function(result){if(result.error)throw new Error(result.error);cachedEvents=result.events||[];drawLogs();logSummary.textContent=result.note||'已更新';})
                .catch(function(err){logSummary.textContent='读取日志失败：'+err.message;})
                .finally(function(){refreshButton.disabled=false;});
        }
        var domainInput=E('input',{type:'text',placeholder:'例如 console.example.com','aria-label':'要检查的域名',autocomplete:'off',spellcheck:'false'});
        var domainResult=E('div',{'class':'xd-result','aria-live':'polite'},'按需查询当前规则，不发送网站访问请求。DNS 查询会更新匹配的地址集合。');
        var domainButton=E('button',{'class':'cbi-button cbi-button-action',click:function(){
            domainButton.disabled=true;domainResult.textContent='正在解析并检查规则…';
            return inspectDomain(domainInput.value.trim()).then(function(result){
                if(result.error)throw new Error(result.error);
                var labels={vpn:'命中 VPN 规则',fallback:'交还原网络',protected:'保护排除'};
                replace(domainResult,(result.results||[]).map(function(v){return E('div',{'class':'xd-route-result'},[E('span',{'class':'xd-route-label '+v.decision},labels[v.decision]||'无法判断'),E('code',{},v.ip),E('p',{},v.reason)]);}).concat([E('p',{'class':'xd-muted'},result.note)]));
            }).catch(function(err){domainResult.textContent='查询未完成：'+err.message;}).finally(function(){domainButton.disabled=false;});
        }},'检查去向');
        return m.render().then(function(formNode) {
            function sectionSummary(title, description) {
                return E('summary', {}, [E('span', { 'class': 'xd-summary-copy' }, [E('span', { 'class': 'xd-summary-title' }, title), E('span', { 'class': 'xd-summary-description' }, description)]), E('span', { 'class': 'xd-summary-action', 'aria-hidden': 'true' }, '')]);
            }
            var blacklistSection;
            if (formNode.querySelectorAll) {
                Array.from(formNode.querySelectorAll('.cbi-section')).forEach(function(section) {
                    var heading = section.querySelector('h3');
                    if (heading && heading.textContent === '其他黑名单') blacklistSection = section;
                    if (heading && heading.textContent === '分流设置') {
                        var routingDetails = E('details', { 'class': 'xd-routing xd-connection' }, [sectionSummary('分流与保护', '选择流量模式，管理公司白名单')]);
                        section.parentNode.insertBefore(routingDetails, section);
                        routingDetails.appendChild(section);
                        heading.hidden = true;
                    }
                    if (heading && heading.textContent === '连接与账号') {
                        var details = E('details', { 'class': 'xd-connection xd-account' }, [sectionSummary('连接与账号', '服务器、身份验证与连接选项')]);
                        section.parentNode.insertBefore(details, section.parentNode.firstChild);
                        details.appendChild(section);
                        heading.hidden = true;
                        if (!uci.get('network', 'xdvpn', 'server') || !uci.get('network', 'xdvpn', 'username') || !uci.get('network', 'xdvpn', 'password')) details.open = true;
                    }
                });
            }
            var protectionDetails = E('details', { 'class': 'xd-connection xd-protection' }, [
                sectionSummary('Claude 保护范围', '查看内置域名与只读规则来源'),
                E('p', { 'class': 'xd-protection-copy' }, '内置 anthropic.com、claude.ai、claude.com、claude.app、claudeusercontent.com、claudemcpcontent.com，包含子域名。另只读合并 OpenClash 相关域名规则；内置列表目前不可逐条编辑。')
            ]);
            var settings = E('div', { 'class': 'xd-settings', id: 'xd-settings-panel', role: 'tabpanel', 'aria-labelledby': 'xd-settings-tab' }, [formNode, protectionDetails]);
            if (blacklistSection) {
                var blacklistDetails = E('details', { 'class': 'xd-blacklist xd-connection' }, [sectionSummary('其他黑名单', '这些目标始终交还原网络')]);
                blacklistSection.parentNode.insertBefore(protectionDetails, blacklistSection);
                blacklistSection.parentNode.insertBefore(blacklistDetails, blacklistSection);
                blacklistDetails.appendChild(blacklistSection);
                blacklistSection.querySelector('h3').hidden = true;
            }
            var logs = E('div', { 'class': 'xd-logs', id: 'xd-logs-panel', role: 'tabpanel', 'aria-labelledby': 'xd-logs-tab', hidden: true }, [
                E('section', { 'class': 'xd-card xd-domain-card' }, [
                    E('div', { 'class': 'xd-section-heading' }, [E('div', {}, [E('h3', {}, '域名去向'), E('p', { 'class': 'xd-section-description' }, '检查当前规则，不发起网站访问')]), E('span', { 'class': 'xd-quiet-badge' }, '按需检查')]),
                    E('div', { 'class': 'xd-domain-search' }, [domainInput, domainButton]), domainResult
                ]),
                E('section', { 'class': 'xd-card xd-log-card' }, [
                    E('div', { 'class': 'xd-section-heading' }, [E('div', {}, [E('h3', {}, '运行日志'), E('p', { 'class': 'xd-section-description' }, '连接事件与实际访问记录')]), E('div', { 'class': 'xd-log-tools' }, [filter, refreshButton])]),
                    logSummary, logSearch, logList
                ])
            ]);
            function switchTab(which) {
                var showLogs = which === 'logs';
                settings.hidden = showLogs;
                logs.hidden = !showLogs;
                settingsTab.setAttribute('aria-selected', String(!showLogs));
                logsTab.setAttribute('aria-selected', String(showLogs));
                settingsTab.tabIndex = showLogs ? -1 : 0;
                logsTab.tabIndex = showLogs ? 0 : -1;
                var footer = document.querySelector('.cbi-page-actions');
                if (footer) footer.hidden = showLogs;
                if (showLogs) return loadLogs();
            }
            function tabKeydown(ev) {
                if (['ArrowLeft', 'ArrowRight', 'Home', 'End'].indexOf(ev.key) < 0) return;
                ev.preventDefault();
                var next = ev.key === 'Home' ? settingsTab : (ev.key === 'End' ? logsTab : (this === settingsTab ? logsTab : settingsTab));
                switchTab(next === logsTab ? 'logs' : 'settings');
                next.focus();
            }
            var settingsTab = E('button', { id: 'xd-settings-tab', type: 'button', role: 'tab', 'aria-controls': 'xd-settings-panel', 'aria-selected': 'true', keydown: tabKeydown, click: function() { switchTab('settings'); } }, '连接设置');
            var logsTab = E('button', { id: 'xd-logs-tab', type: 'button', role: 'tab', tabindex: '-1', 'aria-controls': 'xd-logs-panel', 'aria-selected': 'false', keydown: tabKeydown, click: function() { return switchTab('logs'); } }, '日志与诊断');
            var protocolText = E('dd', {}, '');
            var endpointText = E('dd', {}, '');
            var modeText = E('dd', {}, '');
            var modes = { networks: '仅白名单', company: '公司预设 + 手动补充', server: '公司预设 + 手动补充', global4: 'IPv4 全局' };
            page.refreshOverview = function() {
                protocolText.textContent = uci.get('network', 'xdvpn', 'proto') === 'openconnect' ? 'OpenConnect' : 'Hillstone SCVPN';
                var endpoint = uci.get('network', 'xdvpn', 'server') || '尚未配置';
                var port = uci.get('network', 'xdvpn', 'port') || defaultPort(uci.get('network', 'xdvpn', 'proto'));
                if (endpoint !== '尚未配置') endpoint += ':' + port;
                endpointText.textContent = endpoint;
                endpointText.title = endpoint;
                modeText.textContent = modes[uci.get('network', 'xdvpn', 'xdvpn_mode')] || '仅白名单';
            };
            page.refreshOverview();
            return E('div', { 'class': 'xdvpn-app' }, [
                E('link', { rel: 'stylesheet', href: L.resource('view/xdvpn/style_v073.css') }),
                E('section', { 'class': 'xd-header', 'aria-labelledby': 'xd-overview-title' }, [
                    E('div', { 'class': 'xd-overview' }, [
                        E('div', { 'class': 'xd-brand-row' }, [E('h2', { id: 'xd-overview-title' }, 'XDVPN'), statusBadge]),
                        E('p', { 'class': 'xd-overview-caption' }, '公司网络连接 · UI 0.7.3'),
                        E('div', { 'class': 'xd-status-copy', role: 'status', 'aria-live': 'polite' }, statusDetail)
                    ]),
                    E('div', { 'class': 'xd-actions' }, [
                        E('button', { type: 'button', 'class': 'cbi-button cbi-button-action xd-connect-button', click: action(connectVPN) }, '连接 VPN'),
                        E('button', { type: 'button', 'class': 'cbi-button xd-disconnect-button', click: action(down) }, '断开')
                    ]),
                    E('dl', { 'class': 'xd-connection-meta' }, [
                        E('div', {}, [E('dt', {}, '配置核心'), protocolText]),
                        E('div', {}, [E('dt', {}, '配置服务器'), endpointText]),
                        E('div', {}, [E('dt', {}, '已保存模式'), modeText])
                    ])
                ]),
                E('section', { 'class': 'xd-card' }, [priorityDetail, E('button', { type: 'button', 'class': 'cbi-button', click: checkPriority }, '检查分流优先级')]),
                E('div', { 'class': 'xd-tabs', role: 'tablist', 'aria-label': 'XDVPN 页面' }, [settingsTab, logsTab]), settings, logs,
                E('p', { 'class': 'xd-footnote' }, '规则保存在路由器本地。保存并应用立即刷新分流，不重连 VPN。服务器、账号或连接核心变更需重新连接。')
            ]);
        });
    }
});
