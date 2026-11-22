const assert = require('node:assert/strict');
const { test } = require('node:test');
const React = require('react');
const { renderToStaticMarkup } = require('react-dom/server');

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
  assert.deepEqual(Object.fromEntries(live.searchParams), { scope: 'unnamed', q: '音響 & AV+', vlans: '10,20', lang: 'ja' });
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

