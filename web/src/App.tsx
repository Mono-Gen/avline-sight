import { useCallback, useEffect, useRef, useState } from 'react';
import { api, json, message } from './api';
import { AppHeader } from './components/AppHeader';
import { ExportDialog, ScrubBar, SettingsDialog } from './components/Dialogs';
import { ErrorBoundary } from './components/ErrorBoundary';
import { TerminalDrawer } from './components/TerminalDrawer';
import { TrapHelp } from './components/TrapHelp';
import { asArray } from './format';
import type { Event, Overview, Profile, ReplaySession, ReplaySnapshot, Settings, Status, SwitchState, WatchItem } from './types';
import { Devices } from './views/Devices';
import { DhcpView, HealthAudit, Logs, MulticastView } from './views/Monitoring';
import { SwitchPorts } from './views/SwitchPorts';
import { SettingsTools } from './components/SettingsTools';
import { MonitorStopped, OverallStatus } from './components/MonitorStatus';
import { issueAlerts, monitoringStopped } from './monitorHealth';
import { usePcTime } from './pcTime';
import { useDesktopNotifications } from './useDesktopNotifications';
import { PortMonitor } from './views/PortMonitor';
import { Topology } from './views/Topology';
import { useLiveFeed } from './useLiveFeed';
import { t, useLang } from './i18n';
import { useOperations } from './useOperations';
const pages = ['monitor', 'topology', 'devices', 'details', 'logs'] as const;
type Page = typeof pages[number] | 'switch';
const replayPages: readonly string[] = ['monitor', 'switch', 'devices', 'details'];
const empty: Overview = {
  switches: [],
  alerts: {
    critical: 0,
    warning: 0,
    info: 0
  },
  recent_events: []
};
function get(key: string, fallback: string) {
  try {
    return localStorage.getItem(key) || fallback;
  } catch {
    return fallback;
  }
}
function put(key: string, value: string) {
  try {
    localStorage.setItem(key, value);
  } catch {
    return;
  }
}
export default function App() {
  const [language] = useLang();
  useEffect(() => { document.documentElement.lang = language; }, [language]);
  const [page, setPage] = useState<Page>('monitor');
  const [detailTab, setDetailTab] = useState<'multicast' | 'dhcp' | 'health'>('multicast');
  const [theme, setTheme] = useState(() => get('theme', 'dark'));
  const [notifications, setNotifications] = useState(() => get('desktop-notifications', 'false') === 'true');
  const [overview, setOverview] = useState(empty);
  const [status, setStatus] = useState<Status>();
  const [profiles, setProfiles] = useState<Profile[]>([]);
  const [settings, setSettings] = useState<Settings>();
  const [settingsVersion, setSettingsVersion] = useState(0);
  const [part, setPart] = useState('');
  const [online, setOnline] = useState(false);
  const [events, setEvents] = useState<Event[]>([]);
  const [selected, setSelected] = useState(0);
  const [state, setState] = useState<SwitchState>();
  const [watch, setWatch] = useState<WatchItem[]>([]);
  const [busy, setBusy] = useState(false);
  const busyRef = useRef(false);
  const [error, setError] = useState('');
  const [wall, setWall] = useState(() => typeof location !== 'undefined' && (new URLSearchParams(location.search).has('wall') ? new URLSearchParams(location.search).get('wall') === '1' : get('wall', 'false') === 'true'));
  useEffect(() => {
    put('wall', String(wall));
    const url = new URL(location.href);
    if (wall) url.searchParams.set('wall', '1'); else url.searchParams.delete('wall');
    history.replaceState(null, '', url);
    document.documentElement.classList.toggle('wall-active', wall);
    return () => document.documentElement.classList.remove('wall-active');
  }, [wall]);
  const [terminal, setTerminal] = useState(0);
  const [dialog, setDialog] = useState('');
  const trapHelpShown = useRef(false);
  const [replay, setReplay] = useState<ReplaySession | null>(null);
  const replayId = replay?.id;
  const replayStart = replay?.timeline.from;
  const profileId = status?.profile.id;
  const locked = !status || status.locked || !!replay;
  const trapFailed = status?.trap?.ok === false;
  useEffect(() => {
    if (wall || dialog || !trapFailed || trapHelpShown.current) return;
    trapHelpShown.current = true;
    let shown = false;
    try {
      shown = sessionStorage.getItem('trap-help-shown') === 'yes';
      sessionStorage.setItem('trap-help-shown', 'yes');
    } catch {
      shown = false;
    }
    if (!shown) setDialog('trap');
  }, [wall, dialog, trapFailed]);
  const live = useLiveFeed({ setOverview, setState, setEvents, setOnline });
  const socket = live.socket;
  const run = useCallback((f: () => Promise<void>) => {
    if (busyRef.current) return;
    busyRef.current = true;
    setBusy(true);
    setError('');
    void f().catch(e => setError(message(e))).finally(() => {
      busyRef.current = false;
      setBusy(false);
    });
  }, []);
  const refresh = useCallback(async () => {
    const [s, pr] = await Promise.all([
      api<Status>('/api/status'),
      api<Profile[]>('/api/profiles'),
    ]);
    setStatus(s);
    setProfiles(pr || []);
  }, []);
  useEffect(() => {
    if (profileId === undefined) return;
    const request = new AbortController();
    const { signal } = request;
    setSettings(undefined);
    setWatch([]);
    void Promise.all([
      api<Settings>('/api/settings', { signal }),
      api<WatchItem[]>('/api/watchlist', { signal }),
    ]).then(([currentSettings, currentWatch]) => {
      if (signal.aborted) return;
      setSettings(currentSettings);
      setWatch(asArray(currentWatch));
    }).catch(reason => {
      if (!signal.aborted) setError(message(reason));
    });
    return () => request.abort();
  }, [profileId, settingsVersion]);
  useEffect(() => {
    const handler = (e: globalThis.Event) => setError((e as CustomEvent<string>).detail);
    window.addEventListener('api-error', handler);
    return () => window.removeEventListener('api-error', handler);
  }, []);
  useEffect(() => {
    void refresh().catch(e => setError(message(e)));
  }, [refresh]);
  useEffect(() => {
    document.documentElement.classList.toggle('dark', theme === 'dark');
    put('theme', theme);
  }, [theme]);
  useEffect(() => put('desktop-notifications', String(notifications)), [notifications]);
  useEffect(() => put('view', page), [page]);
  useEffect(() => {
    socket.current?.subscribe(replay ? 0 : selected);
    if (selected && !replay) api<SwitchState>(`/api/switches/${selected}/state`).then(setState).catch(e => setError(message(e)));
  }, [selected, replay]);
  useEffect(() => {
    if (online || replay) return;
    const load = () => api<Overview>('/api/overview').then(setOverview).catch(e => setError(message(e)));
    void load();
    const timer = setInterval(load, 5000);
    return () => clearInterval(timer);
  }, [online, replay]);
  const marker = useCallback(() => {
    const note = prompt(t('rack.marker'));
    if (note !== null) run(async () => {
      await api('/api/markers', json('POST', { note }));
    });
  }, [run]);
  useEffect(() => {
    const key = (e: KeyboardEvent) => {
      if (e.key === 'Escape') { setWall(false); return; }
      if (wall || locked || dialog) return;
      const el = e.target as HTMLElement;
      if (e.key.toLowerCase() === 'm' && !e.ctrlKey && !e.metaKey && !e.altKey && !asArray(['INPUT', 'TEXTAREA', 'SELECT']).includes(el.tagName) && !el.isContentEditable && !terminal) marker();
    };
    window.addEventListener('keydown', key);
    return () => window.removeEventListener('keydown', key);
  }, [marker, terminal, wall, locked, dialog]);
  useEffect(() => {
    if (!replayId || !replayStart) return;
    const controller = new AbortController();
    const url = `/api/replay/${encodeURIComponent(replayId)}/snapshot?ts=${encodeURIComponent(replayStart)}`;
    void api<ReplaySnapshot>(url, { signal: controller.signal }).then(result => {
      if (controller.signal.aborted) return;      setReplay(old => old?.id === replayId && !old.ts
        ? { ...old, ts: result.ts, requestedTs: replayStart, states: asArray(result.switches) }
        : old);
    }).catch(reason => {
      if (!controller.signal.aborted) setError(message(reason));
    });
    return () => controller.abort();
  }, [replayId, replayStart]);
  const snapshot = (id: string, ts: string) => run(async () => {
    const result = await api<ReplaySnapshot>(`/api/replay/${encodeURIComponent(id)}/snapshot?ts=${encodeURIComponent(ts)}`);
    setReplay(old => old?.id === id
      ? { ...old, ts: result.ts, requestedTs: ts, states: asArray(result.switches) }
      : old);
  });
  const select = (id: number) => {
    setSelected(id);
    setPage('switch');
  };
  const operations = useOperations({ online, replay: !!replay, profileId, setStatus, run });
  const now = usePcTime();
  const stopped = !replay && monitoringStopped(online && live.fresh, live.updatedAt, now);
  const issues = issueAlerts(operations.alerts);
  const issueCount = replay ? asArray(replay.states).reduce((count, state) => count + (state.alert_counts?.CRITICAL || 0) + (state.alert_counts?.WARNING || 0), 0) : issues.length;
  const critical = replay ? asArray(replay.states).some(state => (state.alert_counts?.CRITICAL || 0) > 0) : issues.some(alert => alert.severity === 'CRITICAL');
  const ready = replay ? !!replay.ts : operations.ready;
  const activeDetailTab = replay && detailTab === 'dhcp' ? 'multicast' : detailTab;
  useDesktopNotifications({ enabled: notifications, alerts: operations.alerts, profileId, ready, paused: !!replay || stopped });
  useEffect(() => { document.title = `(${issueCount}) AVLine-Sight`; }, [issueCount]);
  const summary = <OverallStatus count={issueCount} critical={critical} ready={ready} stopped={stopped} replay={!!replay} />;
  const pageTitle = page === 'details' ? t('cleanup.details') : page === 'switch' ? t('cleanup.switchPorts') : t(`rack.nav.${page}`);
  const props = {
    overview,
    operations,
    wall,
    onTrapHelp: () => setDialog('trap'),
    status,
    settings,
    part,
    selected,
    select,
    state,
    replay: replay?.states || null,
    replayRef: replay?.ts ? { id: replay.id, ts: replay.ts } : undefined,
    events,
    setEvents,
    run,
    busy,
    locked,
    openTerminal: setTerminal,
    watch,
    setWatch
  };
  return <div className={wall ? 'wall-shell' : 'min-h-screen'}>
    <AppHeader {...{ online, phase: operations.phase || status?.phase, onPhase: operations.setPhase, onTrapHelp: props.onTrapHelp,
      wall, setWall, status, busy, locked, profiles, settings, part, setPart, replay, marker, run, refresh, setDialog, summary }} />
    {stopped && <MonitorStopped updatedAt={live.updatedAt} />}
    {replay && !wall && <ScrubBar
      key={replay.id}
      sid={replay.id}
      timeline={replay.timeline}
      ts={replay.ts}
      requestedTs={replay.requestedTs}
      busy={busy}
      onSnapshot={ts => snapshot(replay.id, ts)}
      onClose={() => setReplay(null)}
      run={run}
    />}

    {!wall && error && <div role="alert" className="bg-red-900 p-3 text-white">⚠
      {error}

      <button onClick={() => setError('')}>{t('rack.close')}
      </button>
    </div>}
    {!wall && busy && <div role="status" className="bg-amber-100 text-black">{t('rack.busy')}
    </div>}
    <div className={wall ? 'wall-content' : 'flex flex-col md:flex-row'}>
      {!wall && <nav className="app-nav">
        {asArray(pages).map(v => <button
          key={v}
          disabled={!!replay && !replayPages.includes(v)}
          aria-current={page === v || page === 'switch' && v === 'monitor' ? 'page' : undefined}
          onClick={() => setPage(v)}
        >
          {v === 'details' ? t('cleanup.details') : t(`rack.nav.${v}`)}
        </button>)}
      </nav>}
      <main className={wall ? 'wall-main' : 'min-w-0 flex-1 space-y-4 p-4'}>
        {!wall && <h1 className="text-2xl font-bold">{pageTitle}</h1>}
        <ErrorBoundary key={wall ? 'wall' : page} name={wall ? t('rack.wall') : pageTitle}>
          {(wall || page === 'monitor') && <PortMonitor key={status?.profile.id || 0} {...props} />}

          {!wall && page === 'topology' && <Topology {...props} />}

          {!wall && page === 'switch' && <SwitchPorts {...props} />}


          {!wall && page === 'devices' && <Devices {...props} />}

          {!wall && page === 'details' && <>
            <div role="tablist" aria-label={t('cleanup.details')} className="detail-tabs">
              {(['multicast', 'dhcp', 'health'] as const).map(tab => <button key={tab} role="tab" id={`tab-${tab}`}
                aria-controls="detail-panel" aria-selected={activeDetailTab === tab} disabled={!!replay && tab === 'dhcp'} onClick={() => setDetailTab(tab)}>{t(`rack.nav.${tab}`)}</button>)}
            </div>
            <div role="tabpanel" id="detail-panel" aria-labelledby={`tab-${activeDetailTab}`}>
              {activeDetailTab === 'multicast' && <MulticastView {...props} />}
              {activeDetailTab === 'dhcp' && <DhcpView />}
              {activeDetailTab === 'health' && <HealthAudit {...props} select={setSelected} />}
            </div>
          </>}

          {!wall && page === 'logs' && <Logs {...props} />}
        </ErrorBoundary>
      </main>
    </div>
    {!wall && terminal > 0 && !locked && <TerminalDrawer id={terminal} close={() => setTerminal(0)} />}

    {!wall && !locked && dialog === 'export' && <ExportDialog close={() => setDialog('')} busy={busy} run={run} />}

    {!wall && dialog === 'trap' && <TrapHelp close={() => setDialog('')} error={status?.trap.error} />}
    {!wall && dialog === 'settings' && settings && <SettingsDialog
      key={profileId}
      locked={locked}
      tools={<SettingsTools {...{ theme, setTheme, notifications, setNotifications, busy, locked, groups: operations.groups, profileId, run, setDialog }}
        replay={!!replay} refresh={operations.refresh} setReplay={next => { setPage('monitor'); setReplay(next); }} />}
      value={settings}
      webAddr={status?.web_addr}
      close={() => {
        setDialog('');
        setSettingsVersion(value => value + 1);
      }}
      busy={busy}
      run={run}
    />}
  </div>;
}
