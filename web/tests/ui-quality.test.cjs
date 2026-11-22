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
const render = (component, props) => renderToStaticMarkup(React.createElement(component, props));
const noop = () => {};
const fault = (id, extra = {}) => ({ event_id: id, severity: 'WARNING', suppressed: false, acked: false, code: 'CUSTOM', title: 'Fault', ...extra });

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
  assert.match(render(MonitorStopped, { updatedAt: 2000 }), /監視停止中.*最終更新/);
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

test('壁タイルは大きなスイッチ名・異常ポートと機器名を表示し、小ランプと操作を置かない', () => {
  const props = { summary: { name: 'Audio SW', group_tag: 'Main', reachable: true }, ports: [{ name: '1/0/8', if_index: 8, oper: 'up' }], devices: [], stale: false };
  const html = render(WallSwitchTile, { ...props, alerts: [fault(1, { severity: 'CRITICAL', port: '1/0/8', device_name: 'Mixer' })] });
  assert.match(html, /status-critical/);
  assert.match(html, /Audio SW/);
  assert.match(html, /1\/0\/8.*Mixer/);
  assert.doesNotMatch(html, /port-lamp|<button/);
  assert.match(render(WallSwitchTile, { ...props, alerts: [], stale: true }), /status-unknown/);
  assert.match(render(WallSwitchTile, { ...props, alerts: [], ports: [{ ...props.ports[0], poe: { fault: 'overload' } }] }), /status-critical/);
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
