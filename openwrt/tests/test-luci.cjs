global.L = {resource: p => '/luci-static/resources/' + p};
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');

// Optionally use the target router's real uci.js to verify its API contract.
const sections = {
    lan: { '.name': 'lan', '.type': 'interface' },
    wan: { '.name': 'wan', '.type': 'interface' },
    xdvpn: { '.name': 'xdvpn', '.type': 'interface' },
    loopback: { '.name': 'loopback', '.type': 'interface' },
    bridge: { '.name': 'bridge', '.type': 'device' }
};
const rpcCalls = [];
const rpc = { declare: spec => (...args) => { rpcCalls.push([spec.object, spec.method, ...args]); return Promise.resolve(spec.method === 'commit' ? 0 : {status:'refresh_requested'}); } };
const uci = process.argv[2] ? new Function('rpc', 'baseclass', fs.readFileSync(process.argv[2], 'utf8'))(
    rpc, { extend: value => { value.__init__(); return value; } }
) : {
    get: (config, name, option) => option == null ? sections[name] : sections[name]?.[option],
    set: (config, name, option, value) => { sections[name][option] = value; },
    unset: (config, name, option) => { delete sections[name][option]; },
    sections: (config, type, cb) => Object.values(sections).filter(s => s['.type'] === type).forEach(cb)
};
if (uci.state) uci.state.values.network = sections;
const options = {};
const form = { Map: function() {
    this.section = () => ({ option: (type, name) => options[name] = {
        choices: [], value(value) { this.choices.push(value); }, depends() {},
        formvalue() { return this.current; }
    }});
    this.render = () => Promise.resolve({});
}};
const source = fs.readFileSync(path.join(__dirname, '../package/luci-app-xdvpn/htdocs/luci-static/resources/view/xdvpn/main.js'), 'utf8');
const renderedNodes = [];
const view = new Function('view', 'form', 'uci', 'rpc', 'poll', 'ui', 'E', source)(
    { extend: value => value }, form, uci, rpc, { add() {} }, { addNotification() {} }, (tag, attrs, children) => { const node={tag,attrs,children};renderedNodes.push(node);return node; }
);
view.render().then(async () => {
    assert.equal(options.xdvpn_certpin, undefined, 'no manual fingerprint field');
    assert.ok(options._certificate);
    assert.equal(options.xdvpn_domains.validate('xdvpn', 'service.example.com'), true);
    for (const value of ['https://example.com', '*.example.com', '127.0.0.1', 'a..com'])
        assert.notEqual(options.xdvpn_domains.validate('xdvpn', value), true);
    assert.equal(options.xdvpn_domains.validate('xdvpn', 'example.com\nservice.example.org\r\n'), true);
    options.xdvpn_domains.write('xdvpn', 'Example.com\nexample.com\nservice.example.org');
    assert.deepEqual(uci.get('network','xdvpn','xdvpn_domains'), ['example.com','service.example.org']);
    assert.equal(options.xdvpn_domains.cfgvalue('xdvpn'), 'example.com\nservice.example.org');
    assert.equal(options.xdvpn_networks.validate('xdvpn', '10.1.2.3\n10.2.0.0/16'), true);
    assert.notEqual(options.xdvpn_networks.validate('xdvpn', '10.1.2.3\n999.0.0.1'), true);
    assert.deepEqual(options.xdvpn_mode.choices, ['networks', 'company', 'global4']);
    for (const key of ['xdvpn_domains', 'xdvpn_networks', 'xdvpn_protect_anthropic'])
        assert.equal(options[key].retain, true, key + ' must survive mode changes');
    assert.equal(options.xdvpn_blacklist.validate('xdvpn', 'example.com\n192.0.2.1\n192.0.2.0/24'), true);
    assert.notEqual(options.xdvpn_blacklist.validate('xdvpn','https://example.com'),true);
    assert.notEqual(options.xdvpn_blacklist.validate('xdvpn','0.0.0.0/0'),true);
    options.xdvpn_blacklist.write('xdvpn','Example.COM\nexample.com');
    assert.deepEqual(uci.get('network','xdvpn','xdvpn_blacklist'), ['example.com']);
    options.xdvpn_blacklist.write('xdvpn','');
    assert.equal(uci.get('network','xdvpn','xdvpn_blacklist'),undefined);
    assert.equal(options.xdvpn_protect_anthropic.default, '1');
    assert.notEqual(options.xdvpn_protect_anthropic.readonly, true);
    const field = options.interface;
    assert.deepEqual(field.choices.sort(), ['lan', 'wan']);
    assert.equal(field.validate('xdvpn', 'lan'), true);
    assert.equal(field.validate('xdvpn', 'wan'), true);
    for (const value of ['xdvpn', 'loopback', 'bridge', 'missing', ''])
        assert.notEqual(field.validate('xdvpn', value), true, value);
    assert.equal(options.auto.rmempty, false);
    options.server.current = 'vpn.example.com';
    options.port.current = '8443';
    options.server.write('xdvpn', options.server.current);
    assert.equal(uci.get('network', 'xdvpn', 'uri'), 'https://vpn.example.com:8443');
    options.port.current = '4443';
    options.port.write('xdvpn', '4443');
    assert.equal(uci.get('network', 'xdvpn', 'uri'), 'https://vpn.example.com:4443');
    options.port.current = '';
    options.port.remove('xdvpn');
    assert.equal(uci.get('network', 'xdvpn', 'uri'), 'https://vpn.example.com:8443');
    assert.equal(uci.get('network','xdvpn','port'),'8443');
    options.port.getUIElement = () => ({setValue: v => {options.port.current=v;}});
    options.proto.current='xdvpn_hillstone';
    options.proto.onchange(null,'xdvpn','xdvpn_hillstone');
    assert.equal(options.port.current,'4433');
    options.proto.current='openconnect';
    options.proto.onchange(null,'xdvpn','openconnect');
    assert.equal(options.port.current,'8443');
    options.port.current='9443';options.port.write('xdvpn','9443');
    assert.equal(uci.get('network','xdvpn','port'),'9443');
    assert.equal(options.port.cfgvalue('xdvpn'),'9443');
    rpcCalls.length = 0;
    view.handleSave = async () => { rpcCalls.push(['form', 'save']); };
    uci.set("network","xdvpn","xdvpn_mode","global4");
    await view.handleSaveApply();
    const metaMode = renderedNodes.find(n => n.tag === "div" && Array.isArray(n.children) && n.children[0]?.tag === "dt" && n.children[0].children === "已保存模式");
    assert.equal(metaMode.children[1].textContent,"IPv4 全局","saved mode overview must refresh");
    assert.deepEqual(rpcCalls, [['form','save'], ['uci','commit','network'], ['xdvpn','reload_rules']]);
    console.log('Interface dropdown and validation: PASS' + (process.argv[2] ? ' (target LuCI UCI implementation)' : ''));
}).catch(error => { console.error(error); process.exitCode = 1; });
