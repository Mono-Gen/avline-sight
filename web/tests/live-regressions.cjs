const assert = require('node:assert/strict');
const { test } = require('node:test');
const React = require('react');
const { renderToStaticMarkup } = require('react-dom/server');
const { FrontPanel } = require('../src/components/FrontPanel.tsx');
const { DeviceInventoryTable } = require('../src/views/Devices.tsx');
const render = (component, props) => renderToStaticMarkup(React.createElement(component, props));
const port = (changes = {}) => ({
  name: '1/0/1', if_index: 1, type: 'rj45', oper: 'up', excluded: false,
  err_delta: 0, discard_delta: 0, fcs_delta: 0, cos_drop_delta: 0, err_since_reset: 0,
  ...changes,
});

const { mergeOverviewTick, pushRecentEvent, logRows } = require('../src/events.ts');
const { createPoller } = require('../src/polling.ts');
const blankFilter = { category: '', severity: '', switch_id: '', q: '', from: '', to: '' };
const event = (id, changes = {}) => ({
  id, ts: new Date(id * 1000).toISOString(), switch_id: 1, if_index: 1,
  category: 'ANOMALY', severity: 'INFO', title: `イベント${id}`, acked: false, ...changes,
});

test('Server search acknowledgement overrides stale acknowledgement in live rows', () => {
  const [row] = logRows([event(1)], [event(1, { acked: true })], blankFilter);
  assert.equal(row.acked, false);
});

test('nullのoverview_tickでも直前のイベント履歴を維持する', () => {
  const previous = { switches: [], alerts: {}, recent_events: [event(2), event(1)] };
  const merged = mergeOverviewTick(previous, { switches: [{ id: 2 }], alerts: { critical: 1 }, recent_events: null });
  assert.deepEqual(merged.recent_events.map(item => item.id), [2, 1]);
  assert.equal(merged.alerts.critical, 1);
  assert.deepEqual(mergeOverviewTick(previous, { ...previous, recent_events: [] }).recent_events, []);
});

test('event_pushedを俯瞰へ追加し、重複を除いて30件に制限する', () => {
  let overview = { switches: [], alerts: {}, recent_events: Array.from({ length: 30 }, (_, index) => event(index + 1)) };
  overview = pushRecentEvent(overview, event(31));
  assert.equal(overview.recent_events.length, 30);
  assert.equal(overview.recent_events[0].id, 31);
  overview = pushRecentEvent(overview, event(31, { title: '更新済み' }));
  assert.equal(overview.recent_events.filter(item => item.id === 31).length, 1);
  assert.equal(overview.recent_events[0].title, '更新済み');
});

test('2000件の新着があっても過去の検索結果は捨てない', () => {
  const live = Array.from({ length: 2000 }, (_, index) => event(index + 10000));
  const rows = logRows([event(1)], live, blankFilter);
  assert.equal(rows.length, 2001);
  assert.equal(rows.at(-1).id, 1);
  assert.equal(live.length, 2000);
});

test('ライブは現在の検索条件で絞り込み、検索とライブを個別に制限する', () => {
  const history = Array.from({ length: 2100 }, (_, index) => event(index + 1, { severity: 'WARNING' }));
  const live = [event(5000, { severity: 'WARNING' }), event(5001, { severity: 'CRITICAL' })];
  const rows = logRows(history, live, { ...blankFilter, severity: 'WARNING' });
  assert.equal(rows.length, 2001);
  assert.equal(rows.some(item => item.id === 5001), false);
  assert.equal(rows[0].id, 5000);
});

test('定期更新は通信中に重複せず、停止後の応答・タイマーを無効にする', async context => {
  context.mock.timers.enable({ apis: ['setInterval', 'setTimeout'] });
  let calls = 0;
  let finish;
  let signal;
  const values = [];
  const polling = createPoller({
    intervalMs: 5000,
    load: currentSignal => { calls++; signal = currentSignal; return new Promise(resolve => { finish = resolve; }); },
    onData: value => values.push(value),
    onError: error => { throw error; },
  });
  const pending = polling.refresh();
  await Promise.resolve();
  context.mock.timers.tick(15000);
  assert.equal(calls, 1);
  polling.stop();
  assert.equal(signal.aborted, true);
  finish('遅延応答');
  await pending;
  context.mock.timers.tick(15000);
  assert.deepEqual(values, []);
  assert.equal(calls, 1);
});

