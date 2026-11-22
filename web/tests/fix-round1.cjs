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

test('抑止された重大イベントは履歴を更新しても通知音を鳴らさない', () => {
  for (const preferences of [
    { suppressed: true, expected: 0 },
    { suppressed: false, expected: 1 },
    { suppressed: false, muted: true, expected: 0 },
    { suppressed: false, replay: true, expected: 0 },
    { suppressed: false, sound: false, expected: 0 },
    { suppressed: false, severity: 'WARNING', expected: 0 },
  ]) {
    let receive, cleanup, played = 0, closed = 0, socketClosed = 0, updated = 0;
    class FakeSocket {
      constructor(handler) { receive = handler; }
      close() { socketClosed++; }
    }
    class FakeAudio {
      currentTime = 0;
      createOscillator() {
        const oscillator = { connect() {}, start() { played++; }, stop() { oscillator.onended(); } };
        return oscillator;
      }
      close() { closed++; return Promise.resolve(); }
    }
    const { useLiveFeed } = loadSource('../src/useLiveFeed.ts', {
      react: { useRef: value => ({ current: value }), useEffect: callback => { cleanup = callback(); } },
      './ws': { LiveSocket: FakeSocket },
    }, { AudioContext: FakeAudio });
    useLiveFeed({ muted: !!preferences.muted, replay: !!preferences.replay, settings: { alert_sound: preferences.sound !== false },
      setOverview() { updated++; }, setEvents() { updated++; }, setState() {}, setOnline() {} });
    receive({ type: 'event_pushed', data: { severity: preferences.severity || 'CRITICAL', detail: { _suppressed: preferences.suppressed } } });
    assert.equal(played, preferences.expected, JSON.stringify(preferences));
    assert.equal(closed, played);
    assert.equal(updated, 2);
    cleanup();
    assert.equal(socketClosed, 1);
  }
});

test('復旧時刻nullはアラート帯とポート詳細で不明と表示する', context => {
  context.after(() => setLang('ja'));
  for (const [lang, expected] of [['ja', '復旧: 不明'], ['en', 'Cleared: Unknown']]) {
    setLang(lang);
    const alerts = [alert(1)];
    assert.ok(render(AlertBand, { alerts }).includes(expected));
    assert.ok(render(PortDetail, { switchId: 1, switchName: 'SW', port: { if_index: 1, name: '1/0/1' },
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

test('壁表示の実コンポーネントは重要度・新着順で全件か+n件を切り替え、内側をスクロールしない', () => {
  const refs = [];
  let refIndex = 0, count, effect, resize, disconnected = false;
  const hooks = {
    useRef: initial => refs[refIndex++] ||= { current: initial },
    useState: () => [count, next => { count = next; }],
    useEffect: callback => { effect = callback; },
  };
  const parent = { clientHeight: 500 };
  const heading = { offsetHeight: 30, style: { marginBottom: '8' } };
  const heights = [60, 60, 60].map(offsetHeight => ({ offsetHeight }));
  const list = { style: { rowGap: '4' }, querySelectorAll: () => heights };
  const band = { parentElement: parent, style: { paddingTop: '4', paddingBottom: '4', borderTopWidth: '1', borderBottomWidth: '1' },
    querySelector: () => list, querySelectorAll: () => heights };
  const { AlertBand: MeasuredBand } = loadSource('../src/components/AlertBand.tsx', { react: hooks }, {
    getComputedStyle: element => element.style,
    ResizeObserver: class { constructor(callback) { resize = callback; } observe() {} disconnect() { disconnected = true; } },
  });
  const alerts = [alert(1, { severity: 'WARNING', code: 'CUSTOM', title: 'warning' }),
    alert(2, { code: 'CUSTOM', title: 'older critical' }), alert(3, { code: 'CUSTOM', title: 'newer critical' })];
  const draw = () => { refIndex = 0; return renderToStaticMarkup(MeasuredBand({ alerts, wall: true })); };
  draw();
  refs[0].current = band;
  refs[1].current = heading;
  refs[2].current = { offsetHeight: 20 };
  const cleanup = effect();
  assert.equal(count, 3);
  assert.doesNotMatch(draw(), /class="wall-alert-overflow" aria-hidden="true"><article/);
  parent.clientHeight = 350;
  resize();
  assert.equal(count, 2);
  const html = draw();
  assert.ok(html.indexOf('newer critical') < html.indexOf('older critical'));
  assert.match(html, /wall-alert-overflow" aria-hidden="true"><article[^]*warning/);
  assert.match(html, /wall-alert-more">\+1件/);
  parent.clientHeight = 500;
  resize();
  assert.equal(count, 3);
  cleanup();
  assert.equal(disconnected, true);
  const css = fs.readFileSync(path.join(__dirname, '../src/style.css'), 'utf8');
  assert.match(css, /\.wall-main \.alert-list\s*\{[^}]*max-height: none;[^}]*overflow: visible;/);
  assert.match(css, /\.wall-main \.rack-groups\s*\{[^}]*flex: 1;/);
});

test('通常ヘッダーの運用フェーズは色付きセレクター1つで表示する', () => {
  for (const phase of ['setup', 'rehearsal', 'show', 'teardown']) {
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
