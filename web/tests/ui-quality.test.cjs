const fs = require('node:fs');
const assert = require('node:assert/strict');
const { test } = require('node:test');
const ts = require('typescript');
const React = require('react');
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

const { asArray, decimal, formatBps, formatUptime, resolveSwitchId, labSwitchIndex, alertCount } = require('../src/format.ts');
const { FrontPanel, portHasAlert, poeHasFault } = require('../src/components/FrontPanel.tsx');
const { Table } = require('../src/components/Table.tsx');
const { SettingsDialog } = require('../src/components/SettingsDialog.tsx');
const { PortHardware } = require('../src/components/PortHardware.tsx');
const { ErrorBoundary } = require('../src/components/ErrorBoundary.tsx');
const { OverviewView } = require('../src/views/Overview.tsx');
const { decodeDeviceCsv, deviceUrl } = require('../src/deviceCsv.ts');
const { DeviceInventoryTable, Devices } = require('../src/views/Devices.tsx');
const { ScrubBar } = require('../src/components/ScrubBar.tsx');
const render = (component, props) => renderToStaticMarkup(React.createElement(component, props));
const port = (changes = {}) => ({
  name: '1/0/1', if_index: 1, type: 'rj45', oper: 'up', excluded: false,
  err_delta: 0, discard_delta: 0, fcs_delta: 0, cos_drop_delta: 0, err_since_reset: 0,
  ...changes,
});

test('機器名CSVの文字コードはBOM付きUTF-8、UTF-8、Shift_JISを判別する', () => {
  const bytes = new TextEncoder().encode('MAC,名称\r\n');
  assert.deepEqual(decodeDeviceCsv(Uint8Array.of(0xef, 0xbb, 0xbf, ...bytes)), {
    csv: 'MAC,名称\r\n', encoding: 'UTF-8 (BOM付き)',
  });
  assert.deepEqual(decodeDeviceCsv(bytes), { csv: 'MAC,名称\r\n', encoding: 'UTF-8' });
  // Shift_JISの「あ」は82 A0で、厳密なUTF-8では不正な並び。
  assert.deepEqual(decodeDeviceCsv(Uint8Array.of(0x82, 0xa0, 0x2c, 0x31)), {
    csv: 'あ,1', encoding: 'Shift_JIS',
  });
  assert.deepEqual(decodeDeviceCsv(new Uint8Array()), { csv: '', encoding: 'UTF-8' });
});

test('接続機器の一覧と書出URLにscope・検索・VLAN・再生時刻を渡す', () => {
  const filters = { scope: 'unnamed', q: ' 音響 & AV+ ', vlans: [10, 20] };
  assert.equal(deviceUrl({ scope: 'all', q: ' ', vlans: [] }), '/api/devices?scope=all');
  const live = new URL(deviceUrl(filters, undefined, true), 'http://test.invalid');
  assert.equal(live.pathname, '/api/devices/export.csv');
  assert.deepEqual(Object.fromEntries(live.searchParams), { scope: 'unnamed', q: '音響 & AV+', vlans: '10,20' });
  const replay = { id: 'id/1', ts: '2026-01-01T00:00:00+09:00' };
  const recording = new URL(deviceUrl({ ...filters, scope: 'all' }, replay), 'http://test.invalid');
  assert.equal(recording.pathname, '/api/replay/id%2F1/devices');
  assert.deepEqual(Object.fromEntries(recording.searchParams), {
    ts: replay.ts, scope: 'all', q: '音響 & AV+', vlans: '10,20',
  });
  assert.match(deviceUrl(filters, replay, true), /^\/api\/replay\/id%2F1\/devices\/export\.csv\?/);
});

test('接続機器一覧に未検出・経路上・記録なしを示す', () => {
  const html = render(DeviceInventoryTable, { rows: [
    { mac: '00:00:00:00:00:01', label: '', ip: '192.0.2.5', ip_stale: true, seen: false, last_seen: '0001-01-01T00:00:00Z' },
    { mac: '00:00:00:00:00:02', label: '卓', seen: true, source: 'lldp', via_uplink: true, poe_w: 0 },
  ] });
  assert.match(html, /（名前なし）/);
  assert.match(html, /○ 未検出/);
  assert.match(html, /◷ 過去のIP/);
  assert.match(html, /記録なし/);
  assert.match(html, /スイッチ間リンク.*↪ 経路上/);
  assert.match(html, />0\.0</);
});

test('再生中の接続機器は再生時刻と担当VLAN付きの書出を表示し読込を無効にする', () => {
  const html = render(Devices, {
    replay: [], replayRef: { id: 'replay-id', ts: '2026-01-01T00:00:00Z' },
    part: 'audio', settings: { views: [{ id: 'audio', vlans: [10, 20] }] }, locked: true, busy: false,
  });
  const links = [...html.matchAll(/href="([^"]+)" download=""/g)].map(match => match[1].replaceAll('&amp;', '&'));
  assert.equal(links.length, 2);
  assert.deepEqual(links.map(link => new URL(link, 'http://test.invalid').searchParams.get('scope')), ['all', 'unnamed']);
  for (const link of links) {
    const parsed = new URL(link, 'http://test.invalid');
    assert.equal(parsed.pathname, '/api/replay/replay-id/devices/export.csv');
    assert.equal(parsed.searchParams.get('ts'), '2026-01-01T00:00:00Z');
    assert.equal(parsed.searchParams.get('vlans'), '10,20');
  }
  assert.match(html, /disabled="" title="オフライン再生中はCSVを読み込めません。"/);
});

