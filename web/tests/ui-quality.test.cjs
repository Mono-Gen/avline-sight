const fs = require('node:fs');
const { test } = require('node:test');
const ts = require('typescript');
const { renderToStaticMarkup } = require('react-dom/server');

// 既存のTypeScript依存だけで回帰テストを動かし、テスト用パッケージの追加を避ける。
for (const extension of ['.ts', '.tsx']) {
  require.extensions[extension] = (module, filename) => {
    const result = ts.transpileModule(fs.readFileSync(filename, 'utf8'), {
      compilerOptions: {
        module: ts.ModuleKind.CommonJS,
        jsx: ts.JsxEmit.ReactJSX,
        target: ts.ScriptTarget.ES2020,
        esModuleInterop: true,
      },
      fileName: filename,
    });
    module._compile(result.outputText, filename);
  };
}

require('./ui-regressions.cjs');
require('./live-regressions.cjs');
require('./rack-quality.cjs');
require('./fix-round1.cjs');
require('./topology-quality.cjs');

const assert = require('node:assert/strict');
const React = require('react');
const path = require('node:path');
const vm = require('node:vm');
const { createRequire } = require('node:module');
const { eventDetails, eventHint, eventTitle, isRecommendationEvent, setLang, t } = require('../src/i18n/index.ts');
const { monitoringStopped, issueAlerts, newIssueAlerts } = require('../src/monitorHealth.ts');
const { OverallStatus, MonitorStopped } = require('../src/components/MonitorStatus.tsx');
const { WallSwitchTile } = require('../src/components/WallSwitchTile.tsx');
const { BrowserDisplaySettings, DataTransfers, GroupOrder } = require('../src/components/SettingsTools.tsx');
const { AppHeader } = require('../src/components/AppHeader.tsx');
const { AppSidebar } = require('../src/components/AppSidebar.tsx');
const { SwitchFilters } = require('../src/components/SwitchFilters.tsx');
const { PhaseControls } = require('../src/components/PhaseControls.tsx');
const { AlertBand, AlertEntry } = require('../src/components/AlertBand.tsx');
const { summarizeAlerts, groupWallAlerts } = require('../src/wallAlerts.ts');
const { acknowledgeableEventIds, logRows, SEARCH_EVENT_LIMIT, syslogMessage } = require('../src/events.ts');
const { LogEventRow } = require('../src/views/Logs.tsx');
const { DeviceInventoryTable } = require('../src/views/Devices.tsx');
const { VlanQuerierTable } = require('../src/views/MulticastView.tsx');
const { PollingStatus } = require('../src/components/PollingStatus.tsx');
const render = (component, props) => renderToStaticMarkup(React.createElement(component, props));
const noop = () => {};
const fault = (id, extra = {}) => ({ event_id: id, severity: 'WARNING', suppressed: false, acked: false, code: 'CUSTOM', title: 'Fault', ...extra });
const headers = html => [...html.matchAll(/<th\b[^>]*>(.*?)<\/th>/g)].map(match => match[1].replace(/<[^>]*>/g, ''));

test('shared table headers toggle numeric and natural sorting, with blanks last in both directions', () => {
  let sort;
  const { Table } = sourceWith('../src/components/Table.tsx', {
    react: { ...React, useState: () => [sort, update => { sort = update(sort); }] },
  });
  const rows = [{ port: '1/0/10', count: -2 }, { port: ' ', count: null },
    { port: '1/0/2', count: 10 }, { port: '1/0/1', count: 0 }];
  const draw = () => Table({ rows, labels: { port: 'Port', count: 'Count' } });
  const click = label => elements(draw(), node => node.type === 'button' && node.props.children[0] === label)[0].props.onClick();
  const markup = () => renderToStaticMarkup(draw());
  const ports = () => [...markup().matchAll(/<tr><td>(.*?)<\/td>/g)].map(match => match[1]);

  assert.deepEqual(ports(), ['1/0/10', ' ', '1/0/2', '1/0/1']);
  click('Port');
  assert.deepEqual(ports(), ['1/0/1', '1/0/2', '1/0/10', ' ']);
  assert.match(markup(), /aria-sort="ascending"><button[^>]*>Port<span aria-hidden="true">.*▲/);
  click('Port');
  assert.deepEqual(ports(), ['1/0/10', '1/0/2', '1/0/1', ' ']);
  assert.match(markup(), /aria-sort="descending"><button[^>]*>Port<span aria-hidden="true">.*▼/);
  click('Count');
  assert.deepEqual(ports(), ['1/0/10', '1/0/1', '1/0/2', ' ']);
  assert.equal([...markup().matchAll(/aria-sort=/g)].length, 1);
  click('Count');
  assert.deepEqual(ports(), ['1/0/2', '1/0/1', '1/0/10', ' ']);
  assert.equal(rows[1].port, ' ');
});

