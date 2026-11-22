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

const assert = require('node:assert/strict');
const React = require('react');
const path = require('node:path');
const vm = require('node:vm');
const { createRequire } = require('node:module');
const { setLang, t } = require('../src/i18n/index.ts');
const { monitoringStopped, issueAlerts, newIssueAlerts } = require('../src/monitorHealth.ts');
const { OverallStatus, MonitorStopped } = require('../src/components/MonitorStatus.tsx');
const { WallSwitchTile } = require('../src/components/WallSwitchTile.tsx');
const { SettingsTools } = require('../src/components/SettingsTools.tsx');
const { AppHeader } = require('../src/components/AppHeader.tsx');
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
const headers = html => [...html.matchAll(/<th\b[^>]*>([^<]+)<\/th>/g)].map(match => match[1]);

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
      header(first).setGroupSelection('group:west');
      cursor = 0;
      const filtered = App();
      const { summary } = header(filtered);
      assert.equal(header(filtered).wall, wall);
      assert.equal(summary.props.count, count);
      assert.ok(render(summary.type, summary.props).includes(`>${t('cleanup.overall')}</span>`));
      assert.equal(document.title, `(${count}) AVLine-Sight · ${t('cleanup.overall')}`);
      const html = render(OverviewView, monitor(filtered));
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
    { group_tag: 'west', mac: '02:00:00:00:00:01', label: '=West, "Mixer"' },
    { group_tag: 'east', mac: '02:00:00:00:00:02', label: 'East camera' },
    { group_tag: 'other', mac: '02:00:00:00:00:03', label: 'Excluded' },
  ];
  const downloads = [];
  let loading = false;
  const { Devices, DeviceInventoryTable: InventoryTable } = sourceWith('../src/views/Devices.tsx', {
    react: { ...React, useState: initial => [initial, noop] },
    '../api': { download: (blob, name) => downloads.push({ blob, name }) },
    '../usePolling': { usePolling: (_load, _interval, [key]) => ({ data: loading ? undefined : { key, rows } }) },
  });
  const props = { part: 'show', settings: { views: [{ id: 'show', tags: ['west', 'east'] }] } };
  for (const replay of [null, []]) {
    const replayRef = replay && { id: 'recording', ts: '2026-01-01T00:00:00Z' };
    const draw = () => Devices({ ...props, replay, replayRef });
    const exportButton = tree => elements(tree, node => node.type === 'button' && node.props.children === t('ui.m235'))[0];
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
      const html = render(ConfigChanges, props);
      assert.ok(html.includes(heading));
      assert.ok(html.includes('Show'));
      assert.ok(html.includes(`1/0/7 ${field} 1522 → 9216`));
      assert.equal(html.split(field).length - 1, 2);
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
    operations: { ready: true, alerts: [fault(1, { switch_id: 1, if_index: 11, active: true, code: 'PHY_ERRORS',
      device_name: 'Mixer', port: '1/0/11' })] },
  });
  const legend = html.match(/<ul class="rack-legend"[^]*?<\/ul>/)[0];
  for (const [tone, symbol, label] of [
    ['critical', '!', '異常'], ['warning', '▲', '未確認'], ['healthy', '✓', '正常'],
    ['unused', '·', '未使用'], ['unknown', '?', '不明'],
  ]) {
    assert.ok(legend.includes(`class="lamp-${tone}" aria-hidden="true">${symbol}</span>${label}`));
  }
  assert.ok(html.indexOf(legend) < html.indexOf('class="rack-groups"'));
  assert.match(html, /class="lamp-number">11<\/span><span class="lamp-symbol" aria-hidden="true">!<\/span>/);
  assert.match(html, /Mixer · 1\/0\/11 物理エラー/);
  assert.match(html, /title="電源 · 異常"/);
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
  const tree = Devices({ ...props, replay: null, status: { phase: 'show', profile: { id: 1 } } });
  elements(tree, node => node.props?.['data-action'] === 'ping-visible')[0].props.onClick();
  assert.equal(started, 3);
  assert.match(messages.at(-1), /本番中に1件.*負荷/);
});

