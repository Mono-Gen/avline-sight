import { useCallback, useEffect, useRef, useState } from 'react';
import { api, json, message } from './api';
import { AppHeader } from './components/AppHeader';
import { ExportDialog, ScrubBar, SettingsDialog } from './components/Dialogs';
import { ErrorBoundary } from './components/ErrorBoundary';
import { TerminalDrawer } from './components/TerminalDrawer';
import { asArray } from './format';
import type { Event, Overview, Profile, ReplaySession, ReplaySnapshot, Settings, Status, SwitchState, WatchItem } from './types';
import { Devices } from './views/Devices';
import { DhcpView, HealthAudit, Logs, MulticastView, PortMatrix } from './views/Monitoring';
import { OverviewView } from './views/Overview';
import { PortMonitor } from './views/PortMonitor';
import { Topology } from './views/Topology';
import { useLiveFeed } from './useLiveFeed';
import { t, useLang } from './i18n';
import { useOperations } from './useOperations';
const pages = ['overview', 'topology', 'monitor', 'ports', 'devices', 'multicast', 'dhcp', 'health', 'logs'] as const;
const replayPages: readonly string[] = ['overview', 'monitor', 'devices', 'multicast', 'health'];
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
  useLang();
  const [page, setPage] = useState(() => {
    const v = get('view', 'overview');
    return (pages as readonly string[]).includes(v) ? v : 'overview';
  });
  const [theme, setTheme] = useState(() => get('theme', 'dark'));
  const [muted, setMuted] = useState(() => get('muted', 'false') === 'true');
  const [overview, setOverview] = useState(empty);
  const [status, setStatus] = useState<Status>();
  const [profiles, setProfiles] = useState<Profile[]>([]);
  const [settings, setSettings] = useState<Settings>();
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
  const [replay, setReplay] = useState<ReplaySession | null>(null);
  const replayId = replay?.id;
  const replayStart = replay?.timeline.from;
  const socket = useLiveFeed({ muted, replay: !!replay, settings, setOverview, setState, setEvents, setOnline });
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
    const [s, pr, se, w] = await Promise.all([
      api<Status>('/api/status'),
      api<Profile[]>('/api/profiles'),
      api<Settings>('/api/settings'),
      api<WatchItem[]>('/api/watchlist')
    ]);
    setStatus(s);
    setProfiles(pr || []);
    setSettings(se);
    setWatch(w || []);
  }, []);
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
  useEffect(() => put('muted', String(muted)), [muted]);
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
      if (wall) return;
      const el = e.target as HTMLElement;
      if (e.key.toLowerCase() === 'm' && !e.ctrlKey && !e.metaKey && !e.altKey && !asArray(['INPUT', 'TEXTAREA', 'SELECT']).includes(el.tagName) && !el.isContentEditable && !terminal) marker();
    };
    window.addEventListener('keydown', key);
    return () => window.removeEventListener('keydown', key);
  }, [marker, terminal, wall]);
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
    setPage('monitor');
  };
  const locked = !status || !!status.locked || !!replay;
  const operations = useOperations({ online, replay: !!replay, profileId: status?.profile.id, setStatus, run });
  const props = {
    overview,
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
  return <div className={wall ? "wall-shell" : "min-h-screen"}>
    <AppHeader
      {...{
        online,
        wall,
        setWall,
        theme,
        setTheme,
        muted,
        setMuted,
        status,
        busy,
        locked,
        profiles,
        settings,
        part,
        setPart,
        replay,
        overview,
        marker,
        run,
        refresh,
        setDialog,
        setPage,
        setReplay
      }}
    />
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

    {error && <div role="alert" className="bg-red-900 p-3 text-white">⚠
      {error}

      <button onClick={() => setError('')}>閉じる
      </button>
    </div>}
    {busy && <div role="status" className="bg-amber-100 text-black">{t('rack.busy')}
    </div>}
    {status?.trap && !status.trap.ok && <div className="bg-amber-100 p-2 text-black">{t('rack.trapFail')}:
      {status.trap.error}
      
    </div>}
    <div className="flex flex-col md:flex-row">
      {!wall && <nav className="flex flex-wrap gap-1 bg-slate-200 p-3 dark:bg-slate-900 md:w-52 md:flex-col">
        {asArray(pages).map(v => <button
          key={v}
          disabled={!!replay && !replayPages.includes(v)}
          aria-current={page === v ? 'page' : undefined}
          onClick={() => setPage(v)}
        >
          {v}
        </button>)}
      </nav>}
      <main className="min-w-0 flex-1 space-y-4 p-4">
        <h1 className="text-2xl font-bold">
          {wall ? t('rack.wall') : t(`rack.nav.${page}` as 'rack.nav.overview')}
        </h1>
        <ErrorBoundary key={wall ? '壁表示' : page} name={wall ? '壁表示' : page}>
          {(wall || page === 'overview') && <div className={wall ? 'wallboard' : undefined}>
            <OverviewView {...props} />
          </div>}

          {!wall && page === 'topology' && <Topology {...props} />}

          {!wall && page === 'monitor' && <PortMonitor {...props} />}

          {!wall && page === 'ports' && <PortMatrix {...props} />}

          {!wall && page === 'devices' && <Devices {...props} />}

          {!wall && page === 'multicast' && <MulticastView {...props} />}

          {!wall && page === 'dhcp' && <DhcpView />}

          {!wall && page === 'health' && <HealthAudit {...props} select={setSelected} />}

          {!wall && page === 'logs' && <Logs {...props} />}
        </ErrorBoundary>
      </main>
    </div>
    {!wall && terminal > 0 && !locked && <TerminalDrawer id={terminal} close={() => setTerminal(0)} />}

    {!wall && dialog === 'export' && <ExportDialog close={() => setDialog('')} busy={busy} run={run} />}

    {!wall && dialog === 'settings' && settings && <SettingsDialog
      value={settings}
      webAddr={status?.web_addr}
      close={() => {
        setDialog('');
        void refresh().catch(e => setError(message(e)));
      }}
      busy={busy}
      run={run}
    />}
  </div>;
}