test('port hardware keeps labels intact and stacks values in narrow sections', () => {
  const { PortHardware } = require('../src/components/PortHardware.tsx');
  const html = render(PortHardware, { port: { poe: { detect: 'delivering' } } });
  const css = fs.readFileSync(path.join(__dirname, '../src/style.css'), 'utf8');
  assert.match(html, /<dt class="whitespace-nowrap">[^<]+<\/dt><dd>delivering<\/dd>/);
  assert.match(css, /\.port-hardware-section\s*\{[^}]*container-type: inline-size/);
  assert.match(css, /\.port-hardware-fields dd\s*\{[^}]*overflow-wrap: anywhere/);
  assert.match(css, /@container[^\{]*\{\s*\.port-hardware-fields\s*\{[^}]*grid-template-columns: max-content minmax\(0, 1fr\)/);
});

test('port details separate summary pairs and memo controls as well as device metadata and actions', () => {
  const port = {
    if_index: 1, name: '1/0/1', eee: 'disable', pause: 'disabled', udld: 'notApplicable',
    stp_state: 'forwarding', trust: 'trustIpDscp', devices: [
      { mac: '02:00:00:00:00:01', label: 'Mixer', lldp_name: 'Audio', vendor: 'Example', ip: '192.0.2.1' },
      { mac: '02:00:00:00:00:02' },
    ],
  };
  const sw = { id: 1, ports: [port] };
  const { SwitchPorts } = sourceWith('../src/views/SwitchPorts.tsx', {
    react: { useState: initial => [initial === 0 ? 1 : initial, noop], useEffect: noop },
    '../useSwitchState': { useSwitchState: () => ({ state: sw }) },
    '../usePolling': { usePolling: () => ({}) },
    '../components/Common': { FrontPanel: () => null, Chart: () => null, Table: () => null },
  });
  const html = renderToStaticMarkup(SwitchPorts({ overview: { switches: [sw] }, selected: 1, locked: true }));
  // Plan section 22: the raw port settings (EEE, PAUSE, ...) are shown only in expert view, one labelled row each.
  assert.doesNotMatch(html, /EEE|trustIpDscp/);
  const expertHtml = renderToStaticMarkup(SwitchPorts({ overview: { switches: [sw] }, selected: 1, locked: true, expert: true }));
  assert.match(expertHtml, /detail-expert[^]*<dt>EEE<\/dt><dd>disable<\/dd><dt>PAUSE<\/dt><dd>disabled<\/dd><dt>UDLD<\/dt><dd>notApplicable<\/dd><dt>STP<\/dt><dd>forwarding<\/dd><dt>Trust<\/dt><dd>trustIpDscp<\/dd>/);
  // The detail reads top to bottom: current values, devices, then the folded port settings.
  assert.match(html, /detail-title">1\/0\/1<[^]*いまの値[^]*<h4>[^<]*<\/h4>[^]*<details class="detail-settings">/);
  const memoRow = html.match(/<div class="flex flex-wrap items-center gap-2">(<label.*?)(?=<\/div>)/)[1];
  const inlineLabels = [...memoRow.matchAll(/<label class="([^"]+)">(.*?)<\/label>/g)];
  assert.deepEqual(inlineLabels.map(([, , content]) => content.match(/<input[^>]*>/)[0].match(/type="(\w+)"/)?.[1]),
    [undefined, 'checkbox', 'number']);
  for (const [, classes] of inlineLabels) assert.match(classes, /\bflex\b.*\bitems-center\b.*\bgap-2\b/);
  const devices = html.match(/<ul class="divide-y[^>]*>(.*?)<\/ul>/)[1];
  const blocks = [...devices.matchAll(/<li[^>]*>(.*?)<\/li>/g)].map(match => match[1]);
  assert.equal(blocks.length, 2);
  assert.match(blocks[0], /<strong[^>]*>Mixer<\/strong>.*<p[^>]*>02:00:00:00:00:01 · Example · 192\.0\.2\.1<\/p>/);
  assert.match(blocks[0], /gap-2.*<button[^>]*disabled[^>]*>Ping<\/button>.*href="http:\/\/192\.0\.2\.1">HTTP<\/a>.*href="https:\/\/192\.0\.2\.1">HTTPS<\/a>/);
  assert.match(blocks[1], /<strong[^>]*>02:00:00:00:00:02<\/strong>/);
  assert.doesNotMatch(blocks[1], /<button|<a | · /);
});

test('switch action popup dismisses outside, on Escape and when an item is chosen', () => {
  let open = false;
  let effect;
  let focusCount = 0;
  let actionCount = 0;
  const listeners = new Map();
  class Target {
    constructor(action = false) { this.action = action; }
    closest() { return this.action ? this : null; }
  }
  const inside = new Target(true);
  const outside = new Target();
  const popup = { contains: target => target === inside };
  const trigger = { focus() { focusCount++; } };
  const document = {
    addEventListener: (kind, handler) => listeners.set(kind, handler),
    removeEventListener(kind, handler) {
      assert.equal(listeners.get(kind), handler);
      listeners.delete(kind);
    },
  };
  let refs;
  const { ActionPopup } = sourceWith('../src/components/ActionPopup.tsx', {
    react: {
      useState: () => [open, value => { open = value; }], useId: () => 'test-items',
      useRef: () => ({ current: refs.shift() }), useEffect: callback => { effect = callback; },
      useCallback: callback => callback,
    },
  }, { document, Node: Target, Element: Target });
  const draw = () => {
    refs = [popup, trigger];
    return ActionPopup({ label: 'Core actions', children: React.createElement('button', { onClick: () => { actionCount++; } }, 'Ports') });
  };
  // An outside click must not pull focus back to the trigger; Escape and choosing an item return it.
  for (const [dismiss, focusDelta] of [
    [() => listeners.get('click')({ target: outside }), 0],
    [() => listeners.get('keydown')({ key: 'Escape', preventDefault: noop, stopPropagation: noop }), 1],
    [menu => {
      menu.props.onClick({ target: inside });
      menu.props.children.props.onClick();
      assert.equal(actionCount, 1);
    }, 1],
  ]) {
    draw().props.children[0].props.onClick({ currentTarget: trigger });
    const [button, menu] = draw().props.children;
    assert.equal(button.props['aria-expanded'], true);
    assert.equal(button.props['aria-controls'], menu.props.id);
    const cleanup = effect();
    listeners.get('click')({ target: inside });
    assert.equal(open, true);
    const priorFocus = focusCount;
    dismiss(menu);
    assert.equal(open, false);
    assert.equal(focusCount, priorFocus + focusDelta);
    cleanup();
    assert.equal(listeners.size, 0);
  }
  const rack = fs.readFileSync(path.join(__dirname, '../src/components/RackRow.tsx'), 'utf8');
  // The switch name opens the switch alone; the labelled menu keeps WebUI, terminal and edit, and deleting lives in the edit form.
  assert.match(rack, /data-action="open-switch"\s+onClick=\{\(\) => actions\.select\(sw\.id\)\}/);
  assert.match(rack, /<ActionPopup [^>]*text=\{`\$\{t\('rack\.actions'\)\} ▾`\}>/);
  assert.doesNotMatch(rack, /<ActionPopup[^]*(actions\.select|rack\.remove|'DELETE')[^]*<\/ActionPopup>/);
  const form = fs.readFileSync(path.join(__dirname, '../src/components/SwitchForm.tsx'), 'utf8');
  assert.match(form, /window\.confirm\(t\('rack\.removeConfirm'[^]*json\('DELETE'\)/);
});

test('front panel shows full short names and uses icons when a status word would crowd the tile', context => {
  context.after(() => setLang('ja'));
  setLang('en');
  const { FrontPanel } = require('../src/components/FrontPanel.tsx');
  const port = { name: '1/0/7', if_index: 7, type: 'rj45', oper: 'up' };
  const lag = { name: 'lag 1', if_index: 8, type: 'lag', oper: 'down' };
  const draw = (ports, alerts = []) => render(FrontPanel, { ports, alerts, switchId: 1, selected: 7, onSelect: noop });
  const html = draw([port, lag]);
  const tiles = [...html.matchAll(/<button\b([^>]*?)>(.*?)<\/button>/g)];
  assert.equal(tiles.length, 2);
  for (const [, attrs, contents] of tiles) {
    assert.match(attrs, /\bw-20 shrink-0\b/);
    assert.equal(attrs.match(/title="([^"]+)"/)[1], attrs.match(/aria-label="([^"]+)"/)[1]);
    assert.doesNotMatch(contents, /truncate/);
  }
  assert.match(tiles[0][1], /title="1\/0\/7 rj45 · Connected · [^"]+"/);
  assert.match(tiles[0][2], /^<span[^>]*>7<\/span>.*<span[^>]*>Connected<\/span>/);
  assert.match(tiles[1][1], /title="lag 1 lag · Disconnected · [^"]+"/);
  assert.match(tiles[1][2], /^<span[^>]*>lag 1<\/span>.*<span[^>]*>—<\/span>/);
  assert.match(draw([{ ...port, name: '1/0/123456789' }]), /break-words">123456789<\/span>/);

  const powered = { ...port, poe: { power_w: 2 } };
  const withPower = draw([powered]);
  assert.match(withPower, /title="1\/0\/7 rj45 · Connected · [^"]*PoE supplying power"/);
  assert.match(withPower, /<span[^>]*>✓<\/span><span[^>]*>⚡<\/span>/);
  const alarm = fault(1, { code: 'POE_PORT_FAULT', active: true, switch_id: 1, if_index: 7 });
  const withFault = draw([powered], [alarm]);
  assert.match(withFault, /aria-label="1\/0\/7 rj45 · Connected · [^"]*PoE fault[^"]*"/);
  assert.match(withFault, /<span[^>]*>✓<\/span><span[^>]*>⚠<\/span><span[^>]*>⛔<\/span>/);
  assert.doesNotMatch(withFault, /⚡/);

  setLang('ja');
  assert.match(draw([powered], [alarm]), /<span[^>]*>接続中<\/span><span[^>]*>⚠<\/span><span[^>]*>⛔<\/span>/);
});

test('switches without credentials show a credential status on the rack and wall', context => {
  context.after(() => setLang('ja'));
  setLang('en');
  const { RackRow } = require('../src/components/RackRow.tsx');
  const { registeredSummary } = require('../src/views/rackSummaries.ts');
  const summary = registeredSummary({ id: 1, name: 'Core', needs_credentials: true });
  const props = {
    summary, ports: [], devices: [], alerts: [], definitions: {}, wall: true,
    uplinks: new Set(), actions: {},
  };
  for (const component of [RackRow, WallSwitchTile]) {
    const html = render(component, props);
    assert.ok(html.includes('SNMP community required'));
    assert.ok(!html.includes(t('rack.offline')));
    assert.ok(render(component, { ...props, summary: { ...summary, needs_credentials: false } }).includes(t('rack.offline')));
  }
});

test('発生元のイベントコードは日英で文言を持ち、通常表示に依頼先を出さない', context => {
  context.after(() => setLang('ja'));
  const { eventJa } = require('../src/i18n/eventJa.ts');
  const { eventEn } = require('../src/i18n/eventEn.ts');
  const source = name => fs.readFileSync(path.join(__dirname, '../../internal', name), 'utf8');
  const eventSources = [
    'api/handlers.go', 'api/ws.go', 'engine/baseline_i18n.go', 'engine/operations.go', 'engine/rules.go',
    'engine/views.go', 'model/config_i18n.go', 'poller/collect.go', 'poller/poller.go', 'trap/trap.go',
  ].map(source).join('\n');
  const codes = new Set([
    ...[...eventSources.matchAll(/\bCode:\s*"([^"]+)"/g)].map(match => match[1]),
    ...[...eventSources.matchAll(/\b(?:ev|add)\(\s*(?:model\.Sev\w+|sev),\s*"([^"]+)"/g)].map(match => match[1]),
    ...[...eventSources.matchAll(/\bconst\s+Code\w+\s*=\s*"([^"]+)"/g)].map(match => match[1]),
    ...[...eventSources.matchAll(/ev\.Code,\s*ev\.Title\s*=\s*"([^"]+)"/g)].map(match => match[1]),
    ...[...source('engine/device_monitor.go').matchAll(/\.deviceEvent\("([^"]+)"/g)].map(match => match[1]),
    ...[...source('engine/recommendations.go').matchAll(/\badd\(\w+,\s*\w+,\s*"([^"]+)"/g)].map(match => match[1]),
    ...[...source('mib/traps.go').matchAll(/\{"(\w+)",\s*"(?:INFO|WARNING|CRITICAL)",/g)].map(match => match[1]),
  ]);
  assert.ok(codes.has('PHY_ERRORS') && codes.has('PORT_CONFIG_CHANGED') && codes.has('linkDown'));
  const fields = text => [...(text ?? '').matchAll(/\{(\w+)\}/g)].map(match => match[1]).sort();
  for (const code of codes) {
    assert.ok(Object.hasOwn(eventJa, code) && eventJa[code].title, `ja / ${code}`);
    assert.ok(Object.hasOwn(eventEn, code) && eventEn[code].title, `en / ${code}`);
    for (const field of ['title', 'hint', 'details']) {
      assert.deepEqual(fields(eventJa[code][field]), fields(eventEn[code][field]), `${code} / ${field}`);
    }
  }
  for (const [, code, hint] of source('model/event_i18n.go').matchAll(/^\s*"([^"]+)":\s*("(?:[^"\\]|\\.)*"),$/gm)) {
    assert.equal(JSON.parse(hint), eventJa[code]?.hint, `Go / ja / ${code}`);
  }
  for (const [lang, dictionary] of [['ja', eventJa], ['en', eventEn]]) {
    setLang(lang);
    const forbidden = /(?:担当(?:者|チーム)?[にへ]|確認を依頼|確認してもら|\b(?:ask|contact|notify|report to|escalate to|have)\s+(?:the\s+|your\s+)?(?:network|IT|someone|administrator)\b)/iu;
    for (const code of Object.keys(dictionary).filter(code => !isRecommendationEvent(code))) {
      const alert = fault(1, { code, title: 'Stored title', hint: 'Ask the network team' });
      assert.doesNotMatch(render(AlertEntry, { alert, busy: false }), forbidden, `${lang} / ${code}`);
    }
  }
});

test('日英のプレースホルダーと技術詳細は保存済みイベントも扱う', context => {
  context.after(() => setLang('ja'));
  const alert = fault(1, { code: 'DEVICE_MISSING', detail: { name: 'Mixer', mac: '00:11:22:33:44:55' } });
  const config = { code: 'PORT_CONFIG_CHANGED', detail: { port: '1/0/7', field: 'max_frame', before: '1522', after: '9216' } };
  for (const [lang, title, shortConfig, detailConfig] of [
    ['ja', '登録機器が未検出: Mixer', '1/0/7 の 最大フレーム が変化', '1/0/7 最大フレーム 1522 → 9216'],
    ['en', 'Registered device missing: Mixer', '1/0/7 Maximum frame size changed', '1/0/7 Maximum frame size 1522 → 9216'],
  ]) {
    setLang(lang);
    assert.equal(eventTitle(alert), title);
    assert.equal(eventTitle(config), shortConfig);
    assert.equal(eventDetails(config), detailConfig);
    assert.match(eventTitle({ code: 'DISCARDS', detail: { delta: 0 } }), /\+0/);
    assert.doesNotMatch(eventTitle({ code: 'DISCARDS', title: 'Stored technical title' }), /\{|Stored technical/);
    const normal = render(AlertEntry, { alert, busy: false });
    // Normal display: no cause/hint text; the expert display shows it as reference.
    assert.ok(!normal.includes(eventHint(alert)));
    assert.ok(render(AlertEntry, { alert, busy: false, expert: true }).includes(eventHint(alert)));
    assert.doesNotMatch(normal, /DEVICE_MISSING|00:11:22:33:44:55/);
    assert.match(render(AlertEntry, { alert, busy: false, expert: true }), /00:11:22:33:44:55[^]*DEVICE_MISSING/);
  }
});

test('AlertEntry uses the API detail fields in the English clock skew title', context => {
  context.after(() => setLang('ja'));
  setLang('en');
  const alert = fault(1, {
    code: 'CLOCK_SKEW',
    title: 'Stored title',
    detail: { offset_seconds: -61 },
  });
  // Values belong to the expert display; the normal display shows the headline only.
  assert.match(render(AlertEntry, { alert, busy: false, expert: true }), /Clock offset from monitoring PC: -61 s/);
  assert.doesNotMatch(render(AlertEntry, { alert, busy: false }), /-61 s/);
  assert.match(render(AlertEntry, { alert: { ...alert, detail: undefined }, busy: false }),
    /Clock offset from monitoring PC</);
});

const groupedSwitches = [
  { id: 1, name: 'West', group_tag: 'west', reachable: true },
  { id: 2, name: 'East', group_tag: 'east', reachable: true },
];
const groupView = { overview: { switches: groupedSwitches }, group: 'west', replay: null, locked: true };
const groupFaults = [
  fault(1, { switch_id: 0, title: 'Network zero' }),
  fault(2, { title: 'Network absent' }),
  fault(3, { switch_id: 1, title: 'West fault' }),
  fault(4, { switch_id: 2, title: 'East fault' }),
].map(alert => ({ ...alert, active: true, ts: '2026-01-01T00:00:00Z' }));

test('Group scope retains zero and absent switch IDs across monitoring views', () => {
  const { OverviewView } = require('../src/views/Overview.tsx');
  const findings = groupFaults;
  const check = html => {
    for (const title of ['Network zero', 'Network absent', 'West fault']) assert.ok(html.includes(title), title);
    assert.doesNotMatch(html, /East fault/);
  };
  const props = { ...groupView, operations: { ready: true, alerts: findings } };
  check(render(OverviewView, props));
  const wall = render(OverviewView, { ...props, wall: true });
  check(wall);
  const globalTiles = [...wall.matchAll(/<article class="wall-switch-tile[^]*?<\/article>/g)].filter(([tile]) => tile.includes(t('wall.global')));
  assert.equal(globalTiles.length, 1);
  assert.match(globalTiles[0][0], /Network zero/);
  assert.match(globalTiles[0][0], /Network absent/);

  const { MulticastView } = sourceWith('../src/views/MulticastView.tsx', {
    '../usePolling': { usePolling: () => ({ data: { multicast: { findings }, states: groupedSwitches } }) },
    '../components/AvStatus': { AvStatus: () => null },
  });
  check(render(MulticastView, props));
  const { DhcpView } = sourceWith('../src/views/DhcpView.tsx', {
    '../usePolling': { usePolling: () => ({ data: { findings, servers: groupedSwitches.map(sw => ({
      switch_id: sw.id, switch_name: sw.name, server_on: true,
    })) } }) },
  });
  const dhcp = render(DhcpView, props);
  check(dhcp);
  assert.ok(dhcp.includes(t('ui.m249')));
  assert.doesNotMatch(dhcp, />East<\/td>/);
  const { HealthAudit } = sourceWith('../src/views/HealthAudit.tsx', {
    '../usePolling': { usePolling: () => ({ data: { audit: { findings } } }) },
  });
  check(render(HealthAudit, { ...props, selected: 0 }));
  const events = findings.map(({ event_id, ...alert }) => ({ ...alert, id: event_id, category: 'SYSTEM' }));
  const { Logs } = sourceWith('../src/views/Logs.tsx', { '../usePolling': { usePolling: () => ({ data: events }) } });
  check(render(Logs, { ...props, events: [] }));
});

test('Filtered live and replay monitors label the overall count and report outside groups in both modes', () => {
  const { PortMonitor } = require('../src/views/PortMonitor.tsx');
  const states = groupedSwitches.map(sw => ({
    ...sw, health: {}, alert_counts: sw.id === 1 ? { WARNING: 1 } : { CRITICAL: 2 },
  }));
  const snapshot = { id: 'recording', ts: '2026-01-01T00:00:00Z', timeline: {}, states };
  for (const wall of [false, true]) {
    for (const { replay, count, outside } of [
      { replay: null, count: 4, outside: 1 },
      { replay: { ...snapshot, alerts: groupFaults.slice(1) }, count: 3, outside: 1 },
      { replay: snapshot, count: 3, outside: 2 },
    ]) {
      const document = {};
      const state = [];
      let cursor = 0;
      const App = sourceWith('../src/App.tsx', {
        react: {
          useState: initial => {
            const index = cursor++;
            if (!(index in state)) state[index] = initial === null ? replay : initial?.switches ? groupView.overview
              : typeof initial === 'function' ? initial() : initial;
            return [state[index], value => { state[index] = value; }];
          },
          useCallback: callback => callback,
          useEffect: effect => { if (effect.toString().includes('document.title')) effect(); },
          useRef: current => ({ current }),
        },
        './components/TerminalDrawer': { TerminalDrawer: () => null },
        './i18n': { t, useLang: () => ['ja'] },
        './pcTime': { usePcTime: () => 0 },
        './monitorHealth': { ...require('../src/monitorHealth.ts'), monitoringStopped: () => false },
        './useDesktopNotifications': { useDesktopNotifications: noop },
        './useLiveFeed': { useLiveFeed: () => ({}) },
        './useOperations': { useOperations: () => ({ ready: true, alerts: groupFaults, groups: [] }) },
      }, { document, URLSearchParams, location: { search: `?wall=${Number(wall)}` } }).default;
      const header = tree => elements(tree, node => node.type === AppHeader)[0].props;
      const monitor = tree => elements(tree, node => node.type === PortMonitor)[0].props;
      const { OverviewView } = require('../src/views/Overview.tsx');
      const first = App();
      assert.equal(document.title, `(${count}) AVLine-Sight`);
      assert.ok(!render(OverviewView, monitor(first)).includes(t('cleanup.outsideGroup', { count: outside })));
      monitor(first).onFilterChange({ group: 'west', part: '' });
      cursor = 0;
      const filtered = App();
      const { summary } = header(filtered);
      assert.equal(header(filtered).wall, wall);
      assert.equal(summary.props.count, count);
      assert.equal(render(summary.type, summary.props).includes(`>${t('cleanup.overall')}</span>`), !wall);
      assert.equal(document.title, `(${count}) AVLine-Sight${wall ? '' : ` · ${t('cleanup.overall')}`}`);
      const html = render(OverviewView, monitor(filtered));
      if (wall) {
        assert.ok(!html.includes(t('cleanup.outsideGroup', { count: outside })));
        continue;
      }
      assert.ok(html.includes(t('cleanup.outsideGroup', { count: outside })));
      if (replay?.alerts) {
        assert.match(html, /Network absent/);
        assert.doesNotMatch(html, /Network zero|East fault|data-action="ack/);
      }
    }
  }
});

test('Multicast flags both kinds of querier conflict, including across groups', () => {
  const vlan = (id, vlan, oper, address) => ({ switch_id: id, vlan, querier_oper: oper, last_querier: address, snooping: true });
  const rows = [
    vlan(1, 10, 'querier', '0.0.0.0'), vlan(2, 10, 'querier', '::'),
    vlan(1, 20, 'non-querier', '192.0.2.1'), vlan(2, 20, 'non-querier', '192.0.2.2'),
    vlan(1, 30, 'querier', '192.0.2.1'), vlan(2, 30, 'non-querier', '192.0.2.1'),
    vlan(1, 40, 'querier', '0.0.0.0'), vlan(2, 40, 'non-querier', '::'),
  ];
  const { MulticastView } = sourceWith('../src/views/MulticastView.tsx', {
    '../usePolling': { usePolling: () => ({ data: { multicast: { vlans: rows }, states: groupedSwitches } }) },
    '../components/AvStatus': { AvStatus: () => null },
  });
  const html = render(MulticastView, groupView);
  for (const [vlan, querierOK, querierReview] of [[10, 0, 1], [20, 0, 1], [30, 1, 0], [40, 1, 0]]) {
    assert.ok(html.includes(t('pages.vlanSummary', { vlan, querierOK, querierReview, snoopingOK: 1, snoopingReview: 0 })));
  }
});

test('Alert band bulk acknowledgement applies the same eligibility rules inside each visible scope', () => {
  const alerts = [...groupFaults, fault(5, { switch_id: 1, suppressed: true }),
    fault(6, { switch_id: 1, severity: 'INFO' }), fault(7, { switch_id: 1, acked: true, active: true }),
    fault(8, { switch_id: 1, severity: 'CRITICAL', active: false })];
  const { AlertBand: clickableBand } = sourceWith('../src/components/AlertBand.tsx', {}, { window: { confirm: () => true } });
  let bandProps;
  const { OverviewView } = sourceWith('../src/views/Overview.tsx', {
    '../components/AlertBand': { AlertBand: props => { bandProps = props; return null; } },
  });
  const calls = [];
  const eligible = [...acknowledgeableEventIds(alerts)];
  const operations = { ready: true, alerts, ack: () => calls.push(eligible), ackMany: ids => calls.push(Array.from(ids)) };
  for (const wall of [false, true]) {
    for (const group of [undefined, 'west']) {
      render(OverviewView, { ...groupView, group, locked: false, wall, operations });
      const tree = clickableBand(bandProps);
      elements(tree, node => node.props?.['data-action'] === 'ack-all')[0].props.onClick();
      assert.deepEqual(calls.pop().sort((a, b) => a - b), group ? [1, 2, 3, 8] : [1, 2, 3, 4, 8]);
    }
  }
});

test('Multi-tag device CSV contains exactly the displayed rows in live mode and replay', async () => {
  const rows = [
    { switch_id: 1, group_tag: 'west', mac: '02:00:00:00:00:01', label: '=West, "Mixer"' },
    { switch_id: 2, group_tag: 'east', mac: '02:00:00:00:00:02', label: 'East camera' },
    { switch_id: 3, group_tag: 'other', mac: '02:00:00:00:00:03', label: 'Excluded' },
  ];
  const downloads = [];
  let loading = false;
  let filter = { part: '' };
  const { Devices, DeviceInventoryTable: InventoryTable } = sourceWith('../src/views/Devices.tsx', {
    react: { ...React, useState: initial => initial?.part === '' ? [filter, value => { filter = value; }] : [initial, noop] },
    '../api': { download: (blob, name) => downloads.push({ blob, name }) },
    '../usePolling': { usePolling: (_load, _interval, [key]) => ({ data: loading ? undefined : { key, rows } }) },
  });
  const props = { settings: { views: [{ id: 'show', tags: ['west', 'east'] }] } };
  for (const replay of [null, []]) {
    const replayRef = replay && { id: 'recording', ts: '2026-01-01T00:00:00Z' };
    const draw = () => Devices({ ...props, replay, replayRef });
    const exportButton = tree => elements(tree, node => node.type === 'button' && node.props.children === t('ui.m235'))[0];
    filter = { part: '' };
    elements(draw(), node => node.type === SwitchFilters)[0].props.onChange({ part: 'show' });
    loading = true;
    assert.equal(exportButton(draw()).props.disabled, true);
    loading = false;
    const tree = draw();
    const tableRows = elements(tree, node => node.type === InventoryTable)[0].props.rows;
    assert.deepEqual(Array.from(tableRows, row => row.mac), ['02:00:00:00:00:01', '02:00:00:00:00:02']);
    assert.equal(elements(tree, node => node.type === 'a' && node.props.href?.includes('export.csv')).length, 0);
    exportButton(tree).props.onClick();
    const { blob, name } = downloads.pop();
    assert.equal(name, 'avline-sight-devices.csv');
    const csv = await blob.text();
    assert.match(csv, /02:00:00:00:00:01,"'=West, ""Mixer"""/);
    assert.match(csv, /02:00:00:00:00:02,East camera/);
    assert.doesNotMatch(csv, /02:00:00:00:00:03|Excluded/);
    assert.equal(csv.trimEnd().split('\r\n').length, 3);
    elements(tree, node => node.type === SwitchFilters)[0].props.onChange({ part: 'show', group: 'west', switchId: 1 });
    assert.deepEqual(Array.from(elements(draw(), node => node.type === InventoryTable)[0].props.rows, row => row.mac), ['02:00:00:00:00:01']);
    elements(tree, node => node.type === SwitchFilters)[0].props.onChange({ part: 'show', group: 'west', switchId: 2 });
    assert.deepEqual(Array.from(elements(draw(), node => node.type === InventoryTable)[0].props.rows), []);
  }
});

test('Details shows the latest baseline diff and recent configuration events in both languages', async () => {
  const captured = '2026-01-01T00:00:00Z';
  const baseline = { id: 9, name: 'Show', captured_ts: captured };
  const diff = { switch_id: 1, switch_name: 'West', item: '1/0/7', config_field: 'max_frame', before: '1522', after: '9216' };
  const event = {
    id: 12, ts: captured, switch_id: 1, switch_name: 'West', category: 'CONFIG', severity: 'INFO',
    code: 'PORT_CONFIG_CHANGED', detail: { port: '1/0/7', field: 'max_frame', before: '1522', after: '9216' },
  };
  const baselines = '/api/baselines';
  const latestDiff = '/api/baselines/9/diff?read_only=1';
  const history = `/api/events?category=CONFIG&severity=INFO&active_profile=1&limit=${SEARCH_EVENT_LIMIT}`;
  const eastEvents = Array.from({ length: 31 }, (_, i) => ({ ...event, id: 100 + i, switch_id: 2, switch_name: 'East' }));
  const westEvents = Array.from({ length: 29 }, (_, i) => ({
    ...event, id: 200 + i, switch_name: `West history ${i + 1}`,
    detail: { ...event.detail, field: 'pvid', before: '1', after: '2' },
  }));
  const systemEvent = { ...westEvents[0], id: 300, switch_id: 0, switch_name: 'System-wide' };
  const responses = {
    [baselines]: [baseline, { id: 3, name: 'Old' }],
    [latestDiff]: { diffs: [diff, { ...diff, switch_id: 2, after: '8000' }] },
    [history]: [...eastEvents, event, systemEvent, ...westEvents],
  };
  const calls = [];
  const results = [];
  const loaders = [];
  const { ConfigChanges } = sourceWith('../src/views/ConfigChanges.tsx', {
    '../api': { api: async (url, { signal }) => {
      assert.ok(signal instanceof AbortSignal);
      calls.push(url);
      assert.ok(Object.hasOwn(responses, url));
      return responses[url];
    } },
    '../usePolling': { usePolling: load => {
      const index = loaders.length;
      loaders.push(load);
      return { data: results[index] };
    } },
  });
  const props = { group: 'west', overview: { switches: [{ id: 1, name: 'West', group_tag: 'west' }] } };
  render(ConfigChanges, props);
  for (const load of loaders) results.push(await load(new AbortController().signal));
  assert.deepEqual(calls, [baselines, latestDiff, history]);
  try {
    for (const [language, heading, field] of [
      ['ja', '最新ベースラインとの差分', '最大フレーム'],
      ['en', 'Changes from the latest baseline', 'Maximum frame size'],
    ]) {
      setLang(language);
      loaders.length = 0;
      const html = render(ConfigChanges, { ...props, expert: true });
      assert.ok(html.includes(heading));
      assert.ok(html.includes('Show'));
      assert.ok(html.includes(`1/0/7 ${field} 1522 → 9216`));
      assert.equal(html.split(field).length - 1, 3);
      assert.match(html, /System-wide/);
      assert.match(html, /West history 28 ·/);
      assert.equal((html.match(/<time\b/g) ?? []).length, 30);
      assert.doesNotMatch(html, /8000|East|West history 29|data-action="ack"/);
      const log = render(LogEventRow, { event, ackable: new Set([event.id]), onAcknowledge: noop });
      assert.ok(log.includes(field));
      assert.doesNotMatch(log, /<button/);
    }
  } finally {
    setLang('ja');
  }
});

test('ラックの手前に5状態の凡例を置き、2桁ポートと状態記号を別にする', () => {
  const { OverviewView } = sourceWith('../src/views/Overview.tsx', {
    '../useRackData': { useRackData: () => ({ data: { ports: [
      { switch_id: 1, port: { name: '1/0/11', if_index: 11, oper: 'up', type: 'rj45' } },
    ] }, definitions: {}, refresh: noop }) },
  });
  const html = render(OverviewView, {
    overview: { switches: [{ id: 1, name: 'West', group_tag: '', reachable: true, psu_ok: false, fans_ok: true }] }, replay: null, locked: true,
    operations: { ready: true, alerts: [fault(1, { switch_id: 1, if_index: 11, active: true, code: 'PHY_ERRORS', severity: 'CRITICAL',
      device_name: 'Mixer', port: '1/0/11' })] },
  });
  const legend = html.match(/<ul class="rack-legend"[^]*?<\/ul>/)[0];
  for (const [tone, symbol, label] of [
    ['critical', '!', '異常'], ['warning', '▲', '注意・未確認'], ['healthy', '✓', '正常'],
    ['unused', '·', '未使用'], ['unknown', '?', '不明'],
  ]) {
    assert.ok(legend.includes(`class="lamp-${tone}" aria-hidden="true">${symbol}</span>${label}`));
  }
  assert.ok(html.indexOf(legend) < html.indexOf('class="rack-groups"'));
  assert.match(html, /class="lamp-number">11<\/span><span class="lamp-symbol" aria-hidden="true">!<\/span>/);
  assert.match(html, /Mixer · 1\/0\/11 物理エラー/);
  // Health: only the abnormal item is named in words; the full list stays in the tooltip.
  assert.match(html, /<span class="health-abnormal">⚠ 電源 異常<\/span>/);
  assert.match(html, /class="rack-health" title="[^"]*電源 異常[^"]*"/);
  assert.doesNotMatch(html, /本体 ✓/);
  // Group header counts are words, and the legend explains the dot before a switch name.
  assert.match(html, /異常 1 · 未確認 1/);
  assert.match(html, /●<\/span>応答あり/);
});

test('接続機器の列順と登録名・LLDP名の表示', () => {
  const html = render(DeviceInventoryTable, { rows: [
    { mac: 'AA', label: 'Mixer', lldp_name: 'Registered LLDP', seen: true },
    { mac: 'BB', lldp_name: 'Console', seen: true },
    { mac: 'CC', seen: true },
  ] });
  assert.deepEqual(headers(html), ['状態', '機器名', 'スイッチ/ポート', 'IP/予定IP', 'Ping', 'メーカー', 'VLAN/PoE', 'MAC']);
  assert.match(html, /<strong>Mixer<\/strong>/);
  assert.match(html, /class="device-secondary">Console<\/span>/);
  assert.doesNotMatch(html, /名前なし/);
});

test('VLAN表の列順とクエリア状態の翻訳、VLANごとの正常・要確認を検証する', () => {
  const rows = [
    { querier_oper: 'non-querier', switch_id: 2, switch_name: 'East', vlan: 10, name: 'Audio', snooping: false, last_querier: '0.0.0.0' },
    { querier_oper: 'querier', switch_id: 1, switch_name: 'West', vlan: 10, name: 'Audio', snooping: true, querier_addr: '192.0.2.4' },
    { querier_oper: 'non-querier', switch_id: 2, switch_name: 'East', vlan: 20, name: 'Video', snooping: true, last_querier: '192.0.2.6' },
  ];
  const html = render(VlanQuerierTable, { rows });
  assert.deepEqual(headers(html).slice(0, 6), ['スイッチ', 'VLAN', '名前', 'Snooping', 'クエリア状態', 'クエリアIP']);
  assert.doesNotMatch(html, /スイッチID|>non-querier<|>querier</);
  assert.match(html, /<td>非クエリア<\/td>/);
  assert.match(html, /VLAN 10 クエリア 正常 1 \/ 要確認 1、Snooping 正常 1 \/ 要確認 1[^]*VLAN 20 クエリア 正常 1 \/ 要確認 0/);
  assert.doesNotMatch(render(VlanQuerierTable, { rows: [] }), /multicast-summary/);
  setLang('en');
  try {
    assert.match(render(VlanQuerierTable, { rows }), /<td>Non-querier<\/td>/);
  } finally {
    setLang('ja');
  }
});

test('本番の一括Pingとカウンタリセットは負荷を説明し、キャンセルで送信しない', () => {
  const messages = [];
  let started = 0;
  let approved = false;
  const { GroupActions, confirmBulkPing } = sourceWith('../src/components/GroupActions.tsx', {},
    { window: { confirm: message => { messages.push(message); return approved; } } });
  const props = { group: 'Audio', phase: 'show', devices: [{ group_tag: 'Audio', ip: '192.0.2.1' }],
    run: () => { started++; }, notice: noop };
  const click = (name, extra = {}) =>
    elements(GroupActions({ ...props, ...extra }), node => node.props?.['data-action'] === name)[0].props.onClick();
  click('ping');
  click('reset');
  assert.equal(started, 0);
  assert.match(messages[0], /本番中に1件.*負荷/);
  assert.match(messages[1], /カウンタをリセット[^]*本番中.*負荷/);
  click('ping', { phase: 'rehearsal' });
  assert.equal(messages.length, 2);
  assert.equal(started, 1);
  approved = true;
  click('ping');
  click('reset');
  assert.equal(started, 3);

  const { Devices } = sourceWith('../src/views/Devices.tsx', {
    '../components/GroupActions': { confirmBulkPing },
    '../usePolling': { usePolling: (_load, _interval, [key]) => ({ data: { key, rows: [{ ip: '192.0.2.1' }] } }) },
    react: { useState: value => [value, noop] },
  });
  approved = false;
  // During the show the Ping target starts on manual entry, so nothing is sent until an address is typed.
  const showTree = Devices({ ...props, replay: null, status: { phase: 'show', profile: { id: 1 } } });
  assert.equal(elements(showTree, node => node.props?.['data-action'] === 'ping-target')[0].props.value, 'manual');
  assert.equal(elements(showTree, node => node.props?.['data-action'] === 'ping-run')[0].props.disabled, true);
  // Choosing the shown devices during the show asks once and sends nothing on cancel.
  const { Devices: VisibleDevices } = sourceWith('../src/views/Devices.tsx', {
    '../components/GroupActions': { confirmBulkPing },
    '../usePolling': { usePolling: (_load, _interval, [key]) => ({ data: { key, rows: [{ ip: '192.0.2.1' }] } }) },
    react: { useState: value => [value?.target === 'manual' ? { target: 'visible', phase: 'show' } : value, noop] },
  });
  const tree = VisibleDevices({ ...props, replay: null, status: { phase: 'show', profile: { id: 1 } } });
  elements(tree, node => node.props?.['data-action'] === 'ping-run')[0].props.onClick();
  assert.equal(started, 3);
  assert.match(messages.at(-1), /本番中に1件.*負荷/);
});

test('Header and sidebar place project, monitoring and edit controls in the approved order', () => {
  setLang('ja');
  const status = { profile: { id: 1, name: 'Studio' }, trap: { ok: true } };
  const header = render(AppHeader, { status, online: true, phase: 'show', locked: false });
  assert.match(header, /AVLine-Sight[^]*● LIVE[^]*<small class="header-project">プロジェクト: Studio<\/small>[^]*data-action="phase"[^]*trap-ready[^]*header-marker[^]*>記録<[^]*>壁表示<[^]*header-time/);
  assert.doesNotMatch(header, /スイッチグループ|編集可|>設定<|<option value="1"/);
  const sidebar = render(AppSidebar, { status, profiles: [status.profile], locked: false, expert: false,
    children: React.createElement('button', null, 'ポートモニター') });
  assert.match(sidebar, /<nav[^]*ポートモニター[^]*<\/nav>[^]*プロジェクト<select[^]*Studio[^]*編集可（ロックする）[^]*>設定<[^]*class="app-nav-expert">エキスパート表示<input[^]*role="switch"/);
  assert.match(render(AppSidebar, { status, profiles: [], locked: true, expert: true }), /🔒 ロック中[^]*role="switch"[^]*checked=""/);
  const css = fs.readFileSync(path.join(__dirname, '../src/style.css'), 'utf8');
  assert.match(css, /\.app-nav-tools \{[^}]*margin-block-start: auto/);
  assert.match(css, /\.app-nav-expert \{[^}]*border: 1px dashed/);
});

test('Project and project-file text is consistent in both languages', () => {
  try {
    const expected = {
      ja: ['プロジェクト', 'プロジェクトファイルを読み込む', 'プロジェクトファイルを書き出す'],
      en: ['Project', 'Import project file', 'Export project file'],
    };
    for (const [language, labels] of Object.entries(expected)) {
      setLang(language);
      assert.deepEqual(['rack.profile', 'configFile.import', 'configFile.export'].map(key => t(key)), labels);
      assert.match(t('configFile.applyNote'), language === 'ja' ? /新しいプロジェクト/ : /new project/);
      const tools = render(DataTransfers, { profileId: 1, groups: [], replay: false });
      assert.ok(tools.includes(labels[1]));
      assert.ok(tools.includes(labels[2]));
      assert.match(tools, /href="\/api\/profiles\/1\/export" download="avline-sight.config.json"/);
    }
  } finally {
    setLang('ja');
  }
});

test('Diagram and list share the group and part filters, leave switch choice to the index, and preserve system-wide alerts', () => {
  const switches = [
    { id: 1, name: 'West-A', group_tag: 'west' }, { id: 2, name: 'East-A', group_tag: 'east' },
    { id: 3, name: 'West-B', group_tag: 'west' }, { id: 4, name: 'No group', group_tag: '' },
  ];
  const alerts = [
    fault(1, { switch_id: 1, title: 'Visible fault' }), fault(2, { switch_id: 2, title: 'East fault' }),
    fault(3, { switch_id: 3, title: 'West-B fault' }), fault(4, { switch_id: 0, title: 'Network fault' }),
  ];
  let layout = 'diagram';
  let filter = { part: '' };
  const { PortMatrix } = require('../src/views/PortMatrix.tsx');
  const { OverviewView } = sourceWith('../src/views/Overview.tsx', {
    react: { ...React, useState: initial => initial === 'diagram' ? [layout, value => { layout = value; }] : [typeof initial === 'function' ? initial() : initial, noop],
      useEffect: noop, useMemo: make => make(), useRef: current => ({ current }) },
    '../useRackData': { useRackData: () => ({ data: {}, definitions: {}, refresh: noop }) },
  });
  const draw = () => OverviewView({ ...filter, onFilterChange: value => { filter = value; }, overview: { switches },
    settings: { views: [{ id: 'stage', name: 'Stage', tags: ['west', 'east'] }] }, replay: null, locked: true,
    operations: { alerts, ready: true } });
  const filterRow = tree => elements(tree, node => node.type === SwitchFilters)[0];
  const selects = () => elements(SwitchFilters(filterRow(draw()).props), node => node.type === 'select');
  assert.deepEqual(Array.from(selects(), input => input.props.value), ['', '']);
  const initialRow = render(SwitchFilters, filterRow(draw()).props);
  assert.match(initialRow, /スイッチグループ[^]*担当パート/);
  // 1台の切替は左の索引にあるので、ツールバーにスイッチ選択を重ねない
  assert.doesNotMatch(initialRow, /<label>スイッチ<select/);
  assert.equal((initialRow.match(/>すべて</g) || []).length, 2);
  assert.match(initialRow, /value="group:">未所属/);
  selects()[0].props.onChange({ target: { value: 'group:west' } });
  selects()[1].props.onChange({ target: { value: 'stage' } });
  assert.equal(filter.switchId, undefined);
  const diagram = renderToStaticMarkup(draw());
  assert.match(diagram, /Visible fault/);
  assert.match(diagram, /West-B fault/);
  assert.match(diagram, /Network fault/);
  assert.doesNotMatch(diagram, /East fault/);
  assert.match(diagram, /id="rack-switch-1"[^]*id="rack-switch-3"|id="rack-switch-3"[^]*id="rack-switch-1"/);
  assert.doesNotMatch(diagram, /id="rack-switch-[24]"/);
  const switchLayout = label => elements(draw(), node => node.type === 'button' && node.props.children === t(label))[0].props.onClick();
  switchLayout('cleanup.list');
  assert.deepEqual(Array.from(selects(), input => input.props.value), ['group:west', 'stage']);
  assert.deepEqual([...elements(draw(), node => node.type === PortMatrix)[0].props.visibleSwitchIds].sort(), [1, 3]);
  switchLayout('cleanup.diagram');
  assert.deepEqual(Array.from(selects(), input => input.props.value), ['group:west', 'stage']);
});

test('Port list localizes link state and lays out search, sort, refresh and device metadata', () => {
  const ports = [
    { switch_id: 1, switch_name: 'Visible switch', port: { name: '1/0/1', oper: 'up', pvid: 10,
      devices: [{ label: 'Mixer', mac: '02:00:00:00:00:01', ip: '192.0.2.1', vendor: 'Maker' }] } },
    { switch_id: 1, switch_name: 'Visible switch', port: { name: '1/0/2', oper: 'down', pvid: 10 } },
    { switch_id: 2, switch_name: 'Excluded switch', port: { name: '1/0/3', oper: 'up', pvid: 10 } },
  ];
  const { PortMatrix } = sourceWith('../src/views/PortMatrix.tsx', {
    '../usePolling': { usePolling: () => ({ updatedAt: new Date('2026-01-01T00:00:00Z'), data: ports }) },
  });
  const html = render(PortMatrix, { part: '', switchId: 1, visibleSwitchIds: new Set([1]), replay: null });
  assert.match(html, /class="port-list-controls"[^]*更新:[^]*placeholder="機器名 \/ MAC \/ IP \/ VLAN"[^]*aria-label="並び順"[^]*type="checkbox"[^]*異常のみ<\/label><\/div>/);
  assert.match(html, /<td>接続中<\/td>[^]*<strong>Mixer<\/strong><small class="port-list-device-meta">02:00:00:00:00:01 · 192.0.2.1 · Maker<\/small>/);
  assert.match(html, /<td>未接続<\/td>/);
  assert.doesNotMatch(html, /Excluded switch|deviceDetails/);
  const css = fs.readFileSync(path.join(__dirname, '../src/style.css'), 'utf8');
  assert.match(css, /\.port-list-controls input\[type="search"\] \{ width: 17rem; max-width: 100%;/);
  assert.match(css, /\.port-list-device-meta \{ display: block; font-size: .8em;/);
});

test('壁の上部・未確認キュー・全タイルは発生中と未確認の同じ集合を集計する', () => {
  const alerts = [
    fault(1, { switch_id: 1, active: true }), fault(2, { switch_id: 1, active: true, acked: true }),
    fault(3, { switch_id: 2, active: false }), fault(4, { switch_id: 2, active: false, acked: true }),
    fault(5, { switch_id: 2, active: true, suppressed: true }), fault(6, { switch_id: 1, active: true, severity: 'INFO' }),
  ];
  const counts = summarizeAlerts(alerts);
  assert.deepEqual([counts.active, counts.unacked, counts.count], [2, 2, 3]);
  const tiles = [1, 2].map(id => summarizeAlerts(alerts.filter(alert => alert.switch_id === id)));
  assert.equal(tiles.reduce((n, tile) => n + tile.active, 0), counts.active);
  assert.equal(tiles.reduce((n, tile) => n + tile.unacked, 0), counts.unacked);
  assert.deepEqual(summarizeAlerts([]).items, []);
  const top = render(OverallStatus, { count: counts.count, active: counts.active, unacked: counts.unacked, ready: true, stopped: false });
  assert.match(top, /発生中 2件 \/ 未確認 2件/);
  assert.match(render(AlertBand, { alerts, wall: true }), /未確認 2件/);
});

test('グループ、総合帯、壁、ランプは同じ発生中・未確認を表示する', () => {
  const summary = { id: 1, name: 'Switch West', group_tag: 'West', reachable: true };
  const ports = [1, 2, 3].map(if_index => ({
    switch_id: 1, port: { if_index, name: `1/0/${if_index}`, oper: if_index === 3 ? 'down' : 'up', type: 'sfp', sfp: { los: if_index === 3 } },
  }));
  const alerts = [
    fault(1, { switch_id: 1, if_index: 1, active: true, acked: true, severity: 'CRITICAL' }),
    fault(2, { switch_id: 1, if_index: 2, active: false }),
    fault(3, { switch_id: 1, if_index: 3, active: true, suppressed: true }),
    fault(4, { switch_id: 1, if_index: 3, active: true, severity: 'INFO' }),
  ];
  const { OverviewView } = sourceWith('../src/views/Overview.tsx', {
    '../useRackData': { useRackData: () => ({ data: { ports }, definitions: {}, refresh: noop }) },
  });
  const props = { overview: { switches: [summary] }, operations: { alerts, ready: true }, replay: null, locked: true };
  const monitor = render(OverviewView, props);
  assert.match(monitor, /rack-group-counts" aria-label="発生中 1件 \/ 未確認 1件"/);
  assert.match(monitor, /lamp-critical" title="1\/0\/1/);
  assert.match(monitor, /lamp-warning" title="1\/0\/2/);
  assert.match(monitor, /lamp-unknown" title="1\/0\/3/);
  assert.match(render(OverviewView, { ...props, wall: true }), /status-critical[^]*発生中 1件 \/ 未確認 1件/);
  assert.match(render(OverallStatus, { ...summarizeAlerts(alerts), critical: true, ready: true }), /発生中 1件 \/ 未確認 1件/);
  assert.match(render(WallSwitchTile, { summary, alerts: [alerts[1]] }), /status-warning[^]*復旧済み・未確認/);
  assert.match(render(OverviewView, { ...props, target: { switchId: 1, ifIndex: 2 } }), /ポート詳細 · Switch West \/ 1\/0\/2/);
});

test('トポロジーと接続機器は同じ色・記号を使い、監視対象を特定する', () => {
  const alerts = [fault(1, { switch_id: 1, if_index: 2, port: '1/0/2', active: false })];
  const cy = require('cytoscape')({ headless: true, styleEnabled: true });
  try {
    const { updateTopology } = require('../src/topologyGraph.ts');
    const { buildTopology, topologyNodeDetails } = require('../src/topologyModel.ts');
    const model = buildTopology({
      nodes: [{ id: 'sw', switch_id: 1, label: 'West', kind: 'switch', reachable: true },
        { id: 'ext', label: 'Other', kind: 'external', reachable: true }],
      edges: [{ id: 'link', source: 'sw', target: 'ext', local_port: '1/0/2' }],
    }, [], []);
    updateTopology(cy, model, undefined, { nodeDetails: topologyNodeDetails(model, alerts) });
    assert.equal(cy.getElementById('sw').data('statusLabel'), '▲ West');
    assert.equal(cy.getElementById('ext').data('statusTone'), 'warning');
    assert.equal(cy.getElementById('ext').data('monitorSwitchId'), 1);
    assert.equal(cy.getElementById('ext').data('monitorPortName'), '1/0/2');
  } finally {
    cy.destroy();
  }
  const clicked = [];
  const props = { alerts, onNavigate: target => clicked.push(target), rows: [
    { mac: 'AA', switch_id: 1, if_index: 2, switch_name: 'West', port: '1/0/2', seen: true },
    { mac: 'BB', switch_id: 2, if_index: 2, seen: true },
    { mac: 'CC', seen: false },
  ] };
  const html = render(DeviceInventoryTable, props);
  assert.match(html, /status-warning[^>]*>▲</);
  assert.match(html, /status-healthy[^>]*>✓</);
  assert.match(html, /status-unknown[^>]*>—</);
  const clickableRows = elements(DeviceInventoryTable(props), node => node.type === 'tr' && node.props.onClick);
  assert.equal(clickableRows.length, 2);
  clickableRows[0].props.onClick();
  assert.deepEqual(clicked, [{ switchId: 1, ifIndex: 2, portName: '1/0/2' }]);
});

test('通常・壁の更新時刻とカードの発生・復旧をJSTでそろえる', () => {
  const updatedAt = Date.parse('2026-11-22T15:01:02Z');
  const props = { updatedAt, online: true, profiles: [], part: '', busy: false, locked: true };
  for (const wall of [false, true]) {
    assert.match(render(AppHeader, { ...props, wall }), /header-time[^]*更新: 00:01:02 JST/);
  }
  assert.match(render(PollingStatus, { updatedAt: new Date(updatedAt) }), /更新: 00:01:02 JST/);
  const recovered = fault(1, { active: false, ts: '2026-11-22T09:29:32Z', cleared_at: '2026-11-22T09:30:10Z' });
  const html = render(AlertEntry, { alert: recovered });
  assert.match(html, /alert-warning alert-recovered/);
  // Short times without the zone (the header clock shows JST), time first, state as a badge.
  assert.match(html, /<time[^>]*>18:29:32<\/time> 発生 → <time[^>]*>18:30:10<\/time> 復旧/);
  assert.match(html, /alert-state-badge state-recovered">復旧・未確認/);
});

test('External topology nodes use all linked switch ports and select the highest-priority edge', () => {
  const cy = require('cytoscape')({ headless: true, styleEnabled: true });
  const { updateTopology } = require('../src/topologyGraph.ts');
  const { buildTopology, topologyNodeDetails } = require('../src/topologyModel.ts');
  const topology = {
    nodes: [
      ...[1, 2, 3].map(id => ({ id: `sw${id}`, switch_id: id, kind: 'switch', label: `Switch ${id}`, reachable: true })),
      { id: 'ext', label: 'External', kind: 'external', reachable: true },
    ],
    edges: [
      { id: 'first', source: 'sw1', target: 'ext', local_port: '1/0/1' },
      { id: 'second', source: 'sw2', target: 'ext', local_port: '1/0/2' },
      { id: 'third', source: 'ext', target: 'sw3', remote_port: '1/0/3' },
    ],
  };
  const recovered = fault(2, { switch_id: 2, if_index: 2, port: '1/0/2', active: false });
  const active = fault(3, { switch_id: 3, if_index: 3, port: '1/0/3', active: true, acked: true, severity: 'CRITICAL' });
  const check = (alerts, tone, id) => {
    const model = buildTopology(topology, [], []);
    updateTopology(cy, model, undefined, { nodeDetails: topologyNodeDetails(model, alerts) });
    const node = cy.getElementById('ext');
    assert.equal(node.data('statusTone'), tone);
    assert.equal(node.data('monitorSwitchId'), id);
    assert.equal(node.data('monitorPortName'), `1/0/${id}`);
  };
  try {
    check([recovered, active], 'critical', 3);
    check([recovered, { ...active, suppressed: true }], 'warning', 2);
    check([{ ...recovered, acked: true }, { ...active, port: '1/0/20' }], 'healthy', 1);
  } finally {
    cy.destroy();
  }
});

test('Topology opens the current external port before graph effects run and fetches plain inventory', async () => {
  const data = {
    topology: {
      nodes: [
        { id: 'a', switch_id: 1, kind: 'switch' },
        { id: 'b', switch_id: 2, kind: 'switch' },
        { id: 'ext', label: 'External', kind: 'external' },
      ],
      edges: [
        { id: 'first', source: 'a', target: 'ext', local_port: '11' },
        { id: 'second', source: 'b', target: 'ext', local_port: '12' },
      ],
    },
    multicast: {}, ports: [], inventory: [],
  };
  const urls = [];
  let load;
  const { Topology } = sourceWith('../src/views/Topology.tsx', {
    react: {
      useState: initial => [initial === undefined ? { kind: 'node', id: 'ext' } : initial, noop],
      useRef: initial => ({ current: initial }),
      useEffect: noop,
      useMemo: compute => compute(),
    },
    '../i18n': { t, useLang: () => ['en'] },
    '../usePolling': { usePolling: callback => { load = callback; return { data }; } },
    '../api': { api: async url => { urls.push(url); } },
  });
  const targets = [];
  for (const [alerts, expected] of [
    [[], { switchId: 1, portName: '11' }],
    [[fault(2, { switch_id: 2, if_index: 12, port: '12', active: true })], { switchId: 2, portName: '12' }],
  ]) {
    const tree = Topology({ operations: { alerts }, openPortMonitor: target => targets.push(target) });
    const [button] = elements(tree, node => node.type === 'button' && node.props.children === t('rack.nav.monitor'));
    button.props.onClick();
    assert.deepEqual(targets.at(-1), expected);
  }
  await load(new AbortController().signal);
  assert.deepEqual(urls.filter(url => url.startsWith('/api/devices')), ['/api/devices?scope=all']);
});

test('Tapping a switch count opens its connected devices and links to the Devices view', context => {
  const graph = require('cytoscape')({ headless: true });
  const state = [];
  const refs = [];
  const effects = [];
  let stateIndex = 0;
  let refIndex = 0;
  const topology = {
    nodes: [
      { id: 'sw1', switch_id: 1, kind: 'switch', label: 'West', reachable: true },
      { id: 'sw2', switch_id: 2, kind: 'switch', label: 'East', reachable: true },
    ],
    edges: [],
  };
  const inventory = topology.nodes.map(node => ({
    switch_id: node.switch_id, label: node.label, mac: `MAC-${node.id}`, port: '1', seen: true,
  }));
  const { Topology } = sourceWith('../src/views/Topology.tsx', {
    cytoscape: { default: () => graph },
    react: {
      useState(initial) {
        const index = stateIndex++;
        if (!(index in state)) state[index] = initial;
        return [state[index], value => { state[index] = value; }];
      },
      useRef(initial) {
        const index = refIndex++;
        return refs[index] ?? (refs[index] = { current: initial === null ? {} : initial });
      },
      useEffect: effect => effects.push(effect),
      useMemo: compute => compute(),
    },
    '../i18n': { t, useLang: () => ['en'] },
    '../usePolling': { usePolling: () => ({ data: { topology, inventory, ports: [], multicast: {} } }) },
  });
  let openedDevices = 0;
  const props = { openDevices: () => openedDevices++ };
  Topology(props);
  const cleanups = effects.map(effect => effect());
  context.after(() => cleanups.forEach(cleanup => cleanup?.()));
  graph.getElementById('count:sw2').emit('tap');
  stateIndex = refIndex = 0;
  const tree = Topology(props);
  const tables = elements(tree, node => node.type === DeviceInventoryTable);
  assert.equal(tables.length, 1);
  assert.deepEqual([...tables[0].props.rows], [inventory[1]]);
  const [button] = elements(tree, node => node.type === 'button' && node.props.children === t('rack.nav.devices'));
  button.props.onClick();
  assert.equal(openedDevices, 1);
});

test('Syslog retains the received time and shows the raw line only in expert display', () => {
  const raw = '<13> Nov  7 15:16:44 SwitchWest LLDP: neighbor keepalive';
  assert.equal(syslogMessage(raw), 'LLDP: neighbor keepalive');
  assert.equal(syslogMessage('<34>1 2026-11-07T15:16:44Z SwitchWest service 123 ID - data'), 'service 123 ID - data');
  assert.equal(syslogMessage('LLDP: neighbor keepalive'), 'LLDP: neighbor keepalive');
  const event = { id: 9, ts: '2026-11-07T15:16:45Z', title: raw, code: 'SWITCH_LOG', category: 'SYSLOG', severity: 'INFO' };
  const normal = render(LogEventRow, { event, ackable: new Set([9]) });
  assert.match(normal, /<td class="log-content">LLDP: neighbor keepalive<\/td>/);
  assert.doesNotMatch(normal, /<details|<pre|SwitchWest/);
  const html = render(LogEventRow, { event, ackable: new Set([9]), expert: true });
  assert.equal((html.match(/<time /g) || []).length, 1);
  assert.match(html, /2026-11-08 00:16:45 JST/);
  assert.match(html, /<summary>LLDP: neighbor keepalive<\/summary>/);
  assert.match(html, /<pre>&lt;13&gt; Nov  7 15:16:44 SwitchWest LLDP: neighbor keepalive<\/pre>/);
  assert.doesNotMatch(html, /data-action="ack"/);
});

test('Expert display defaults off, persists locally, and leaves expert pages when turned off', async () => {
  setLang('ja');
  const storage = new Map();
  const states = [];
  let cursor;
  let updateStatus;
  let loadSettings;
  const Drawer = () => null;
  const App = sourceWith('../src/App.tsx', {
    react: {
      useState: initial => {
        const index = cursor++;
        if (!(index in states)) states[index] = typeof initial === 'function' ? initial() : initial;
        return [states[index], next => { states[index] = typeof next === 'function' ? next(states[index]) : next; }];
      },
      useCallback: fn => fn,
      useRef: current => ({ current }),
      useEffect: effect => { if (effect.toString().includes('/api/settings')) loadSettings = effect; },
    },
    './api': { api: async url => url === '/api/settings' ? { views: [] } : [] },
    './components/TerminalDrawer': { TerminalDrawer: Drawer },
    './useLiveFeed': { useLiveFeed: () => ({}) },
    './useOperations': { useOperations: props => { updateStatus = props.setStatus; return { alerts: [], groups: [] }; } },
    './useDesktopNotifications': { useDesktopNotifications: noop },
    './pcTime': { usePcTime: () => 0 },
    './i18n': { t, useLang: () => ['ja'] },
  }, { AbortController, localStorage: { getItem: key => storage.get(key), setItem: (key, value) => storage.set(key, value) } }).default;
  const draw = () => { cursor = 0; return App(); };
  const header = tree => elements(tree, node => node.type === AppHeader)[0].props;
  const sidebar = tree => elements(tree, node => node.type === AppSidebar)[0].props;
  const menu = tree => sidebar(tree).children;
  const openSettings = async locked => {
    draw();
    updateStatus({ profile: { id: 1 }, locked });
    sidebar(draw()).openSettings();
    const cancel = loadSettings();
    await new Promise(setImmediate);
    cancel();
    // Settings is a page now: it marks itself current in the sidebar, then go back to the port monitor for the checks below.
    assert.equal(sidebar(draw()).settingsOpen, true);
    menu(draw())[0].props.onClick();
    return draw();
  };
  const normal = [t('rack.nav.monitor'), t('rack.nav.devices'), t('rack.nav.logs')];
  let tree = await openSettings(false);
  assert.equal(sidebar(tree).expert, false);
  assert.deepEqual(Array.from(menu(tree), item => item.props.children), normal);
  assert.ok(!render(AppHeader, header(tree)).includes('>エキスパート<'));
  const { PortMonitor } = require('../src/views/PortMonitor.tsx');
  const monitor = tree => elements(tree, node => node.type === PortMonitor)[0].props;
  assert.equal(monitor(tree).openTerminal, undefined);

  sidebar(tree).setExpert(true);
  tree = draw();
  assert.equal(storage.get('expert-display'), 'true');
  assert.deepEqual(Array.from(menu(tree), item => item.props.children), [...normal, t('cleanup.details'), t('rack.nav.topology')]);
  assert.ok(render(AppHeader, header(tree)).includes('>エキスパート<'));
  monitor(tree).openTerminal(1);
  assert.equal(elements(draw(), node => node.type === Drawer).length, 1);
  updateStatus({ profile: { id: 1 }, locked: true });
  assert.equal(elements(draw(), node => node.type === Drawer).length, 0);
  assert.equal(monitor(draw()).openTerminal, undefined);
  updateStatus({ profile: { id: 1 }, locked: false });

  for (const page of [t('cleanup.details'), t('rack.nav.topology')]) {
    tree = draw();
    menu(tree).find(item => item.props.children === page).props.onClick();
    assert.equal(menu(draw()).find(item => item.props.children === page).props['aria-current'], 'page');
    sidebar(draw()).setExpert(false);
    tree = draw();
    assert.equal(menu(tree)[0].props['aria-current'], 'page');
    assert.equal(elements(tree, node => node.type === Drawer).length, 0);
    assert.deepEqual(Array.from(menu(tree), item => item.props.children), normal);
    sidebar(tree).setExpert(true);
  }
  states.length = 0;
  assert.equal(sidebar(await openSettings(true)).expert, true);
  assert.equal(monitor(draw()).openTerminal, undefined);
});

test('Expert settings, terminal entries and alert details respect the display preference', () => {
  setLang('ja');
  const changes = [];
  const input = elements(AppSidebar({ profiles: [], expert: false, locked: true, setExpert: value => changes.push(value) }),
    node => node.type === 'input' && node.props.role === 'switch')[0];
  assert.equal(input.props.checked, false);
  input.props.onChange({ target: { checked: true } });
  assert.deepEqual(changes, [true]);

  const { ActionPopup } = sourceWith('../src/components/ActionPopup.tsx', {
    react: { ...React, useState: () => [true, noop] },
  });
  const { RackRow } = sourceWith('../src/components/RackRow.tsx', { './ActionPopup': { ActionPopup } });
  const { SwitchPorts } = require('../src/views/SwitchPorts.tsx');
  const props = { overview: { switches: groupedSwitches }, selected: 1, openTerminal: noop, replay: null };
  const rack = { summary: groupedSwitches[0], ports: [], devices: [], alerts: [], definitions: {}, uplinks: new Set(), samples: {} };
  for (const expert of [undefined, false, true]) {
    for (const locked of [false, true]) {
      const actions = { ...props, expert, locked };
      assert.equal(render(RackRow, { ...rack, actions }).includes(`>${t('rack.terminal')}</button>`), !!expert && !locked);
      const ports = render(SwitchPorts, actions);
      const terminal = [...ports.matchAll(/<button[^>]*>[^<]*<\/button>/g)].find(([button]) => button.endsWith(`>${t('ui.m279')}</button>`));
      assert.equal(!!terminal, !!expert);
      if (terminal) assert.equal(terminal[0].includes('disabled=""'), locked);
    }
  }
  const alert = fault(1, { raw_counters: { dot3StatsFCSErrors: '18446744073709551615' } });
  const normal = render(AlertEntry, { alert, busy: false });
  assert.match(normal, /Fault/);
  assert.doesNotMatch(normal, /dot3StatsFCSErrors|18446744073709551615/);
  const expert = render(AlertEntry, { alert, busy: false, expert: true });
  assert.match(expert, /dot3StatsFCSErrors/);
  assert.match(expert, /18446744073709551615/);
});

test('Expert recommendations show measured values and mark references without a verified edition', () => {
  setLang('ja');
  const { RecommendationTable } = require('../src/views/Recommendations.tsx');
  const rows = [
    { switch_id: 1, port: '1/0/1', code: 'NDI4_NEEDS_FLOWCTRL', current: 'disabled', recommended: 'enable', source: 'NETGEAR ProAV Guide' },
    { switch_id: 1, port: '1/0/2', code: 'QSYS_MTU', current: '9216', recommended: '1522' },
    { switch_id: 1, port: '1/0/3', code: 'EEE_ON_AV_PORT', current: 'enable', recommended: 'disable', lpi: '12', source: 'Example Guide', version: '2' },
  ];
  const html = render(RecommendationTable, { rows, names: new Map([[1, 'A']]) });
  assert.match(html, /1\/0\/1[^]*Flow Control[^]*disabled[^]*enable[^]*出典未登録/);
  assert.match(html, /1\/0\/2[^]*9216 B[^]*MTU 1500[^]*1522 B[^]*出典未登録/);
  assert.match(html, /LPI遷移: \+12[^]*参考: Example Guide（2）/);
  assert.doesNotMatch(html, /TODO|版未登録/);
});

test('Log search, live rows and export use JST datetime boundaries in a non-JST timezone', async () => {
  const previousZone = process.env.TZ;
  process.env.TZ = 'America/New_York';
  try {
    const filter = { category: '', severity: '', switch_id: '', q: '', from: '2026-03-08T02:30', to: '2026-03-08T03:00' };
    const from = '2026-03-07T17:30:00.000Z';
    const to = '2026-03-07T18:00:00.000Z';
    const live = ['2026-03-07T17:29:59.999Z', from, to, '2026-03-07T18:00:00.001Z']
      .map((ts, id) => ({ id, ts }));
    assert.deepEqual(logRows([], live, filter).map(event => event.id), [2, 1]);

    let load;
    let searched;
    const { Logs } = sourceWith('../src/views/Logs.tsx', {
      react: { ...React, useState: initial => [typeof initial === 'object' ? filter : initial, noop] },
      '../usePolling': { usePolling: callback => { load = callback; return {}; } },
      '../api': { api: async url => { searched = new URL(url, 'http://test.invalid').searchParams; } },
    }, { URLSearchParams });
    render(Logs, { overview: { switches: [] }, events: [] });
    await load(new AbortController().signal);
    assert.equal(searched.get('from'), from);
    assert.equal(searched.get('to'), to);

    const states = [[], '', 5, filter.from, filter.to];
    let exported;
    let request;
    const { ExportDialog } = sourceWith('../src/components/ExportDialog.tsx', {
      react: { useState: () => [states.shift(), noop], useEffect: noop },
      '../api': {
        json: (_method, body) => body, download: noop,
        response: async (_url, body) => { exported = body; return { blob: async () => ({}) }; },
      },
    });
    const tree = ExportDialog({ close: noop, run: action => { request = action(); } });
    elements(tree, node => node.type === 'button')[0].props.onClick();
    await request;
    assert.equal(exported.from, from);
    assert.equal(exported.to, to);
  } finally {
    if (previousZone === undefined) delete process.env.TZ;
    else process.env.TZ = previousZone;
  }
});

test('ログのクイック絞り込みと確認ボタンは対象行だけに適用する', () => {
  const events = [
    { id: 1, category: 'ANOMALY', severity: 'CRITICAL', switch_id: 7, switch_name: 'West' },
    { id: 2, category: 'TRAP', severity: 'WARNING', switch_id: 7, switch_name: 'West' },
    { id: 3, category: 'MARKER', severity: 'INFO', switch_id: 0 },
    { id: 4, category: 'ANOMALY', severity: 'WARNING', switch_id: 7, acked: true },
  ].map(event => ({ ts: '2026-11-07T15:16:45Z', title: 'Test', code: 'CUSTOM', ...event }));
  const alerts = [fault(1, { severity: 'CRITICAL' }), fault(2, { suppressed: true }), fault(3, { severity: 'INFO' }), fault(4, { acked: true })];
  const ids = acknowledgeableEventIds(alerts);
  const filter = { from: '', to: '', q: '', switch_id: '', severity: '', category: '' };
  const filtered = extra => logRows(events, [], { ...filter, ...extra }, ids).map(event => event.id);
  assert.deepEqual(filtered({ issuesOnly: true }), [1, 2, 4]);
  assert.deepEqual(filtered({ unackedOnly: true }), [1]);
  assert.deepEqual(filtered({ category: 'MARKER' }), [3]);
  assert.deepEqual(filtered({ category: 'ANOMALY', severity: 'WARNING', switch_id: '7' }), [4]);
  assert.deepEqual(logRows(events, [{ ...events[0], acked: true }], { ...filter, unackedOnly: true }, ids).map(event => event.id), [1]);
  const { Logs } = sourceWith('../src/views/Logs.tsx', { '../usePolling': { usePolling: () => ({ data: events }) } });
  const html = render(Logs, { overview: { switches: [] }, events, operations: { alerts }, locked: false });
  // Category, severity, switch and period live in the folded "more filters" panel.
  assert.equal((html.match(/<select/g) || []).length, 4);
  assert.match(html, /<option value="7">West<\/option>/);
  // Quick filters on one row; the default view is warnings and above.
  assert.match(html, /data-quick="all"[^]*data-quick="issues" aria-pressed="true"[^]*data-quick="unacked"[^]*data-quick="marker"/);
  assert.match(html, /data-action="ack-all"/);
  assert.match(html, /<details class="log-more">/);
  assert.equal((html.match(/data-action="ack"/g) || []).length, 1);
});

test('Log acknowledgement refreshes after remote ack and unack, including during a search', async () => {
  const event = { id: 1, ts: '2026-11-07T15:16:45Z', title: 'Fault', code: 'CUSTOM', category: 'ANOMALY', severity: 'WARNING', acked: false };
  const live = [{ ...event, acked: true }];
  const window = new EventTarget();
  const effects = [];
  const revision = { current: { observed: 0, requested: 0 } };
  const requests = [];
  let data;
  let load;
  const { createPoller } = require('../src/polling.ts');
  const poller = createPoller({
    load: signal => load(signal), onData: value => { data = value; }, onError: assert.fail,
    delayMs: 60000, intervalMs: 0,
  });
  const { Logs } = sourceWith('../src/views/Logs.tsx', {
    react: { ...React, useRef: () => revision, useEffect: effect => effects.push(effect) },
    '../usePolling': { usePolling: callback => { load = callback; return { data, refresh: poller.refresh }; } },
    '../api': { api: () => new Promise(resolve => requests.push(resolve)) },
  }, { window, URLSearchParams });
  const draw = alerts => render(Logs, { overview: { switches: [] }, events: live, operations: { alerts } });
  const notify = type => window.dispatchEvent(new CustomEvent('live-control', { detail: { type } }));
  const flush = () => new Promise(resolve => setImmediate(resolve));
  draw([fault(1)]);
  const cleanup = effects[0]();
  try {
    const initial = poller.refresh();
    await flush();
    notify('alerts_changed');
    requests[0]([event]);
    await initial;
    await flush();
    assert.equal(requests.length, 2);
    requests[1]([{ ...event, acked: true }]);
    await flush();
    assert.ok(draw([fault(1, { acked: true })]).includes(t('ui.m269')));

    notify('alerts_changed');
    await flush();
    assert.equal(requests.length, 3);
    requests[2]([event]);
    await flush();
    assert.match(draw([fault(1)]), /data-action="ack"/);
    // With nothing to acknowledge the acknowledgement column is hidden.
    assert.doesNotMatch(draw([]), /data-action="ack"|<th class="whitespace-nowrap">確認<\/th>/);
    notify('phase_changed');
    await flush();
    assert.equal(requests.length, 3);
  } finally {
    cleanup();
    poller.stop();
  }
});

test('同じスイッチとポートの同一分だけをまとめ、全メンバーを確認する', () => {
  const base = Math.floor(Date.now() / 60000) * 60000;
  const a = (id, ms, extra = {}) => fault(id, { switch_id: 1, if_index: 1, active: false, ts: new Date(base + ms).toISOString(), ...extra });
  const alerts = [a(1, 0), a(2, 59999), a(3, 60000), a(4, 2000, { if_index: 2 }), a(5, 3000, { switch_id: 2 })];
  const groups = groupWallAlerts(alerts);
  assert.equal(groups.length, 4);
  assert.deepEqual(groups[0].members.map(alert => alert.event_id), [3]);
  assert.deepEqual(groups[1].members.map(alert => alert.event_id), [2, 1]);
  const clicked = [];
  const tree = AlertBand({ alerts, wall: true, busy: false, ackMany: ids => clicked.push(ids) });
  const entries = elements(tree, node => node.type === AlertEntry && node.props.ack);
  entries[1].props.ack();
  assert.deepEqual(clicked[0], [2, 1]);
  elements(tree, node => node.props?.['data-action'] === 'ack-all')[0].props.onClick();
  assert.deepEqual(new Set(clicked[1]), new Set([1, 2, 3, 4, 5]));
  const html = render(AlertBand, { alerts, wall: true, ackMany: noop });
  assert.match(html, /<details><summary>Fault（関連2件）/);
  assert.match(html, /復旧・未確認/);
});

test('Alert band and bulk acknowledgement use the same eligible IDs', async () => {
  const alerts = [fault(1), fault(2, { severity: 'CRITICAL' }), fault(3, { suppressed: true, title: 'Suppressed fault' }),
    fault(4, { severity: 'INFO' }), fault(5, { acked: true, active: true })];
  const html = render(AlertBand, { alerts, ack: noop });
  assert.match(html, /Suppressed fault/);
  assert.equal((html.match(/data-action="ack"/g) || []).length, 2);
  assert.match(html, /data-action="ack-all"/);
  assert.doesNotMatch(render(AlertBand, { alerts: alerts.slice(2), ack: noop }), /data-action="ack/);

  const calls = [];
  let action;
  const { useOperations } = sourceWith('../src/useOperations.ts', {
    react: {
      useState: initial => [initial?.length === 0 ? alerts : initial, noop],
      useRef: current => ({ current }), useEffect: noop, useCallback: callback => callback, useMemo: callback => callback(),
    },
    './usePolling': { usePolling: () => ({}) },
    './acknowledge': { acknowledgeEvents: async ids => calls.push(...ids) },
  }, { window: { confirm: () => true } });
  const { ack } = useOperations({ online: true, replay: false, profileId: 1, setStatus: noop, run: callback => { action = callback(); } });
  ack(3);
  assert.equal(action, undefined);
  ack();
  await action;
  assert.deepEqual(calls, [1, 2]);
});

test('確認・取り消しは処理中に無効化し、ロック中は隠し、既知の復旧時刻だけ表示する', () => {
  const alert = fault(1, { active: true, ts: new Date().toISOString(), severity: 'CRITICAL' });
  const active = render(AlertBand, { alerts: [alert], wall: true, busy: true, ackMany: noop, undoCount: 1, undo: noop });
  assert.match(active, /wall-alert-group alert-active/);
  // Plan section 22: an ongoing WARNING group on the wall is orange, not the red group.
  assert.match(render(AlertBand, { alerts: [{ ...alert, severity: 'WARNING' }], wall: true, ackMany: noop }), /wall-alert-group wall-group-caution/);
  assert.match(active, /data-action="ack" disabled/);
  assert.match(active, /data-action="undo" disabled/);
  const locked = render(AlertBand, { alerts: [alert], wall: true, locked: true, ackMany: noop, undoCount: 1, undo: noop });
  assert.doesNotMatch(locked, /<button/);
  assert.match(locked, /ロック中のため確認不可/);
  const recovered = render(AlertBand, { alerts: [{ ...alert, active: false, cleared_at: new Date().toISOString() }], wall: true });
  assert.match(recovered, /alert-recovered/);
  assert.match(recovered, / 発生 → .* 復旧/);
  assert.doesNotMatch(active, /復旧:|復旧 <time/);
  assert.match(render(AlertEntry, { alert: { ...alert, severity: 'CRITICAL' } }), /class="error-symbol"[^>]*>!</);
  assert.doesNotMatch(active, /[✖✕]/);
});

test('壁タイルは現在の異常を3行に制限し、登録名・ポート名・残件数を表示する', () => {
  const alerts = Array.from({ length: 5 }, (_, i) => fault(i + 1, { active: true, port: `1/0/${i + 1}`, device_name: i ? '' : 'Mixer' }));
  const html = render(WallSwitchTile, { summary: { name: 'SW', reachable: true }, alerts, stale: false, onSelect: noop, selected: true });
  assert.equal((html.match(/<li>/g) || []).length, 3);
  assert.match(html, /Mixer · Fault/);
  assert.match(html, /1\/0\/2 · Fault/);
  assert.match(html, /他 2件/);
  assert.match(html, /aria-pressed="true"/);
});

test('確認取り消しは指定IDだけを重複なく送信し、失敗時も最新状態を取得する', async () => {
  const calls = []; const restored = []; let refreshed = 0;
  const { unacknowledgeEvents } = sourceWith('../src/acknowledge.ts', {
    './api': { json: method => ({ method }), api: async url => { calls.push(url); if (url.includes('/2/')) throw Error('locked'); } },
  });
  await assert.rejects(unacknowledgeEvents([1, 1, 2, 3, 4, 5], async () => { refreshed++; }, ids => restored.push(...ids)), /locked/);
  assert.deepEqual(calls, [1, 2, 3, 4].map(id => `/api/events/${id}/unack`));
  assert.deepEqual(restored, [1, 3, 4]);
  assert.equal(refreshed, 1);
});

function sourceWith(relative, overrides, globals = {}) {
  const filename = path.join(__dirname, relative);
  const realRequire = createRequire(filename);
  const result = ts.transpileModule(fs.readFileSync(filename, 'utf8'), {
    compilerOptions: { module: ts.ModuleKind.CommonJS, jsx: ts.JsxEmit.ReactJSX, target: ts.ScriptTarget.ES2020 }, fileName: filename,
  });
  const module = { exports: {} };
  vm.runInNewContext(result.outputText, { module, exports: module.exports, require: name => overrides[name] || realRequire(name), ...globals }, { filename });
  return module.exports;
}

function elements(tree, predicate) {
  if (!tree || typeof tree !== 'object') return [];
  return [ ...(predicate(tree) ? [tree] : []), ...React.Children.toArray(tree.props?.children).flatMap(child => elements(child, predicate)) ];
}

test('監視停止は切断・初回未更新・15秒未着で発火し、配信再開で解除する', () => {
  assert.equal(monitoringStopped(false, 2000, 2001), true);
  assert.equal(monitoringStopped(true, undefined, 2000), true);
  assert.equal(monitoringStopped(true, 2000, 16999), false);
  assert.equal(monitoringStopped(true, 2000, 17000), true);
  assert.equal(monitoringStopped(true, 17000, 17000), false);
  assert.match(render(MonitorStopped, { updatedAt: 2000 }), /監視停止中.*更新: 09:00:02 JST/);
});

test('総合状態は抑止とINFOを数えず、復旧未確認・発生中確認済みは残す', () => {
  const alerts = [fault(1), fault(2, { active: false }), fault(3, { acked: true, active: true }), fault(4, { suppressed: true }), fault(5, { severity: 'INFO' })];
  assert.deepEqual(issueAlerts(alerts).map(alert => alert.event_id), [1, 2, 3]);
  assert.deepEqual(newIssueAlerts(alerts, new Set([1])).map(alert => alert.event_id), [2]);
  for (const language of ['ja', 'en']) {
    setLang(language);
    assert.match(render(OverallStatus, { count: 0, ready: true, stopped: false }), /status-healthy/);
    assert.match(render(OverallStatus, { count: 2, critical: false, ready: true, stopped: false }), /status-warning/);
    assert.match(render(OverallStatus, { count: 1, critical: true, ready: true, stopped: false }), /status-critical/);
    const stale = render(OverallStatus, { count: 0, ready: true, stopped: true });
    assert.match(stale, /status-unknown/);
    assert.ok(!stale.includes(t('cleanup.normal')));
  }
  setLang('ja');
});

test('The default port monitor shows the unified layout and the three normal menu entries', () => {
  const App = sourceWith('../src/App.tsx', { './components/TerminalDrawer': { TerminalDrawer: () => null } }).default;
  const html = render(App);
  assert.match(html, /aria-current="page"[^]*?ポートモニター/);
  const nav = html.match(/<nav[^]*?<\/nav>/)[0];
  assert.match(nav, /接続機器.*ログ/);
  assert.doesNotMatch(nav, /詳細|トポロジー|俯瞰|ポート一覧|DHCP|マルチキャスト|ヘルス/);
  assert.match(html, /aria-label="表示方法"/);
  assert.match(html, />図<|>一覧</);
});

test('Port monitor clears navigation targets on a screen filter change and when port detail closes', () => {
  const { PortMonitor } = require('../src/views/PortMonitor.tsx');
  const state = [];
  let nextState = 0;
  const App = sourceWith('../src/App.tsx', {
    react: {
      useState: initial => {
        const index = nextState++;
        if (!(index in state)) state[index] = typeof initial === 'function' ? initial() : initial;
        return [state[index], value => { state[index] = value; }];
      },
      useCallback: callback => callback, useEffect: noop, useRef: current => ({ current }),
    },
    './components/TerminalDrawer': { TerminalDrawer: () => null },
    './i18n': { t, useLang: () => ['ja'] },
    './pcTime': { usePcTime: () => 0 },
    './useDesktopNotifications': { useDesktopNotifications: noop },
    './useLiveFeed': { useLiveFeed: () => ({}) },
    './useOperations': { useOperations: () => ({ alerts: [], groups: [] }) },
  }).default;
  const drawApp = () => { nextState = 0; return App(); };
  const monitor = tree => elements(tree, node => node.type === PortMonitor)[0].props;
  const target = { switchId: 1, ifIndex: 2 };

  monitor(drawApp()).openPortMonitor(target);
  const beforeGroupChange = drawApp();
  assert.equal(monitor(beforeGroupChange).target, target);
  monitor(beforeGroupChange).onFilterChange({ part: 'stage' });
  const afterGroupChange = drawApp();
  assert.equal(monitor(afterGroupChange).part, 'stage');
  assert.equal(monitor(afterGroupChange).target, undefined);

  monitor(afterGroupChange).openPortMonitor(target);
  // Plan section 21: opening a port shows that switch alone and hands the port to the single-switch view.
  let close;
  let requested;
  const { OverviewView } = sourceWith('../src/views/Overview.tsx', {
    './SwitchPorts': { SwitchPorts: props => { close = props.onClosePort; requested = props.initialPort; return null; } },
    '../useRackData': { useRackData: () => ({ data: { ports: [
      { switch_id: 1, port: { name: '1/0/2', if_index: 2, oper: 'up' } },
    ] }, definitions: {}, refresh: noop }) },
  });
  const props = monitor(drawApp());
  assert.equal(props.target, target);
  assert.equal(props.focus, 1);
  const html = render(OverviewView, { ...props, overview: { switches: [{ id: 1, name: 'Switch', group_tag: '', reachable: true }] } });
  assert.equal(requested, target);
  assert.match(html, /aria-label="ポートモニターの全体状況"/);
  assert.match(html, /focus-crumb[^]*>すべて<[^]*>未所属<[^]*aria-current="page">Switch</);
  close();
  assert.equal(monitor(drawApp()).target, undefined);
  // Choosing "all" in the index returns to the rack of every switch, without a group filter left over.
  monitor(drawApp()).onFilterChange({ group: 'FOH', part: '' });
  monitor(drawApp()).onFocus(2);
  monitor(drawApp()).onFocus(0);
  assert.equal(monitor(drawApp()).focus, 0);
  assert.equal(monitor(drawApp()).group, undefined);
});

test('Number panels show whole-project counts and stay unknown when a replay has no alert list', () => {
  setLang('ja');
  const { MonitorKpis } = require('../src/components/MonitorKpis.tsx');
  const switches = [{ reachable: true, ports_up: 10 }, { reachable: false, ports_up: 0 }];
  const alerts = [fault(1, { active: true, severity: 'CRITICAL' }), fault(2, { active: false }), fault(3, { active: true, suppressed: true })];
  const html = render(MonitorKpis, { switches, alerts, uplink: { rx: 2e9, tx: 1e9 } });
  // Expected from the inputs: 1 of 2 switches reachable, 1 ongoing, 1 recovered (the suppressed one is not an issue), 10 linked ports.
  assert.match(html, /kpi kpi-critical" data-kpi="switches"[^]*>1 \/ 2</);
  assert.match(html, /kpi kpi-critical" data-kpi="active"[^]*>1</);
  assert.match(html, /kpi kpi-warning" data-kpi="recovered"[^]*>1</);
  assert.match(html, /data-kpi="linked"[^]*>10</);
  assert.match(html, /data-kpi="uplink"[^]*受信 [^<]* \/ 送信 /);
  const legacy = render(MonitorKpis, { switches, alerts: [], alertsKnown: false });
  assert.match(legacy, /class="kpi" data-kpi="active"[^]*>—</);
  assert.match(legacy, /class="kpi" data-kpi="recovered"[^]*>—</);
  // An ongoing WARNING only is orange ("caution"), not red.
  assert.match(render(MonitorKpis, { switches, alerts: [fault(4, { active: true })] }), /kpi kpi-caution" data-kpi="active"[^]*>1</);
});

test('Only an ongoing CRITICAL alert is red; an ongoing WARNING is the orange triangle everywhere', () => {
  setLang('ja');
  const warning = fault(1, { switch_id: 1, if_index: 2, active: true, code: 'CLOCK_SKEW' });
  const critical = fault(2, { switch_id: 1, if_index: 3, active: true, severity: 'CRITICAL', code: 'LINK_DOWN' });
  const recovered = fault(3, { switch_id: 1, if_index: 4, active: false });
  const { AlertEntry } = require('../src/components/AlertBand.tsx');
  const { alertTone, summarizeAlerts } = require('../src/wallAlerts.ts');
  // Expected tones from the rule: critical > caution (ongoing warning) > warning (recovered, unacknowledged).
  assert.equal(alertTone(summarizeAlerts([warning]), true), 'caution');
  assert.equal(alertTone(summarizeAlerts([warning, critical]), true), 'critical');
  assert.equal(alertTone(summarizeAlerts([recovered]), true), 'warning');
  const entry = render(AlertEntry, { alert: warning, busy: false });
  assert.match(entry, /alert-entry alert-caution[^]*class="caution-symbol">▲</);
  assert.doesNotMatch(entry, /error-symbol/);
  assert.match(render(AlertEntry, { alert: critical, busy: false }), /alert-entry alert-critical[^]*error-symbol/);
  const banner = render(OverallStatus, { count: 1, critical: false, ready: true, stopped: false, active: 1, unacked: 1 });
  assert.match(banner, /status-caution[^]*caution-symbol/);
  assert.match(render(OverallStatus, { count: 1, critical: true, ready: true, stopped: false, active: 1, unacked: 1 }), /status-critical[^]*error-symbol/);
  assert.match(render(WallSwitchTile, { summary: { id: 1, name: 'West', reachable: true }, alerts: [warning] }), /status-caution[^]*caution-symbol[^]*発生中（警告）/);
  const css = fs.readFileSync(path.join(__dirname, '../src/style.css'), 'utf8');
  assert.match(css, /.status-caution {[^}]*background: #15181e/);
});

test('Switch index shows fault marks on every page and the list only on the port monitor', () => {
  setLang('ja');
  const { SwitchIndex, orderedGroups } = require('../src/components/SwitchIndex.tsx');
  const switches = [
    { id: 1, name: 'FOH-SW1', group_tag: 'FOH', reachable: true },
    { id: 2, name: 'CORE-SW1', group_tag: 'core', reachable: true },
    { id: 3, name: 'Loose', group_tag: '', reachable: false },
    { id: 4, name: 'STG-SW1', group_tag: 'Stage', reachable: true },
  ];
  // Expected order from the rule: registered groups in project order, then other names sorted, ungrouped last.
  assert.deepEqual(orderedGroups(switches, ['core', 'FOH']), ['core', 'FOH', 'Stage', '']);
  const alerts = [
    fault(1, { switch_id: 1, if_index: 14, active: true, severity: 'CRITICAL' }),
    fault(2, { switch_id: 4, if_index: 3, active: false }),
  ];
  const props = { switches, groups: ['core', 'FOH'], alerts, focus: 1, open: true, showList: true, onFocus: noop, onToggle: noop };
  const list = render(SwitchIndex, props);
  assert.match(list, /aria-label="発生中 1件 \/ 復旧・未確認 1件"/);
  assert.match(list, /aria-expanded="true"[^]*>▾</);
  assert.match(list, />すべて<[^]*core[^]*CORE-SW1[^]*FOH \(1\)[^]*aria-current="true"[^]*state-mark-critical[^]*FOH-SW1[^]*Stage \(1\)[^]*state-mark-warning[^]*STG-SW1[^]*未所属[^]*state-mark-unknown[^]*Loose/);
  const folded = render(SwitchIndex, { ...props, open: false });
  assert.doesNotMatch(folded, /FOH-SW1|<ul/);
  assert.match(folded, /aria-expanded="false"[^]*>▸<[^]*state-mark-critical[^]*1[^]*state-mark-warning[^]*1/);
  const otherPage = render(SwitchIndex, { ...props, showList: false });
  assert.doesNotMatch(otherPage, /FOH-SW1|aria-expanded/);
  assert.match(otherPage, /state-mark-critical/);
});

test('ヘッダーから音声・転送・言語を撤去し、設定には既定OFFの通知・CSV・表示順を置く', () => {
  const html = render(AppHeader, { online: true, wall: false, phase: 'show', profiles: [], part: '', status: { locked: false, profile: { id: 1 } }, run: noop });
  assert.doesNotMatch(html, /音あり|消音|表示言語|>書出<|type="file"/);
  const tools = [render(BrowserDisplaySettings, { theme: 'dark', notifications: false, run: noop }),
    render(DataTransfers, { groups: [{ name: 'Audio' }], replay: false, run: noop }), render(GroupOrder, { groups: [], run: noop })].join('');
  assert.match(tools, /表示言語|Language/);
  assert.match(tools, /type="checkbox"/);
  assert.doesNotMatch(tools, /checked=""/);
  assert.match(tools, /export\.csv/);
  assert.match(tools, /グループの表示順/);
  const phases = render(PhaseControls, { phase: 'show', locked: false, busy: false, wall: false });
  assert.equal((phases.match(/<option/g) || []).length, 3);
  assert.doesNotMatch(phases, /teardown|撤収/);
});

test('グループPingは対象機器だけ、全体Pingは重複IPをまとめ、リセットは指定グループへ送る', async () => {
  const calls = [];
  let action;
  const { GroupActions } = sourceWith('../src/components/GroupActions.tsx', {
    '../api': { json: (method, body) => ({ method, body }), api: async (url, options) => { calls.push([url, options]); return { ok: 1, results: [] }; } },
    '../groupRequest': { groupRequest: (_id, method, body) => ({ method, body }) },
  }, { window: { confirm: () => true } });
  const props = { group: 'Audio', locked: false, wall: false, busy: false, devices: [
    { ip: '192.0.2.1', group_tag: 'Audio' }, { ip: '192.0.2.2', group_tag: 'Video' }, { ip: '192.0.2.1', group_tag: 'Audio' },
  ], run: callback => { action = callback(); }, refresh: async () => {}, notice: noop, replay: false };
  const invoke = async (extra, name) => {
    const tree = GroupActions({ ...props, ...extra });
    const button = elements(tree, node => node.props?.['data-action'] === name)[0];
    button.props.onClick(); await action;
  };
  await invoke({}, 'ping');
  assert.deepEqual(Array.from(calls[0][1].body.ips), ['192.0.2.1']);
  await invoke({ all: true }, 'ping');
  assert.deepEqual(Array.from(calls[1][1].body.ips), ['192.0.2.1', '192.0.2.2']);
  await invoke({}, 'reset');
  assert.equal(calls[2][0], '/api/counters/reset');
  assert.equal(calls[2][1].body.group, 'Audio');
});

test('壁タイルはアラート集合で現在状態を表示し、ポートから件数を増やさない', () => {
  const props = { summary: { name: 'Audio SW', group_tag: 'Main', reachable: true }, ports: [{ name: '1/0/8', if_index: 8, oper: 'up' }], devices: [], stale: false };
  const html = render(WallSwitchTile, { ...props, alerts: [fault(1, { active: true, severity: 'CRITICAL', port: '1/0/8', device_name: 'Mixer' })] });
  assert.match(html, /status-critical/);
  assert.match(html, /Audio SW/);
  assert.match(html, /Mixer.*Fault/);
  assert.doesNotMatch(html, /port-lamp|<button/);
  assert.match(render(WallSwitchTile, { ...props, alerts: [], stale: true }), /status-unknown/);
  assert.match(render(WallSwitchTile, { ...props, alerts: [], ports: [{ ...props.ports[0], poe: { fault: 'overload' } }] }), /status-healthy/);
});

test('任意PC通知は新規の未確認異常だけを無音で通知し、履歴・復旧・確認では重複しない', () => {
  const refs = []; let index = 0; const effects = []; const delivered = [];
  class FakeNotification {
    static permission = 'granted';
    constructor(title, options) { delivered.push({ title, options }); }
    close() {}
  }
  const { useDesktopNotifications } = sourceWith('../src/useDesktopNotifications.ts', {
    react: { useRef: () => refs[index++] ||= { current: undefined }, useEffect: callback => effects.push(callback) },
  }, { Notification: FakeNotification });
  const draw = extra => {
    index = 0; effects.length = 0;
    useDesktopNotifications({ enabled: true, profileId: 1, ready: true, paused: false, alerts: [fault(1)], ...extra });
    effects.forEach(effect => effect());
  };
  draw({ enabled: false }); draw({ ready: false, alerts: [] }); draw({});
  assert.equal(delivered.length, 0);
  draw({ alerts: [fault(1), fault(2)] });
  assert.equal(delivered.length, 1); assert.equal(delivered[0].options.silent, true);
  draw({ alerts: [fault(1), fault(2, { active: false })] });
  draw({ alerts: [fault(1), fault(2), fault(3, { suppressed: true }), fault(4, { acked: true })] });
  assert.equal(delivered.length, 1);
  draw({ alerts: [fault(1), fault(2, { suppressed: true }), fault(3, { suppressed: true })] });
  draw({ alerts: [fault(1), fault(2), fault(3, { suppressed: true })] });
  assert.equal(delivered.length, 1, 'notified IDs survive phase suppression');
  draw({ alerts: [fault(1), fault(2), fault(3)] });
  assert.equal(delivered.length, 2, 'first appearance after suppression is still notified');
  draw({ alerts: [fault(1), fault(2), fault(3)] });
  assert.equal(delivered.length, 2);
  draw({ profileId: 2, alerts: [fault(5)] });
  assert.equal(delivered.length, 2);
  draw({ profileId: 2, paused: true, alerts: [fault(5), fault(6)] });
  draw({ profileId: 2, alerts: [fault(5), fault(6)] });
  assert.equal(delivered.length, 2);
  assert.ok(refs[0].current.ids.size <= 2);
});

test('Pre-show checklist is offered in setup and rehearsal, and hidden during show or replay', () => {
  const props = { online: true, wall: false, profiles: [], run: noop, openChecklist: noop };
  for (const phase of ['setup', 'rehearsal']) {
    assert.match(render(AppHeader, { ...props, phase }), /data-action="pre-show-checklist"/);
  }
  for (const extra of [{ phase: 'show' }, { phase: 'setup', replay: true }, { phase: undefined }]) {
    assert.doesNotMatch(render(AppHeader, { ...props, ...extra }), /data-action="pre-show-checklist"/);
  }
});

test('Selecting show in setup opens the App confirmation gate', () => {
  const { ShowChecklist } = require('../src/components/ShowChecklist.tsx');
  const state = [];
  let cursor;
  let phase = 'setup';
  let updateStatus;
  let gateEffect;
  const mockReact = {
    useState: initial => {
      const index = cursor++;
      if (!(index in state)) state[index] = typeof initial === 'function' ? initial() : initial;
      return [state[index], value => { state[index] = value; }];
    },
    useCallback: callback => callback,
    useEffect: (effect, deps) => {
      if (deps?.[0] === 'show-gate') gateEffect = effect;
    },
    useRef: current => ({ current }),
  };
  const App = sourceWith('../src/App.tsx', {
    react: mockReact,
    './components/TerminalDrawer': { TerminalDrawer: () => null },
    './i18n': { t, useLang: () => ['en'] },
    './pcTime': { usePcTime: () => 0 },
    './useDesktopNotifications': { useDesktopNotifications: noop },
    './useLiveFeed': { useLiveFeed: () => ({}) },
    './useOperations': { useOperations: ({ setStatus }) => {
      updateStatus = setStatus;
      return { phase, alerts: [], groups: [], setPhase: () => assert.fail('Show requires confirmation') };
    } },
  }).default;
  const draw = () => { cursor = 0; return App(); };
  const gate = tree => elements(tree, node => node.type === ShowChecklist)[0];
  const header = sourceWith('../src/components/AppHeader.tsx', {
    react: { useState: value => [value, noop] },
  }).AppHeader;

  draw();
  updateStatus({ profile: { id: 1 }, locked: false });
  const before = draw();
  assert.equal(gate(before), undefined);
  const [appHeader] = elements(before, node => node.type === AppHeader);
  const [phaseControl] = elements(header(appHeader.props), node => node.type === PhaseControls);
  const [selector] = elements(PhaseControls(phaseControl.props), node => node.props?.['data-action'] === 'phase');
  selector.props.onChange({ target: { value: 'show' } });
  assert.equal(gate(draw())?.props.gate, true);
  gateEffect();
  assert.equal(gate(draw())?.props.gate, true);
  phase = 'show';
  assert.equal(gate(draw()), undefined);
  gateEffect();
  phase = 'setup';
  assert.equal(gate(draw()), undefined);
});

test('Setup and rehearsal to show open the gate and only explicit confirmation requests remainder logging', async () => {
  let opened = 0;
  const requests = [];
  const phaseChanges = [];
  const phaseControls = sourceWith('../src/components/PhaseControls.tsx', {
    '../api': {
      api: async (url, options) => { requests.push({ url, ...options }); return { phase: 'show' }; },
      json: (method, payload) => ({ method, body: JSON.stringify(payload) }),
    },
  }, { window: { confirm: () => assert.fail('The checklist is the confirmation dialog') } });
  const { PhaseControls: Selector, submitPhase } = phaseControls;
  for (const phase of ['setup', 'rehearsal', 'show']) {
    const selector = Selector({ phase, openChecklist: () => { opened++; },
      run: () => assert.fail('Opening the checklist must not send a phase change'), onPhase: noop });
    elements(selector, element => element.type === 'select')[0].props.onChange({ target: { value: 'show' } });
  }
  assert.equal(opened, 2);
  assert.deepEqual(requests, []);

  const result = { data: { items: [{ id: 'clock', state: 'review', reason: 'Clock data unavailable',
    action: 'Check synchronization', findings: [{ state: 'review', reason: 'Unavailable', target: { switch_id: 2, label: 'Switch B' } }] }] },
    pending: false, error: '' };
  const { ShowChecklist } = sourceWith('../src/components/ShowChecklist.tsx', {
    '../usePolling': { usePolling: () => result, STATE_REFRESH_MS: 5000 },
  });
  let closed = 0;
  let action;
  const targetCalls = [];
  const draw = gate => ShowChecklist({ gate, busy: false, locked: false, close: () => { closed++; },
    onTarget: target => targetCalls.push(target), onStart: confirmed => { action = submitPhase('show', phase => phaseChanges.push(phase), confirmed); } });
  const findButton = (tree, key) => elements(tree, element => element.type === 'button' && element.props.children === t(key))[0];
  assert.equal(findButton(draw(false), 'checklist.startWithRemaining'), undefined);
  const gate = draw(true);
  assert.match(render(() => gate, {}), /Switch B.*Unavailable.*Check synchronization/);
  const rows = elements(gate, element => element.type?.name === 'ChecklistRows')[0];
  elements(rows.type(rows.props), element => element.type === 'button')[0].props.onClick();
  assert.deepEqual(targetCalls, [{ switch_id: 2, label: 'Switch B' }]);
  findButton(gate, 'checklist.cancel').props.onClick();
  assert.equal(closed, 1);
  assert.deepEqual(requests, []);
  findButton(gate, 'checklist.startWithRemaining').props.onClick();
  await action;
  assert.equal(requests[0].url, '/api/phase');
  assert.equal(requests[0].method, 'PUT');
  assert.deepEqual(JSON.parse(requests[0].body), { phase: 'show', confirm_checklist: true });
  assert.deepEqual(phaseChanges, ['show']);
  result.data.items[0] = { id: 'clock', state: 'pass', reason: 'Within 60 seconds' };
  findButton(draw(true), 'checklist.start').props.onClick();
  await action;
  assert.deepEqual(JSON.parse(requests[1].body), { phase: 'show', confirm_checklist: false });
});

test('The post-show report defaults to latest and supports a JST range', () => {
  const { showReportURL } = require('../src/components/ShowReportButton.tsx');
  assert.equal(showReportURL(), '/api/export/show-report.csv');
  const query = new URLSearchParams(showReportURL('2024-01-03T01:00', '2024-01-03T02:00').split('?')[1]);
  assert.equal(query.get('from'), '2024-01-02T16:00:00.000Z');
  assert.equal(query.get('to'), '2024-01-02T17:00:00.000Z');
  assert.throws(() => showReportURL('2024-01-03T01:00'));
});

test('Switch health and the clock warning show the measured offset in both languages', () => {
  let state;
  const { HealthAudit } = sourceWith('../src/views/HealthAudit.tsx', {
    '../usePolling': { usePolling: () => ({}) },
    '../useSwitchState': { useSwitchState: () => ({ state, polling: {} }) },
    '../components/Common': { Chart: () => null, Table: () => null },
  });
  const { eventTitle } = require('../src/i18n/index.ts');
  const props = { selected: 1, overview: { switches: [{ id: 1, name: 'Stage', group_tag: '' }] } };
  try {
    for (const [language, skew, missing] of [
      ['ja', /監視PCとの時刻差 -61秒/, /要確認.*測定値/],
      ['en', /Clock offset from monitoring PC: -61 s/, /needs review.*no measurement/],
    ]) {
      setLang(language);
      state = { id: 1, clock_offset_seconds: -61 };
      assert.match(render(HealthAudit, props), skew);
      assert.match(eventTitle({ code: 'CLOCK_SKEW', detail: { offset_seconds: -61 } }), skew);
      state = { id: 1 };
      assert.match(render(HealthAudit, props), missing);
    }
  } finally {
    setLang('ja');
  }
});

test('The post-show report failure is visible next to its button and clears on retry', async () => {
  const failure = 'Report download failed';
  let error = '';
  let fail = true;
  let action;
  let downloaded = false;
  const { ShowReportButton } = sourceWith('../src/components/ShowReportButton.tsx', {
    react: { useState: () => [error, value => { error = value; }] },
    '../api': {
      message: cause => cause.message,
      responseBlob: async () => { if (fail) throw new Error(failure); return {}; },
      download: () => { downloaded = true; },
    },
  });
  const draw = () => ShowReportButton({ busy: false, run: callback => { action = callback(); } });
  const click = () => elements(draw(), node => node.type === 'button')[0].props.onClick();
  click();
  await action;
  const html = renderToStaticMarkup(draw());
  assert.match(html, /role="alert"[^>]*>[^<]*Report download failed/);
  assert.equal(downloaded, false);
  fail = false;
  click();
  await action;
  assert.equal(downloaded, true);
  assert.equal(error, '');
});

test('設定ファイルの選択とドロップは同じ入口を使い、ロック中は受け付けない', () => {
  const file = { name: 'avline-sight.config.json', size: 64 };
  const files = [file];
  const received = [];
  let dragging = false;
  let prevented = 0;
  const { ConfigFilePicker } = sourceWith('../src/components/ConfigImportDialog.tsx', {
    react: { useState: () => [dragging, value => { dragging = value; }] },
  });
  const draw = disabled => ConfigFilePicker({ disabled, onFiles: value => received.push(value) });
  const event = { preventDefault: () => { prevented++; }, dataTransfer: { files } };
  draw(false).props.onDragOver(event);
  assert.equal(event.dataTransfer.dropEffect, 'copy');
  assert.equal(dragging, true);
  draw(false).props.onDrop(event);
  assert.equal(dragging, false);
  const input = elements(draw(false), node => node.type === 'input')[0];
  assert.equal(input.props.type, 'file');
  const currentTarget = { files, value: file.name };
  input.props.onChange({ currentTarget });
  assert.equal(currentTarget.value, '');
  assert.deepEqual(received, [files, files]);
  draw(true).props.onDragOver(event);
  draw(true).props.onDrop(event);
  assert.equal(event.dataTransfer.dropEffect, 'none');
  assert.equal(prevented, 4);
  assert.deepEqual(received, [files, files]);
});

test('設定ファイルの要約と差分を表示し、適用時だけ確認済みのファイルを送る', async context => {
  context.after(() => setLang('ja'));
  setLang('ja');
  const file = { name: 'avline-sight.config.json', size: 128 };
  const preview = { profile_id: 7, revision: 'preview-token', switch_count: 2, group_count: 1, changes: [
    { field: 'settings.setup_ping_interval_sec', before: 60, after: 30 },
    { field: 'settings.expert_display_default', before: false, after: true },
  ] };
  const requests = [];
  let state = { phase: 'idle' };
  const action = {};
  const { ConfigImportDialog, ConfigImportPreview } = sourceWith('../src/components/ConfigImportDialog.tsx', {
    react: { useState: () => [state, value => { state = value; }], useRef: () => action, useEffect: noop },
    '../api': { api: async (url, options) => { requests.push({ url, options }); return preview; }, message: error => error.message },
  }, { AbortController });
  let cancelled = 0;
  let applied = 0;
  const draw = locked => ConfigImportDialog({ locked, close: () => { cancelled++; }, onApplied: async () => { applied++; } });
  const button = (key, locked = false) => elements(draw(locked), node => node.type === 'button' && node.props.children === t(key))[0];
  const pick = () => elements(draw(false), node => node.type?.name === 'ConfigFilePicker')[0].props.onFiles([file]);
  assert.equal(button('configFile.apply').props.disabled, true);
  const pending = pick();
  assert.equal(state.phase, 'previewing');
  assert.equal(button('configFile.apply').props.disabled, true);
  await pending;
  const confirm = elements(draw(false), node => node.type === ConfigImportPreview)[0];
  assert.equal(confirm.props.preview, preview);
  const html = render(ConfigImportPreview, confirm.props);
  assert.match(html, /スイッチ 2 台、グループ 1 件、変更項目 2 件/);
  assert.match(html, /現在の設定との差分/);
  assert.match(html, /現在<\/strong><pre[^>]*>60<\/pre>[^]*読込後<\/strong><pre[^>]*>30<\/pre>/);
  assert.match(html, /オフ[^]*オン/);
  assert.equal(button('configFile.apply', true).props.disabled, true);
  await button('configFile.apply', true).props.onClick();
  button('checklist.cancel').props.onClick();
  assert.equal(cancelled, 1);
  assert.deepEqual(requests.map(request => request.url), ['/api/profiles/import/preview']);
  state = { phase: 'idle' };
  await pick();
  const writing = button('configFile.apply').props.onClick();
  assert.equal(button('checklist.cancel').props.disabled, true);
  await writing;
  assert.equal(applied, 1);
  assert.deepEqual(requests.map(request => request.url), ['/api/profiles/import/preview', '/api/profiles/import/preview', '/api/profiles/import']);
  assert.equal(requests[2].options.body, file);
  assert.equal(requests[2].options.headers['X-AVLine-Profile'], '7');
  assert.equal(requests[2].options.headers['X-AVLine-Config-Revision'], 'preview-token');
});

test('stale switch filters are cleared and empty part presets do not count as a filter', () => {
  const { pruneSwitchFilter } = require('../src/components/SwitchFilters.tsx');
  const switches = [{ id: 1, name: 'A', group_tag: 'core' }, { id: 2, name: 'B', group_tag: 'edge' }];
  const parts = [{ id: 'all', name: 'All' }, { id: 'audio', name: 'Audio', vlans: [10] }];
  assert.equal(pruneSwitchFilter({ group: 'core', part: 'audio', switchId: 1 }, switches, ['core', 'edge'], parts), undefined);
  assert.deepEqual(pruneSwitchFilter({ group: 'old', part: '', switchId: 9 }, switches, ['core', 'edge'], parts),
    { group: undefined, part: '', switchId: undefined });
  assert.deepEqual(pruneSwitchFilter({ part: 'all' }, switches, [], parts), { group: undefined, part: '', switchId: undefined });
  // While switches are still loading, nothing is cleared.
  assert.equal(pruneSwitchFilter({ group: 'old', part: '' }, [], [], parts), undefined);
});

test('logs and expert detail tabs get the team filter, the wall display never does', () => {
  const app = fs.readFileSync(path.join(__dirname, '../src/App.tsx'), 'utf8');
  assert.match(app, /page === 'logs' && <>\{teamFilterRow\}<Logs key=\{logKey\} \{\.\.\.props\} \{\.\.\.activeTeamFilter\} \/>/);
  for (const view of ['MulticastView', 'DhcpView', 'HealthAudit', 'ConfigChanges', 'Recommendations']) {
    assert.match(app, new RegExp(`<${view} [^>]*\{\.\.\.activeTeamFilter\}`));
  }
  assert.match(app, /\.\.\.\(wall \? emptySwitchFilter : monitorFilter\)/);
});

test('Consecutive syslog lines of the same kind collapse into one row with a count', () => {
  const { collapseRepeated } = require('../src/views/Logs.tsx');
  const line = (id, port, name) => ({ id, ts: '2026-11-07T15:16:45Z', category: 'SYSLOG', severity: 'INFO', code: '',
    title: `LLDP: neighbor keepalive on ${port}`, switch_id: id, if_index: 0, acked: false, switch_name: name });
  const warn = { id: 9, ts: '2026-11-07T15:16:45Z', category: 'ANOMALY', severity: 'WARNING', code: 'X', title: 'Fault',
    switch_id: 1, if_index: 0, acked: false };
  const rows = collapseRepeated([line(1, '1/0/1', 'A'), line(2, '1/0/9', 'B'), line(3, '1/0/30', 'A'), warn, line(4, '1/0/2', 'A')]);
  assert.deepEqual(rows.map(row => [row.event.id, row.count]), [[1, 3], [9, 1], [4, 1]]);
  assert.deepEqual(rows[0].switches, ['A', 'B']);
});

test('Entering the show switches a remembered Ping target to manual entry; a vanished group never pings everything', () => {
  const draw = choice => {
    const { Devices } = sourceWith('../src/views/Devices.tsx', {
      '../usePolling': { usePolling: (_load, _interval, [key]) => ({ data: { key, rows: [{ ip: '192.0.2.1', group_tag: 'Audio' }] } }) },
      react: { useState: value => [value && typeof value === 'object' && 'target' in value ? choice : value, noop] },
    });
    const tree = Devices({ replay: null, status: { phase: 'show', profile: { id: 1 } }, run: noop, notice: noop });
    return elements(tree, node => node.props?.['data-action'] === 'ping-target')[0].props.value;
  };
  assert.equal(draw({ target: 'visible', phase: 'rehearsal' }), 'manual');
  assert.equal(draw({ target: 'visible', phase: 'show' }), 'visible');
  assert.equal(draw({ target: 'group:Gone', phase: 'show' }), 'manual');
  assert.equal(draw({ target: 'group:Audio', phase: 'show' }), 'group:Audio');
});

test('Wall alert groups put ongoing faults before recovered ones', () => {
  const { groupWallAlerts } = require('../src/wallAlerts.ts');
  const alert = (id, active, ts, port) => ({ event_id: id, active, acked: false, severity: 'CRITICAL', ts, switch_id: 1, if_index: port, code: 'X', title: 'X' });
  const groups = groupWallAlerts([alert(1, false, '2026-11-25T01:00:00Z', 2), alert(2, false, '2026-11-25T00:59:00Z', 3), alert(3, true, '2026-11-25T00:10:00Z', 4)]);
  assert.equal(groups[0].alert.event_id, 3);
});

test('Normal display headline has no counter values or location; expert keeps the full title', () => {
  setLang('ja');
  const { eventHeadline, eventTitle } = require('../src/i18n/index.ts');
  const phy = { code: 'PHY_ERRORS', detail: { fcs: 22, symbol: 6, align: 0 } };
  assert.equal(eventHeadline(phy), '物理エラーが増加');
  assert.match(eventTitle(phy), /FCS\+22/);
  assert.equal(eventHeadline({ code: 'PING_FAIL', detail: { name: 'Mixer', last_ip: '192.0.2.1' } }), 'Ping応答なし');
  setLang('en');
  assert.equal(eventHeadline(phy), 'Physical layer errors increasing');
  setLang('ja');
});

test('Every alert headline is free of counter values, placeholders and dangling text in both languages', () => {
  const { eventHeadline } = require('../src/i18n/index.ts');
  const { eventJa } = require('../src/i18n/eventJa.ts');
  const detail = new Proxy({}, { get: (_target, key) => typeof key === 'string' ? '987654' : undefined, has: () => true });
  for (const lang of ['ja', 'en']) {
    setLang(lang);
    for (const code of Object.keys(eventJa)) {
      if (code === 'MARKER' || code === 'PORT_CONFIG_CHANGED') continue;
      const headline = eventHeadline({ code, detail });
      assert.ok(headline && !/987654|\{|\}/.test(headline), `${lang} ${code}: ${headline}`);
      assert.doesNotMatch(headline, /^[\s:：のをが、。]/, `${lang} ${code}: ${headline}`);
    }
  }
  setLang('ja');
});

test('Switch details show facts only and list a different firmware of the same model without advice', () => {
  setLang('ja');
  const { DeviceInfo, bootVersion, uptimeText } = require('../src/components/DeviceInfo.tsx');
  // Expected values from the real switch's sysDescr and the rule "last field when it looks like a version".
  assert.equal(bootVersion('M4250-26G4F-PoE+ 26x1G PoE+ 300W and 4xSFP Managed Switch, 13.0.7.6, 1.0.0.13'), '1.0.0.13');
  assert.equal(bootVersion('M4250 Managed Switch, 13.0.7.6'), '');
  assert.equal(bootVersion(undefined), '');
  assert.equal(uptimeText(90061), '1日 1時間 1分');
  assert.equal(uptimeText(0), '');
  const device = { id: 1, name: 'Core', ip: '192.0.2.1', model: 'M4250-26G4F-PoE+', fw: '13.0.7.6', serial: 'TESTSERIAL01',
    base_mac: '02:00:5E:00:00:01', uptime_sec: 3600, sys_descr: 'M4250, 13.0.7.6, 1.0.0.13' };
  const peers = [
    { id: 1, name: 'Core', model: 'M4250-26G4F-PoE+', fw: '13.0.7.6' },
    { id: 2, name: 'FOH', model: 'M4250-26G4F-PoE+', fw: '13.0.6.0' },
    { id: 3, name: 'Stage', model: 'M4350-24X4V', fw: '14.0.1.2' },
  ];
  const html = render(DeviceInfo, { device, peers });
  assert.match(html, /<summary>本体情報<span class="device-info-flag">ファームウェアが他と異なる<\/span><\/summary>/);
  assert.match(html, /<dt>シリアル番号<\/dt><dd>TESTSERIAL01<\/dd>[^]*<dt>ブートバージョン<\/dt><dd>1\.0\.0\.13<\/dd>[^]*<dt>稼働時間<\/dt><dd>0日 1時間 0分<\/dd>/);
  assert.match(html, /同じ機種でファームウェアが異なるスイッチ: FOH（13\.0\.6\.0）/);
  // A different model is not compared, and nothing tells the user to update.
  assert.doesNotMatch(html, /Stage|14\.0\.1\.2|更新|最新|推奨/);
  const same = render(DeviceInfo, { device, peers: peers.slice(0, 1) });
  assert.doesNotMatch(same, /device-info-flag|device-info-peers/);
});

test('Unused LAG interfaces are hidden from the port panels but a LAG that is linked or has an alert stays', () => {
  const { withoutUnusedLags } = require('../src/rackModel.ts');
  const ports = [
    { if_index: 1, name: '0/1', type: 'rj45', oper: 'down' },
    { if_index: 50, name: 'lag 1', type: 'lag', oper: 'down' },
    { if_index: 51, name: 'lag 2', type: 'lag', oper: 'up' },
    { if_index: 52, name: 'lag 3', type: 'lag', oper: 'down' },
  ];
  // lag 3 went down and its alert is still unacknowledged, so it must stay visible.
  const alerts = [fault(1, { switch_id: 1, if_index: 52, active: true, severity: 'CRITICAL', code: 'LINK_DOWN' })];
  assert.deepEqual(withoutUnusedLags(ports, alerts).map(port => port.name), ['0/1', 'lag 2', 'lag 3']);
  assert.deepEqual(withoutUnusedLags(ports, []).map(port => port.name), ['0/1', 'lag 2']);
});

test('VLAN card lists settings as facts and the port-description text as written text, without judging', () => {
  setLang('ja');
  const { VlanFacts, writtenText } = require('../src/components/VlanFacts.tsx');
  // Expected values from the real switch (FW 13.0.7.6) after the profiles were applied.
  assert.equal(writtenText('VID:13,HControl-Pri,Video NDI5 with Dante,Q-Sys or AES67 audio'), 'Video NDI5 with Dante,Q-Sys or AES67 audio');
  assert.equal(writtenText('Stage rack 2'), 'Stage rack 2');
  assert.equal(writtenText(''), '');
  const vlans = [
    { vlan: 11, name: 'Dante-Pri', snooping: true, fast_leave: true, querier_oper: 'querier', last_querier: '' },
    { vlan: 14, name: 'HControl-Sec', snooping: false, fast_leave: false, querier_oper: 'disabled', last_querier: '0.0.0.0' },
  ];
  const ports = [
    { name: '0/1', type: 'rj45', oper: 'up', pvid: 11, max_frame: 9198, config_settings: { bcast_storm_ctl: 'enabled' }, alias: 'VID:11,Dante-Pri,Video' },
    { name: '0/3', type: 'rj45', oper: 'down', pvid: 11, max_frame: 1500, config_settings: { bcast_storm_ctl: 'enabled' }, alias: 'VID:11,Dante-Pri,Lighting' },
    { name: '0/5', type: 'rj45', oper: 'up', pvid: 11, max_frame: 9198, config_settings: { bcast_storm_ctl: 'enabled' }, alias: 'VID:11,Dante-Pri,Video' },
    // 0/14: the storm-control setting could not be read (no config_settings value), so nothing is said about it.
    { name: '0/14', type: 'rj45', oper: 'down', pvid: 14, max_frame: 1500, bcast_storm_ctl: false, alias: '' },
    { name: 'lag 1', type: 'lag', oper: 'down', pvid: 1 },
  ];
  const html = render(VlanFacts, { vlans, ports });
  assert.match(html, /VLAN 11<\/strong><span>Dante-Pri<\/span><span class="vlan-facts-ports">ポート 3（接続中 2）<\/span>/);
  assert.match(html, /Snooping 有効 · クエリア このスイッチ · Fast Leave 有効 · フレームサイズ 9198 ×2 \/ 1500 ×1 · ブロードキャスト制御 有効/);
  assert.match(html, /vlan-facts-written" title="[^"]*連動しません[^"]*"><span class="vlan-facts-label">記入<\/span><q>Video ×2 \/ Lighting ×1<\/q>/);
  // 0.0.0.0 is not a querier; a VLAN without written text shows no "written" row; the unused LAG adds no VLAN 1.
  assert.match(html, /VLAN 14[^]*クエリア なし/);
  assert.doesNotMatch(html.slice(html.indexOf('VLAN 14')), /ブロードキャスト制御/);
  // Waiting as non-querier with no real address is "unknown", not "another switch"; whole-switch snooping off is stated.
  const waiting = render(VlanFacts, { vlans: [{ ...vlans[0], querier_oper: 'non-querier', last_querier: '0.0.0.0' }], ports: ports.slice(0, 1), globalSnoopingOff: true });
  assert.match(waiting, /Snooping 有効（スイッチ全体は無効） · クエリア 不明（待機中）/);
  assert.doesNotMatch(html.slice(html.indexOf('VLAN 14')), /vlan-facts-written/);
  assert.doesNotMatch(html, /VLAN 1<\/strong>/);
  // Facts only: no judgement words in what is shown (the help tooltip explains where the written text comes from).
  assert.doesNotMatch(html.replace(/ title="[^"]*"/g, ''), /推奨|異常|要確認|プロファイル/);
});

test('Link change shows time today, date otherwise, and how long ago; unknown stays a dash', () => {
  setLang('ja');
  const { linkChangeText } = require('../src/views/SwitchPorts.tsx');
  const now = Date.parse('2026-11-25T08:00:00Z'); // 17:00 JST
  // Expected from the definition: 14:28:00 JST today is 2 h 32 min before 17:00 JST.
  assert.deepEqual(linkChangeText('2026-11-25T05:28:00Z', now), { when: '14:28:00 から', ago: '2時間前' });
  assert.deepEqual(linkChangeText('2026-11-25T07:50:00Z', now), { when: '16:50:00 から', ago: '10分前' });
  const older = linkChangeText('2026-11-20T00:00:00Z', now);
  assert.match(older.when, /^2026-11-20 09:00:00 から$/);
  assert.equal(older.ago, '5日前');
  assert.equal(linkChangeText('0001-01-01T00:00:00Z', now), null);
  assert.equal(linkChangeText('', now), null);
  const css = fs.readFileSync(path.join(__dirname, '../src/style.css'), 'utf8');
  assert.match(css, /\.focus-ports-table \.col-extra \{ display: none; \}/);
  assert.match(css, /@container \(min-width: 1100px\) \{\s*\.focus-ports-table \.col-extra \{ display: table-cell; \}/);
});

test('Ping all reports running, then the result with the devices that did not answer, through onPing only', async () => {
  const outcomes = [];
  const notices = [];
  let action;
  const { GroupActions } = sourceWith('../src/components/GroupActions.tsx', {
    '../api': { json: (method, body) => ({ method, body }), api: async () => ({ ok: 1, total: 2, results: [
      { ip: '192.0.2.1', status: 'ok', rtt_ms: 1 }, { ip: '192.0.2.2', status: 'timeout', rtt_ms: 0 },
    ] }) },
  }, { window: { confirm: () => true } });
  const props = { group: 'Audio', locked: false, wall: false, busy: false, replay: false, phase: 'setup',
    devices: [{ ip: '192.0.2.1', group_tag: 'Audio', label: 'Mixer' }, { ip: '192.0.2.2', group_tag: 'Audio', label: 'Stage box' }],
    run: callback => { action = callback(); }, refresh: async () => {}, notice: value => notices.push(value), onPing: value => outcomes.push(value) };
  elements(GroupActions(props), node => node.props?.['data-action'] === 'ping')[0].props.onClick();
  await action;
  assert.equal(outcomes[0].running, 2);
  assert.equal(outcomes[1].ok, 1);
  assert.equal(JSON.stringify(outcomes[1].noReply.map(row => [row.ip, row.names])), JSON.stringify([['192.0.2.2', ['Stage box']]]));
  assert.deepEqual(notices, []);
});

test('the pre-show checklist opens Settings as a page, not a removed dialog', () => {
  const app = fs.readFileSync(path.join(__dirname, '../src/App.tsx'), 'utf8');
  assert.match(app, /setDialog\(target\.destination === 'trap' \? 'trap' : ''\)/);
  assert.match(app, /target\.destination === 'settings'\) \{\s*setMonitorTarget\(undefined\);\s*setPage\('settings'\)/);
  assert.doesNotMatch(app, /setDialog\('settings'\)|dialog === 'settings'/);
});