test('再生バーは表示中の時刻と移動先を別々に保ち、取得中は移動を無効にする', () => {
  const ts = '2026-01-01T00:01:00Z';
  const requestedTs = '2026-01-01T00:01:02Z';
  const html = render(ScrubBar, {
    sid: 'replay-id', ts, requestedTs, busy: true,
    timeline: { from: '2026-01-01T00:00:00Z', to: '2026-01-01T00:04:00Z', markers: [{ ts, title: 'マーカー' }] },
  });
  assert.match(html, new RegExp(`<input[^>]*type="range"[^>]*disabled=""[^>]*value="${Date.parse(requestedTs)}"`));
  assert.match(html, /<button[^>]*title="マーカー"[^>]*disabled=""/);
  assert.ok(html.includes(new Date(ts).toLocaleString('ja-JP', { hour12: false })));
});

test('配列以外のAPI応答とnull要素を安全に扱う', () => {
  for (const value of [null, undefined, {}, { length: 2 }, 'ports', 1]) assert.deepEqual(asArray(value), []);
  assert.deepEqual(asArray([1, null, 2, undefined]), [1, 2]);
  assert.match(render(Table, { rows: { length: 2 } }), /データなし/);
  assert.doesNotThrow(() => render(FrontPanel, { ports: {}, selected: 0, onSelect() {} }));
});

test('bpsはSI単位で小数1桁に変換する', () => {
  for (const [input, expected] of [[0, '0.0 bps'], [999, '999.0 bps'], [1000, '1.0 kbps'],
    [1250000, '1.3 Mbps'], [2500000000, '2.5 Gbps']]) assert.equal(formatBps(input), expected);
  assert.equal(formatBps(NaN), '—');
  assert.equal(formatBps(Infinity), '—');
  assert.equal(decimal(-10.126, 2), '-10.13');
});

test('稼働時間を日・時間・分へ変換する', () => {
  assert.equal(formatUptime(0), '0日 0時間 0分');
  assert.equal(formatUptime(90060), '1日 1時間 1分');
  assert.equal(formatUptime(-1), '—');
});

test('未選択は先頭へ、指定済みスイッチは保持する', () => {
  const switches = [{ id: 7 }, { id: 3 }];
  assert.equal(resolveSwitchId(switches, 0), 7);
  assert.equal(resolveSwitchId(switches, 3), 3);
  assert.equal(resolveSwitchId(switches, 99), 7);
  assert.equal(resolveSwitchId([], 0), 0);
});

test('ラボの番号はDBのIDや並び順に依存しない', () => {
  assert.equal(labSwitchIndex('AV-SW-01'), 0);
  assert.equal(labSwitchIndex('AV-SW-20'), 19);
  assert.equal(labSwitchIndex('AV-SW-00'), null);
  assert.equal(labSwitchIndex('未命名'), null);
});

test('PoEは実際の給電だけを表示し障害を区別する', () => {
  const panel = value => render(FrontPanel, { ports: [port(value)], selected: 1, onSelect() {} });
  assert.doesNotMatch(panel({ poe: { power_w: 0, fault: 'none' } }), /⚡|⛔/);
  assert.match(panel({ poe: { power_w: 1.5, fault: 'none' } }), /⚡/);
  assert.match(panel({ poe: { power_w: 0, fault: 'overload' } }), /⛔ PoE障害/);
  assert.equal(poeHasFault(port({ poe: { fault: ' NONE ' } })), false);
  assert.equal(poeHasFault(port({ poe: { fault: '' } })), false);
});

test('全指定カウンターの増加を異常マーカーにする', () => {
  assert.equal(portHasAlert(port()), false);
  for (const key of ['err_delta', 'discard_delta', 'fcs_delta', 'cos_drop_delta', 'err_since_reset']) {
    const value = port({ [key]: 1 });
    assert.equal(portHasAlert(value), true);
    assert.match(render(FrontPanel, { ports: [value], selected: 1, onSelect() {} }), /⚠ 異常/);
  }
});

test('ポート詳細は日本語の項目と指定の単位桁数で表示する', () => {
  const html = render(PortHardware, { port: port({
    poe: { power_w: 1.24, detect: 'delivering' },
    sfp: { rx_dbm: -3.126, tx_dbm: 1.2, temp_c: 40.2, volts: 3.3, bias_ma: 5.2 },
    cable: { status: 'normal', min_len_m: 2, max_len_m: 4, fail_loc_m: 0 },
  }) });
  assert.match(html, /電力/);
  assert.match(html, /1.2 W/);
  assert.match(html, /受信光/);
  assert.match(html, /-3.13 dBm/);
  assert.match(html, /1.20 dBm/);
  assert.match(html, /ケーブル診断/);
  assert.doesNotMatch(html, /<pre/);
});

