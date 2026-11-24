const fs = require('node:fs');
const path = require('node:path');
const assert = require('node:assert/strict');
const { test } = require('node:test');
const React = require('react');
const { renderToStaticMarkup } = require('react-dom/server');
const { t, setLang, eventTitle } = require('../src/i18n/index.ts');
const { AlertBand } = require('../src/components/AlertBand.tsx');
const { AppHeader } = require('../src/components/AppHeader.tsx');
const { PhaseControls, changePhase } = require('../src/components/PhaseControls.tsx');
const { GroupActions, resetGroup } = require('../src/components/GroupActions.tsx');
const { PortLamp, RackRow } = require('../src/components/RackRow.tsx');
const { DeviceInventoryTable } = require('../src/views/Devices.tsx');
const { SettingsDialog } = require('../src/components/SettingsDialog.tsx');
const { confirmAck } = require('../src/useOperations.ts');
const { deviceUrl, pingTargets } = require('../src/deviceCsv.ts');
const { pcTimestamp } = require('../src/pcTime.ts');
const { api } = require('../src/api.ts');
const { acknowledgeEvents } = require('../src/acknowledge.ts');

const render = (component, props) => renderToStaticMarkup(React.createElement(component, props));
const noop = () => {};
const alert = (changes = {}) => ({
  event_id: 9, switch_id: 4, switch_name: 'Switch A', port: '1/0/1', device_name: 'Mixer', if_index: 1,
  code: 'LINK_DOWN', severity: 'CRITICAL', ts: new Date().toISOString(), active: false, acked: false,
  cleared_at: new Date().toISOString(), state: 'latched', suppressed: false, ...changes,
});
const controls = { phase: 'setup', locked: false, busy: false, wall: false, run: noop, onPhase: noop };
const group = { group: 'Audio', locked: false, busy: false, wall: false, devices: [], run: noop, refresh: noop, notice: noop, collapse: noop, collapseLabel: 'Collapse', onEdit: noop, replay: false };

test('i18n外のTypeScriptに日本語の直書きがない', () => {
  const root = path.join(__dirname, '../src');
  const found = [];
  function walk(dir) {
    for (const entry of fs.readdirSync(dir, { withFileTypes: true })) {
      const file = path.join(dir, entry.name);
      if (entry.isDirectory() && file !== path.join(root, 'i18n')) walk(file);
      if (entry.isFile() && /\.tsx?$/.test(file) && /[\p{Script=Han}\p{Script=Hiragana}\p{Script=Katakana}]/u.test(fs.readFileSync(file, 'utf8'))) {
        found.push(path.relative(root, file));
      }
    }
  }
  walk(root);
  assert.deepEqual(found, []);
});

test('英語のイベントはコードと引数で表示し、未知のコードは保存タイトルを使う', context => {
  context.after(() => setLang('ja'));
  setLang('en');
  assert.equal(t('rack.phase'), 'Operating phase');
  assert.equal(eventTitle({ code: 'DEVICE_MISSING', detail: { name: 'Mixer', mac: 'AA' } }), 'Registered device is missing: Mixer');
  assert.equal(eventTitle({ code: 'LEGACY', title: 'Former title' }), 'Former title');
  const html = render(DeviceInventoryTable, { rows: [{ mac: 'AA', seen: true, planned_ip: '192.0.2.5', ip_mismatch: true, ping_ok: false }] });
  assert.match(html, /Planned IP/);
  assert.match(html, /192\.0\.2\.5/);
  assert.match(html, /▲ Planned IP mismatch/);
  assert.match(html, /No response/i);
  const url = new URL(deviceUrl({ scope: 'all', q: '', vlans: [], group: '' }, undefined, true), 'http://test.invalid');
  assert.equal(url.searchParams.get('lang'), 'en');
  assert.equal(url.searchParams.get('group'), '');
  assert.deepEqual(pingTargets([{ ip: '', planned_ip: '192.0.2.5' }]), ['192.0.2.5']);
});

