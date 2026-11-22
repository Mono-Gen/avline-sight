const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');
const { createRequire } = require('node:module');
const { test } = require('node:test');
const ts = require('typescript');
const React = require('react');
const { renderToStaticMarkup } = require('react-dom/server');
const { setLang } = require('../src/i18n/index.ts');
const { AlertBand } = require('../src/components/AlertBand.tsx');
const { PortDetail } = require('../src/components/PortDetail.tsx');
const { PhaseControls } = require('../src/components/PhaseControls.tsx');
const { wallAlertCount } = require('../src/wallAlerts.ts');

const render = (component, props) => renderToStaticMarkup(React.createElement(component, props));
const alert = (id, changes = {}) => ({
  event_id: id, switch_id: 1, switch_name: 'SW', if_index: 1, port: '1/0/1',
  severity: 'CRITICAL', code: 'POE_FAULT', title: `Fault ${id}`, ts: new Date(id * 1000).toISOString(),
  active: false, acked: false, state: 'latched', cleared_at: null, suppressed: false, ...changes,
});

// フックとブラウザー境界だけ差し替え、実際のコンポーネントと購読コールバックを実行する。
function loadSource(relative, overrides, globals = {}) {
  const filename = path.join(__dirname, relative);
  const realRequire = createRequire(filename);
  const compiled = ts.transpileModule(fs.readFileSync(filename, 'utf8'), {
    compilerOptions: { module: ts.ModuleKind.CommonJS, jsx: ts.JsxEmit.ReactJSX, target: ts.ScriptTarget.ES2020 },
    fileName: filename,
  });
  const module = { exports: {} };
  vm.runInNewContext(compiled.outputText, {
    module, exports: module.exports,
    require: name => Object.hasOwn(overrides, name) ? overrides[name] : realRequire(name),
    ...globals,
  }, { filename });
  return module.exports;
}

test('重大イベントでも音声を作らず、履歴と接続状態を更新する', () => {
  let receive, connected, cleanup, socketClosed = 0, updated = 0;
  const values = [];
  const { useLiveFeed } = loadSource('../src/useLiveFeed.ts', {
    react: { useRef: value => ({ current: value }), useState: () => [undefined, value => values.push(value)], useEffect: callback => { cleanup = callback(); } },
    './ws': { LiveSocket: class { constructor(handler, status) { receive = handler; connected = status; } close() { socketClosed++; } } },
  }, { AudioContext: class { constructor() { assert.fail('AudioContext must never be used'); } } });
  useLiveFeed({ setOverview() { updated++; }, setEvents() { updated++; }, setState() {}, setOnline() {} });
  for (const severity of ['CRITICAL', 'WARNING']) {
    receive({ type: 'event_pushed', data: { severity, detail: {} } });
  }
  assert.equal(updated, 4);
  receive({ type: 'overview_tick', data: {} });
  assert.equal(typeof values[0], 'number');
  assert.equal(values[1], true);
  connected(false);
  assert.equal(values.at(-1), false);
  cleanup(); assert.equal(socketClosed, 1);
});

test('復旧時刻nullではアラート帯とポート詳細の復旧行を隠す', context => {
  context.after(() => setLang('ja'));
  for (const [lang, expected] of [['ja', '復旧: 不明'], ['en', 'Cleared: Unknown']]) {
    setLang(lang);
    const alerts = [alert(1)];
    assert.ok(!render(AlertBand, { alerts }).includes(expected));
    assert.ok(!render(PortDetail, { switchId: 1, switchName: 'SW', port: { if_index: 1, name: '1/0/1' },
      devices: [], alerts, close() {}, replay: true }).includes(expected));
  }
});

test('壁表示は収まる全件を表示し、超過時だけ集約行の高さも確保する', () => {
  assert.equal(wallAlertCount([], 0, 4, 20), 0);
  assert.equal(wallAlertCount([60, 60, 60], 188, 4, 20), 3);
  assert.equal(wallAlertCount([60, 60, 60], 187, 4, 20), 2);
  assert.equal(wallAlertCount([60, 60, 60], 147, 4, 20), 1);
  assert.equal(wallAlertCount([60], 59, 4, 20), 0);
  assert.equal(wallAlertCount([60, 140, 40], 190, 4, 20), 1);
});

test('壁表示は未確認だけを新着順に全件表示し、件数が多くても行を省略しない', () => {
  const alerts = Array.from({ length: 30 }, (_, index) => alert(index + 1, { if_index: index + 1, code: 'CUSTOM', title: `UniqueFault${index + 1}` }));
  alerts.push(alert(100, { acked: true, title: 'Acked fault', code: 'CUSTOM' }));
  alerts.push(alert(101, { suppressed: true, title: 'Suppressed fault', code: 'CUSTOM' }));
  const html = render(AlertBand, { alerts, wall: true });
  assert.equal((html.match(/<article/g) || []).length, 30);
  assert.ok(html.indexOf('UniqueFault30') < html.indexOf('UniqueFault1<'));
  assert.doesNotMatch(html, /Acked fault|Suppressed fault|wall-alert-overflow|<button/);
  assert.match(html, /未確認 30件/);
  assert.match(html, /wall-alerts-dense/);
});

test('通常ヘッダーの運用フェーズは色付きセレクター1つで表示する', () => {
  for (const phase of ['setup', 'rehearsal', 'show']) {
    const input = { phase, locked: false, busy: false, wall: false, run() {}, onPhase() {} };
    const html = render(PhaseControls, input);
    assert.equal((html.match(/<select\b/g) || []).length, 1);
    assert.match(html, new RegExp(`class="phase-selector phase-${phase}"`));
    assert.doesNotMatch(html, /phase-pill/);
    assert.match(render(PhaseControls, { ...input, busy: true }), /disabled=""/);
    for (const change of [{ wall: true }, { locked: true }]) {
      const display = render(PhaseControls, { ...input, ...change });
      assert.match(display, /phase-pill/);
      assert.doesNotMatch(display, /<select/);
    }
  }
});