test('定期更新は失敗後の次の周期で復帰できる', async context => {
  context.mock.timers.enable({ apis: ['setInterval', 'setTimeout'] });
  let calls = 0;
  const errors = [];
  const values = [];
  const polling = createPoller({
    intervalMs: 5000,
    load: async () => { if (++calls === 1) throw new Error('通信失敗'); return '復帰'; },
    onData: value => values.push(value),
    onError: error => errors.push(error.message),
  });
  await assert.rejects(polling.refresh(), /通信失敗/);
  context.mock.timers.tick(5000);
  await polling.refresh();
  assert.deepEqual(errors, ['通信失敗']);
  assert.deepEqual(values, ['復帰']);
  polling.stop();
});
const cytoscape = require('cytoscape');
const { updateTopology, topologyStyles } = require('../src/topologyGraph.ts');
const { buildTopology } = require('../src/topologyModel.ts');
const { frontPanelRows } = require('../src/frontPanelLayout.ts');

test('Topology hides blocked links and preserves multicast highlights on forwarding links', context => {
  const core = cytoscape({ headless: true, styleEnabled: true, elements: [], style: topologyStyles });
  context.after(() => core.destroy());
  const topology = {
    nodes: [{ id: 'a', label: 'A', kind: 'switch', switch_id: 1 }, { id: 'b', label: 'B', kind: 'switch', switch_id: 2 }],
    edges: [
      { id: 'ab', source: 'a', target: 'b', local_port: '1/0/1', remote_port: '1/0/2' },
      { id: 'blocked', source: 'a', target: 'b', local_port: '1/0/3', remote_port: '1/0/4', blocking: true },
    ],
  };
  const multicast = { groups: [
    { switch_id: 1, group: '239.1.1.1', vlan: 10, ports: ['1/0/1'] },
    { switch_id: 2, group: '239.1.1.1', vlan: 10, ports: ['1/0/2'] },
  ] };
  const model = buildTopology(topology, [], []);
  updateTopology(core, model, multicast, { group: '239.1.1.1' });
  assert.equal(core.getElementById('blocked').length, 0);
  assert.equal(core.edges('[?highlight]').length, 1);
  assert.equal(core.getElementById('ab').style('underlay-opacity'), '1');
  updateTopology(core, model, multicast);
  assert.equal(core.edges('[?highlight]').length, 0);
  assert.equal(core.getElementById('blocked').length, 0);
});

test('トポロジー再取得・表示切替はドラッグ位置とズームを維持する', context => {
  const core = cytoscape({ headless: true, elements: [] });
  context.after(() => core.destroy());
  let layouts = 0;
  const original = core.makeLayout.bind(core);
  core.makeLayout = options => { layouts++; return original(options); };
  const topology = { nodes: [{ id: 'a', label: '旧名', kind: 'switch', x: 10, y: 20 }], edges: [] };
  updateTopology(core, buildTopology(topology, [], []), undefined, { initial: true });
  core.getElementById('a').position({ x: 321, y: 654 });
  core.zoom(1.5);
  core.pan({ x: 25, y: 30 });
  updateTopology(core, buildTopology({ ...topology, nodes: [{ ...topology.nodes[0], label: '新名', x: 0, y: 0 }] }, [], []));
  assert.deepEqual(core.getElementById('a').position(), { x: 321, y: 654 });
  assert.equal(core.getElementById('a').data('label'), '新名');
  assert.equal(core.zoom(), 1.5);
  assert.deepEqual(core.pan(), { x: 25, y: 30 });
  assert.equal(layouts, 1);
});

test('定義済みモデルは行と隙間を守り、未定義ポートをその他へ配置する', () => {
  const ports = [port({ name: '1/0/1' }), port({ name: '1/0/2', if_index: 2 }), port({ name: 'lag 1', if_index: 100, type: 'lag' })];
  const definition = { model: 'fixture', rows: [['1/0/2', '', '1/0/9'], ['1/0/1']], note: '' };
  const rows = frontPanelRows(ports, definition);
  assert.deepEqual(rows[0].slots.map(item => item?.name || null), ['1/0/2', null, null]);
  assert.deepEqual(rows[1].slots.map(item => item.name), ['1/0/1']);
  assert.equal(rows[2].label, 'その他');
  assert.equal(rows[2].slots[0].name, 'lag 1');
  const html = render(FrontPanel, { ports, model: 'fixture', definitions: { fixture: definition }, selected: 1, onSelect() {} });
  assert.match(html, /その他/);
  assert.match(html, /aria-hidden="true"/);
});

test('未知モデルはrj45・sfp・lag順かつ奇数上段・偶数下段へ配置する', () => {
  const ports = [
    port({ name: 'lag 1', type: 'lag' }), port({ name: '1/0/10', type: 'sfp' }),
    port({ name: '1/0/9', type: 'sfp' }), port({ name: '1/0/2' }), port({ name: '1/0/1' }),
  ];
  const rows = frontPanelRows(ports);
  assert.deepEqual(rows[0].slots.map(item => item.name), ['1/0/1', '1/0/9', 'lag 1']);
  assert.deepEqual(rows[1].slots.map(item => item.name), ['1/0/2', '1/0/10']);
});
const { AvStatus } = require('../src/components/AvStatus.tsx');
const { api: requestApi } = require('../src/api.ts');