test('APIは選択言語とリクエスト固有のヘッダーを送る', async context => {
  context.after(() => setLang('ja'));
  context.mock.method(globalThis, 'fetch', async (_url, options) => {
    assert.equal(options.headers.get('Accept-Language'), 'en');
    assert.equal(options.headers.get('Content-Type'), 'application/json');
    return new Response('{}');
  });
  const original = globalThis.window;
  context.after(() => { globalThis.window = original; });
  globalThis.window = { setTimeout, dispatchEvent: noop };
  setLang('en');
  await api('/api/test', { headers: { 'Content-Type': 'application/json' } });
});

test('ラックのランプは発生中を赤、復旧未確認を枠付きの橙、未観測を灰にする', () => {
  const input = { port: { if_index: 1, name: '1/0/1', oper: 'up', devices: [{ mac: 'AA' }] }, devices: [], alerts: [], reachable: true, wall: false, onSelect: noop };
  const css = fs.readFileSync(path.join(__dirname, '../src/style.css'), 'utf8');
  for (const [alerts, expected, symbol, color] of [
    [[alert({ severity: 'WARNING', active: true, acked: true })], 'critical', '!', '#7f1d1d'],
    [[alert()], 'warning', '▲', '#713f12'],
    [[alert({ acked: true })], 'healthy', '✓', '#154434'],
  ]) {
    const html = render(PortLamp, { ...input, alerts });
    assert.match(html, new RegExp(`lamp-${expected}`));
    assert.match(html, new RegExp(`class="lamp-symbol" aria-hidden="true">${symbol}</span>`));
    assert.match(css, new RegExp(`\\.lamp-${expected}[^}]+background: ${color}`));
  }
  assert.match(css, /\.lamp-warning[^}]+box-shadow: inset/);
  assert.match(render(PortLamp, { ...input, port: { ...input.port, oper: 'down', sfp: { los: true } } }), /lamp-unused/);
  assert.match(render(PortLamp, { ...input, reachable: false }), /lamp-unknown/);
  assert.match(render(PortLamp, { ...input, alerts: [alert({ active: true, suppressed: true })] }), /lamp-healthy/);
});

test('壁表示にはラベル付き終了とアラート確認を置き、ラック操作は隠す', () => {
  const header = render(AppHeader, { ...controls, wall: true, setWall: noop });
  assert.equal((header.match(/<button\b/g) || []).length, 1);
  assert.match(header, /wall-exit/);
  assert.match(header, /phase-setup/);
  assert.match(header, /PC時刻/);
  // グループの絞り込みは各画面へ移した。壁表示のヘッダーには選択欄を置かない
  assert.equal((header.match(/<select\b/g) || []).length, 0);
  assert.equal(render(GroupActions, { ...group, wall: true }), '');
  const band = render(AlertBand, { alerts: [alert()], wall: true, ack: noop });
  assert.match(band, /Switch A.*Mixer/);
  assert.match(band, /data-action="ack"/);
  assert.match(band, /data-action="ack-all"/);
  assert.match(header, /通常表示に戻る/);
});

test('登録済み機器のリンク断は赤、筐体番号が一致してもファン故障では正常ポートを赤にしない', () => {
  const input = { port: { if_index: 1, name: '1/0/1', oper: 'up', devices: [{ mac: 'AA' }] }, devices: [], reachable: true, wall: false, onSelect: noop };
  const down = alert({ severity: 'WARNING', active: true });
  assert.match(render(PortLamp, { ...input, alerts: [down], devices: [{ if_index: 1, registered: true }] }), /lamp-critical/);
  assert.match(render(PortLamp, { ...input, alerts: [down], devices: [{ if_index: 1, registered: false, label: 'Console' }] }), /lamp-critical/);
  assert.match(render(PortLamp, { ...input, alerts: [alert({ code: 'FAN_FAIL', if_index: 1 })] }), /lamp-healthy/);
});