test('グループ選択はヘッダーの1か所に置き、機器一覧と書出に同じ選択を渡す', () => {
  const { Devices } = require('../src/views/Devices.tsx');
  const header = render(AppHeader, { profiles: [], groupSelection: 'group:Audio', groupNames: ['Audio', 'Video', ''], locked: false,
    status: { profile: { id: 1 } } });
  const devices = render(Devices, { group: 'Audio', replay: null });
  assert.equal(((header + devices).match(/<select aria-label="グループ"/g) || []).length, 1);
  assert.match(header, /グループ: <select[^]*<option value="group:Audio" selected="">Audio/);
  assert.match(header, /<option value="group:">未所属/);
  assert.doesNotMatch(devices, /<select/);
  assert.equal((devices.match(/group=Audio/g) || []).length, 2);
  assert.match(render(Devices, { group: '', replay: null }), /group=&amp;lang=ja/);
  assert.match(header, /編集可（ロックする）/);
  assert.match(render(AppHeader, { locked: true, profiles: [] }), /🔒 ロック中/);
  assert.match(header, /<form class="header-marker"><input[^]*aria-label="マーカー"[^]*<button type="submit">記録<\/button>/);
  const { OverviewView } = sourceWith('../src/views/Overview.tsx', {
    '../useRackData': { useRackData: () => ({ data: {}, definitions: {}, refresh: noop }) },
  });
  const alerts = [fault(1, { switch_id: 1, active: true, title: 'Audio fault' }),
    fault(2, { switch_id: 2, active: true, title: 'Video fault' })];
  const monitor = render(OverviewView, { group: 'Audio', replay: null, locked: false, operations: { alerts, ready: true, ack: noop, ackMany: noop }, overview: { switches: [
    { id: 1, name: 'Audio SW', group_tag: 'Audio' }, { id: 2, name: 'Video SW', group_tag: 'Video' },
  ] } });
  assert.match(monitor, /Audio SW/);
  assert.match(monitor, /Audio fault/);
  assert.doesNotMatch(monitor, /Video SW|Video fault|<select/);
  assert.match(monitor, /operation-area[^]*?data-action="ack-all"/);
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
    fault(1, { switch_id: 1, if_index: 1, active: true, acked: true }),
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
    updateTopology(cy, {
      nodes: [{ id: 'sw', switch_id: 1, label: 'West', kind: 'switch', reachable: true },
        { id: 'ext', label: 'Other', kind: 'external', reachable: true }],
      edges: [{ id: 'link', source: 'sw', target: 'ext', local_port: '1/0/2' }],
    }, undefined, '', true, false, alerts);
    assert.equal(cy.getElementById('sw').data('statusLabel'), '! West');
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
  assert.match(html, /status-warning[^>]*>!</);
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
  assert.match(html, /発生 <time[^>]*>18:29:32<\/time> → 復旧 <time[^>]*>18:30:10 JST<\/time>/);
});