test('MVRとAVBは空なら折り畳み、失敗時は警告付きで詳細を開く', () => {
  const empty = { id: 1, name: 'AV-SW-01', igmp: { mvr: false, mvr_groups: [], mvr_rx_failures: 0, msrp_streams: 0, msrp_reservations: 0, msrp_failed: [] } };
  const collapsed = render(AvStatus, { state: empty });
  assert.doesNotMatch(collapsed, /<details[^>]*\bopen/);
  assert.match(collapsed, /データなし/);
  for (const change of [{ mvr_rx_failures: 3 }, { msrp_failed: [{ stream_id: 'stream-1', port: '1/0/1', declaration: 'failed', failure_code: 7 }] }]) {
    const expanded = render(AvStatus, { state: { ...empty, igmp: { ...empty.igmp, ...change } } });
    assert.match(expanded, /<details[^>]*\bopen/);
    assert.match(expanded, /⚠ 警告/);
  }
  const active = render(AvStatus, { state: { ...empty, igmp: { ...empty.igmp,
    mvr: true, mvr_groups: [{ group: '239.1.1.1', active: true, ports: ['1/0/1'] }],
    msrp_streams: 2, msrp_reservations: 3,
    gptp: { clock_id: 'clock-1', grandmaster: 'gm-1', steps_removed: 2 },
  } } });
  assert.match(active, /239.1.1.1/);
  assert.match(active, /MSRP予約数/);
  assert.match(active, /gm-1/);
});

test('同じイベントは最新の検索応答の確認状態を採用する', () => {
  const rows = logRows([event(1, { acked: true })], [event(1, { acked: false })], blankFilter);
  assert.equal(rows.length, 1);
  assert.equal(rows[0].acked, true);
});

test('JSON本文の読込中にも外部中断が届き、通常エラーとして通知しない', async context => {
  const previousWindow = global.window;
  const previousFetch = global.fetch;
  let notices = 0;
  global.window = { setTimeout, dispatchEvent() { notices++; } };
  let reading;
  const bodyStarted = new Promise(resolve => { reading = resolve; });
  global.fetch = async (_path, options) => ({
    ok: true, status: 200,
    text: () => new Promise((_resolve, reject) => {
      options.signal.addEventListener('abort', () => reject(new DOMException('aborted', 'AbortError')), { once: true });
      reading();
    }),
  });
  context.after(() => { global.window = previousWindow; global.fetch = previousFetch; });
  const controller = new AbortController();
  const pending = requestApi('/fixture', { signal: controller.signal });
  await bodyStarted;
  controller.abort();
  await assert.rejects(pending, { name: 'AbortError' });
  assert.equal(notices, 0);
});

test('一括Ping: 宛先は表示中の行のIPを重複なく集め、過去のIPも含める', () => {
  const { pingTargets } = require('../src/deviceCsv.ts');
  const rows = [
    { ip: '10.0.0.1' }, { ip: '' }, { ip: '10.0.0.1' }, { ip: '10.0.0.9', ip_stale: true },
  ];
  assert.deepEqual(pingTargets(rows), ['10.0.0.1', '10.0.0.9']);
});

test('一括Ping: 結果表示は記号と文字を併記し、無応答を故障と断定しない', () => {
  const { pingLabel } = require('../src/deviceCsv.ts');
  assert.equal(pingLabel('ok', 2.6), '✓ 応答 3ms');
  assert.equal(pingLabel('timeout', -1), '⚠ 無応答');
  assert.equal(pingLabel('unreachable', -1), '⚠ 到達不能');
  const html = renderToStaticMarkup(React.createElement(DeviceInventoryTable, { rows: [{
    mac: 'AA:BB:CC:DD:EE:FF', label: '卓', note: '', ip: '10.0.0.1', vendor: '', lldp_name: '', switch_name: 'SW', group_tag: '',
    port: '1/0/1', vlan: 10, speed_mbps: 1000, seen: true, source: 'fdb', via_uplink: false,
    ping: { status: 'timeout', rtt_ms: -1, at: '2026-01-02T03:04:05Z' },
  }] }));
  assert.match(html, /⚠ 無応答/);
  assert.doesNotMatch(html, /故障/);
});

test('一括Ping: 応答待ちは全宛先が無応答でも打ち切られない長さにする（監査指摘#6）', () => {
  const { bulkPingTimeoutMs, MAX_BULK_PING } = require('../src/deviceCsv.ts');
  // サーバーの最悪時間: ceil(256/16)=16巡 × 2回 × 1秒 = 32秒
  assert.ok(bulkPingTimeoutMs(MAX_BULK_PING) > 32000);
  assert.ok(bulkPingTimeoutMs(1) > 2000);
});