test('CPU・温度・PoEの無効なしきい値では数値を異常扱いしない', () => {
  const props = {
    summary: { id: 1, name: 'SW', reachable: true, cpu_5s: 85, temp_max_c: 67, temp_state: 'normal', poe_budget_w: 100, poe_used_w: 95, psu_ok: true, fans_ok: true },
    ports: [], devices: [], alerts: [], definitions: {}, wall: true,
    actions: { settings: { thresholds: { cpu_pct: 0, temp_c: 0, poe_pct: 0 } } },
  };
  assert.doesNotMatch(render(RackRow, props), /health-abnormal/);
  assert.match(render(RackRow, { ...props, summary: { ...props.summary, temp_state: 'critical' } }), /health-abnormal/);
});

test('閲覧専用ロックではフェーズ変更と確認を隠し、Pingとリセットを無効にする', () => {
  assert.doesNotMatch(render(PhaseControls, { ...controls, locked: true }), /<select/);
  const html = render(GroupActions, { ...group, locked: true });
  assert.equal((html.match(/disabled=""/g) || []).length, 2);
  assert.doesNotMatch(html, /data-action="work"|group-menu|export\.csv/);
  const band = render(AlertBand, { alerts: [alert()], ack: noop, locked: true });
  assert.doesNotMatch(band, /data-action="ack"|data-action="ack-all"/);
});

test('旧データの * グループでは全台へのPingとリセットを無効にする', () => {
  const legacy = render(GroupActions, { ...group, group: '*' });
  assert.equal((legacy.match(/disabled=""/g) || []).length, 2);
  assert.doesNotMatch(legacy, /export\.csv/);
  assert.match(render(GroupActions, { ...group, group: '*', all: true }), /data-action="reset"/);
});

test('一括確認は渡されたイベントIDだけを個別確認し、途中で失敗しても再取得する', async context => {
  const original = globalThis.window;
  context.after(() => { globalThis.window = original; });
  globalThis.window = { setTimeout, dispatchEvent: noop };
  const requested = [];
  context.mock.method(globalThis, 'fetch', async (url, options) => {
    requested.push(url);
    assert.equal(options.method, 'POST');
    return new Response(null, { status: url.includes('/2/') ? 503 : 204 });
  });
  let refreshed = 0;
  const confirmed = [];
  await assert.rejects(acknowledgeEvents([1, 2, 1, 3, 4, 5], async () => { refreshed++; }, ids => confirmed.push(...ids)), /503/);
  assert.deepEqual(requested, [1, 2, 3, 4].map(id => `/api/events/${id}/ack`));
  assert.deepEqual(confirmed, [1, 3, 4]);
  assert.equal(refreshed, 1);
});

test('フェーズ変更・リセット・一括確認はキャンセル時に処理を始めない', context => {
  const original = globalThis.window;
  context.after(() => { globalThis.window = original; });
  let confirms = [];
  let actions = 0;
  let allow = false;
  globalThis.window = { confirm: message => { confirms.push(message); return allow; } };
  const run = () => { actions++; };
  const invoke = () => {
    changePhase('setup', run, noop);
    resetGroup('Audio', 'Audio', run, noop);
    confirmAck(undefined, run, async () => {});
  };
  invoke();
  assert.equal(confirms.length, 3);
  assert.match(confirms[0], /Ping.*抑止.*3倍/);
  assert.match(confirms[1], /Audio.*リセット/);
  assert.match(confirms[2], /すべて確認/);
  assert.equal(actions, 0);
  confirms = [];
  allow = true;
  invoke();
  assert.equal(actions, 3);
  assert.equal(confirms.length, 3);
});

test('PC時刻は秒とタイムゾーンを表示し、今日のアラートは日付を省略する', () => {
  const time = Date.parse('2020-09-15T05:03:08Z');
  assert.equal(pcTimestamp(time, true, time), '2020-09-15 14:03:08 JST');
  assert.equal(pcTimestamp(time, false, time), '14:03:08 JST');
  assert.equal(pcTimestamp(time, false, Date.parse('2020-09-15T15:00:00Z')), '2020-09-15 14:03:08 JST');
  const settings = render(SettingsDialog, { value: { thresholds: {}, views: [] }, webAddr: '127.0.0.1:8080', close: noop, run: noop, busy: false });
  assert.match(settings, /他のPCからは接続できません/);
});