test('External topology nodes use all linked switch ports and select the highest-priority edge', () => {
  const cy = require('cytoscape')({ headless: true, styleEnabled: true });
  const { updateTopology } = require('../src/topologyGraph.ts');
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
  const active = fault(3, { switch_id: 3, if_index: 3, port: '1/0/3', active: true, acked: true });
  const check = (alerts, tone, id) => {
    updateTopology(cy, topology, undefined, '', true, false, alerts);
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

test('ログはPC受信時刻だけを時刻列にし、展開時にSyslog原文を表示する', () => {
  const raw = '<13> Nov  7 15:16:44 SwitchWest LLDP: neighbor keepalive';
  assert.equal(syslogMessage(raw), 'LLDP: neighbor keepalive');
  assert.equal(syslogMessage('<34>1 2026-11-07T15:16:44Z SwitchWest service 123 ID - data'), 'service 123 ID - data');
  assert.equal(syslogMessage('LLDP: neighbor keepalive'), 'LLDP: neighbor keepalive');
  const event = { id: 9, ts: '2026-11-07T15:16:45Z', title: raw, code: 'SWITCH_LOG', category: 'SYSLOG', severity: 'INFO' };
  const html = render(LogEventRow, { event, ackable: new Set([9]) });
  assert.equal((html.match(/<time /g) || []).length, 1);
  assert.match(html, /2026-11-08 00:16:45 JST/);
  assert.match(html, /<summary>LLDP: neighbor keepalive<\/summary>/);
  assert.match(html, /<pre>&lt;13&gt; Nov  7 15:16:44 SwitchWest LLDP: neighbor keepalive<\/pre>/);
  assert.doesNotMatch(html, /data-action="ack"/);
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
  assert.equal((html.match(/<select/g) || []).length, 3);
  assert.match(html, /<option value="7">West<\/option>/);
  assert.match(html, /異常・警告のみ[^]*未確認のみ[^]*マーカー/);
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
    assert.match(draw([]), /class="whitespace-nowrap">—<\/td>/);
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
  assert.match(html, /復旧済み・未確認/);
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
  const alert = fault(1, { active: true, ts: new Date().toISOString() });
  const active = render(AlertBand, { alerts: [alert], wall: true, busy: true, ackMany: noop, undoCount: 1, undo: noop });
  assert.match(active, /wall-alert-group alert-active/);
  assert.match(active, /data-action="ack" disabled/);
  assert.match(active, /data-action="undo" disabled/);
  const locked = render(AlertBand, { alerts: [alert], wall: true, locked: true, ackMany: noop, undoCount: 1, undo: noop });
  assert.doesNotMatch(locked, /<button/);
  assert.match(locked, /ロック中のため確認できません/);
  const recovered = render(AlertBand, { alerts: [{ ...alert, active: false, cleared_at: new Date().toISOString() }], wall: true });
  assert.match(recovered, /alert-recovered/);
  assert.match(recovered, /発生 .*→ 復旧 /);
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

test('起動時は統合ポートモニターで、専門画面は詳細の下へまとめる', () => {
  const App = sourceWith('../src/App.tsx', { './components/TerminalDrawer': { TerminalDrawer: () => null } }).default;
  const html = render(App);
  assert.match(html, /aria-current="page"[^]*?ポートモニター/);
  const nav = html.match(/<nav[^]*?<\/nav>/)[0];
  assert.match(nav, /詳細/);
  assert.doesNotMatch(nav, /俯瞰|ポート一覧|DHCP|マルチキャスト|ヘルス/);
  assert.match(html, /aria-label="表示方法"/);
  assert.match(html, />図<|>一覧</);
});

test('Port monitor clears navigation targets on a header group change and when port detail closes', () => {
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
  const header = tree => elements(tree, node => node.type === AppHeader)[0].props;
  const target = { switchId: 1, ifIndex: 2 };

  monitor(drawApp()).openPortMonitor(target);
  const beforeGroupChange = drawApp();
  assert.equal(monitor(beforeGroupChange).target, target);
  header(beforeGroupChange).setGroupSelection('view:stage');
  const afterGroupChange = drawApp();
  assert.equal(monitor(afterGroupChange).part, 'stage');
  assert.equal(monitor(afterGroupChange).target, undefined);

  monitor(afterGroupChange).openPortMonitor(target);
  let close;
  const { OverviewView } = sourceWith('../src/views/Overview.tsx', {
    '../components/PortDetail': { PortDetail: props => { close = props.close; return null; } },
    '../useRackData': { useRackData: () => ({ data: { ports: [
      { switch_id: 1, port: { name: '1/0/2', if_index: 2, oper: 'up' } },
    ] }, definitions: {}, refresh: noop }) },
  });
  const props = monitor(drawApp());
  assert.equal(props.target, target);
  render(OverviewView, { ...props, overview: { switches: [{ id: 1, name: 'Switch', group_tag: '', reachable: true }] } });
  close();
  assert.equal(monitor(drawApp()).target, undefined);
});

test('ヘッダーから音声・転送・言語を撤去し、設定には既定OFFの通知・CSV・表示順を置く', () => {
  const html = render(AppHeader, { online: true, wall: false, phase: 'show', profiles: [], part: '', status: { locked: false, profile: { id: 1 } }, run: noop });
  assert.doesNotMatch(html, /音あり|消音|表示言語|>書出<|type="file"/);
  const tools = render(SettingsTools, { theme: 'dark', notifications: false, groups: [{ name: 'Audio' }], run: noop });
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