test('監視表は日本語見出しと文字付き重要度を表示する', () => {
  const html = render(Table, { rows: [{ switch_name: 'AV-SW-01', severity: 'CRITICAL', title: '障害' }] });
  assert.match(html, />スイッチ</);
  assert.match(html, />重要度</);
  assert.match(html, /⛔ 重大/);
  assert.doesNotMatch(html, />switch_name</);
});

test('設定編集はJSONではなく数値・チェック・パートのフォームを使う', () => {
  const html = render(SettingsDialog, { value: {
    thresholds: { cpu_pct: 80 }, alert_sound: true,
    views: [{ id: 'audio', name: '音響', vlans: [10], tags: ['audio'], categories: [] }],
  }, close() {}, run() {}, busy: false });
  assert.match(html, /type="number"/);
  assert.match(html, /type="checkbox"/);
  assert.match(html, /VLAN（カンマ区切り）/);
  assert.match(html, /パートを追加/);
  assert.doesNotMatch(html, /textarea/);
});

test('エラー境界は原因と対処、再読み込みを表示する', () => {
  const boundary = new ErrorBoundary({ name: 'ポートモニター', children: null });
  boundary.state = { error: new Error('fixture') };
  const html = renderToStaticMarkup(boundary.render());
  assert.match(html, /ポートモニターを表示できませんでした/);
  assert.match(html, /原因:/);
  assert.match(html, /対処:/);
  assert.match(html, /再読み込み/);
});

test('実バックエンドとAPI文書の警告件数キーを両方受け入れる', () => {
  assert.equal(alertCount({ CRITICAL: 3 }, 'critical'), 3);
  assert.equal(alertCount({ warning: 2 }, 'warning'), 2);
  assert.equal(alertCount(null, 'warning'), 0);
});

test('俯瞰は非配列のスイッチ・イベント応答でも描画を継続する', () => {
  assert.doesNotThrow(() => render(OverviewView, {
    overview: { switches: {}, recent_events: {} }, settings: { views: {} },
    replay: null, part: '', locked: false, busy: false,
  }));
});
const { mergeOverviewTick, pushRecentEvent, logRows } = require('../src/events.ts');
const { createPoller } = require('../src/polling.ts');
const blankFilter = { category: '', severity: '', switch_id: '', q: '', from: '', to: '' };
const event = (id, changes = {}) => ({
  id, ts: new Date(id * 1000).toISOString(), switch_id: 1, if_index: 1,
  category: 'ANOMALY', severity: 'INFO', title: `イベント${id}`, acked: false, ...changes,
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
const { frontPanelRows } = require('../src/frontPanelLayout.ts');

test('トポロジーは真偽値セレクターでBlockingと経路候補を描画する', context => {
  const core = cytoscape({ headless: true, styleEnabled: true, elements: [], style: topologyStyles });
  context.after(() => core.destroy());
  const topology = {
    nodes: [{ id: 'a', label: 'A', kind: 'switch', switch_id: 1 }, { id: 'b', label: 'B', kind: 'switch', switch_id: 2 }],
    edges: [{ id: 'ab', source: 'a', target: 'b', local_port: '1/0/1', remote_port: '1/0/2', blocking: true }],
  };
  const multicast = { groups: [
    { switch_id: 1, group: '239.1.1.1', vlan: 10, ports: ['1/0/1'] },
    { switch_id: 2, group: '239.1.1.1', vlan: 10, ports: ['1/0/2'] },
  ] };
  updateTopology(core, topology, multicast, '239.1.1.1', true, false);
  assert.equal(core.edges('[?blocking]').length, 1);
  assert.equal(core.edges('[?highlight]').length, 1);
  assert.equal(core.getElementById('ab').style('line-style'), 'dashed');
  assert.equal(core.getElementById('ab').style('width'), '5px');
  updateTopology(core, topology, multicast, '', false, false);
  assert.equal(core.edges('[?highlight]').length, 0);
  assert.equal(core.getElementById('ab').style('display'), 'none');
});

test('トポロジー再取得・表示切替はドラッグ位置とズームを維持する', context => {
  const core = cytoscape({ headless: true, elements: [] });
  context.after(() => core.destroy());
  let layouts = 0;
  const original = core.layout.bind(core);
  core.layout = options => { layouts++; return original(options); };
  const topology = { nodes: [{ id: 'a', label: '旧名', kind: 'switch', x: 10, y: 20 }], edges: [] };
  updateTopology(core, topology, undefined, '', true, true);
  core.getElementById('a').position({ x: 321, y: 654 });
  core.zoom(1.5);
  core.pan({ x: 25, y: 30 });
  updateTopology(core, { ...topology, nodes: [{ ...topology.nodes[0], label: '新名', x: 0, y: 0 }] }, undefined, '', false, false);
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
