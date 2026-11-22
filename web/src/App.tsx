import { useCallback, useEffect, useRef, useState } from 'react';
import { api, json, message } from './api';
import { AppHeader } from './components/AppHeader';
import { ExportDialog, ScrubBar, SettingsDialog } from './components/Dialogs';
import { submitPhase } from './components/PhaseControls';
import { ShowChecklist } from './components/ShowChecklist';
import type { ChecklistTarget } from './components/ShowChecklist';
import { ErrorBoundary } from './components/ErrorBoundary';
import { TerminalDrawer } from './components/TerminalDrawer';
import { TrapHelp } from './components/TrapHelp';
import { asArray } from './format';
import type { Event, Overview, Profile, ReplaySession, ReplaySnapshot, Settings, Status, SwitchState, WatchItem } from './types';
import { Devices } from './views/Devices';
import { ConfigChanges } from './views/ConfigChanges';
import { DhcpView, HealthAudit, Logs, MulticastView } from './views/Monitoring';
import { SwitchPorts } from './views/SwitchPorts';
import { SettingsTools } from './components/SettingsTools';
import { MonitorStopped, OverallStatus } from './components/MonitorStatus';
import { monitoringStopped } from './monitorHealth';
import { usePcTime } from './pcTime';
import { useDesktopNotifications } from './useDesktopNotifications';
import { PortMonitor } from './views/PortMonitor';
import { Recommendations } from './views/Recommendations';
import { Topology } from './views/Topology';
import { useLiveFeed } from './useLiveFeed';
import { t, useLang } from './i18n';
import { useOperations } from './useOperations';
import { replayIssueCount, summarizeAlerts } from './wallAlerts';
import type { PortMonitorTarget } from './views/props';
import { groupSwitches, hasGroupFilter } from './views/props';
const normalPages = ['monitor', 'devices', 'logs'] as const;
const expertPages = ['details', 'topology'] as const;
type Page = typeof normalPages[number] | typeof expertPages[number] | 'switch';
const replayPages: readonly Page[] = ['monitor', 'switch', 'devices', 'details'];
const emptyReplay: SwitchState[] = [];
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
  const [monitorTarget, setMonitorTarget] = useState<PortMonitorTarget>();
  const [detailTab, setDetailTab] = useState<'multicast' | 'dhcp' | 'health' | 'config' | 'recommendations'>('multicast');
  const [theme, setTheme] = useState(() => get('theme', 'dark'));
  const [notifications, setNotifications] = useState(() => get('desktop-notifications', 'false') === 'true');
  const [expert, setExpert] = useState(() => get('expert-display', 'false') === 'true');
  const pages = expert ? [...normalPages, ...expertPages] : normalPages;
  const [overview, setOverview] = useState(empty);
  const [status, setStatus] = useState<Status>();
  const [profiles, setProfiles] = useState<Profile[]>([]);
  const [settings, setSettings] = useState<Settings>();
  const [settingsVersion, setSettingsVersion] = useState(0);
  const [groupSelection, setGroupSelection] = useState('');
  const part = groupSelection.startsWith('view:') ? groupSelection.slice(5) : '';
  const group = groupSelection.startsWith('group:') ? groupSelection.slice(6) : undefined;
  const markerInput = useRef<HTMLInputElement>(null);
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
  const changeExpert = (enabled: boolean) => {
    setExpert(enabled);
    put('expert-display', String(enabled));
    if (!enabled) {
      setTerminal(0);
      if (page === 'details' || page === 'topology') setPage('monitor');
    }
  };
  const [dialog, setDialog] = useState('');
  const [deviceTarget, setDeviceTarget] = useState('');
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
  useEffect(() => {
    const key = (e: KeyboardEvent) => {
      if (e.key === 'Escape') { setWall(false); return; }
      if (wall || locked || busy || dialog) return;
      const el = e.target as HTMLElement;
      if (e.key.toLowerCase() === 'm' && !e.ctrlKey && !e.metaKey && !e.altKey && !asArray(['INPUT', 'TEXTAREA', 'SELECT']).includes(el.tagName) && !el.isContentEditable && !terminal) {
        e.preventDefault();
        markerInput.current?.focus();
      }
    };
    window.addEventListener('keydown', key);
    return () => window.removeEventListener('keydown', key);
  }, [busy, terminal, wall, locked, dialog]);
  useEffect(() => {
    if (!replayId || !replayStart) return;
    const controller = new AbortController();
    const url = `/api/replay/${encodeURIComponent(replayId)}/snapshot?ts=${encodeURIComponent(replayStart)}`;
    void api<ReplaySnapshot>(url, { signal: controller.signal }).then(result => {
      if (controller.signal.aborted) return;      setReplay(old => old?.id === replayId && !old.ts
        ? { ...old, ts: result.ts, requestedTs: replayStart, states: asArray(result.switches), alerts: result.alerts, portRecommendations: result.port_recommendations }
        : old);
    }).catch(reason => {
      if (!controller.signal.aborted) setError(message(reason));
    });
    return () => controller.abort();
  }, [replayId, replayStart]);
  const snapshot = (id: string, ts: string) => run(async () => {
    const result = await api<ReplaySnapshot>(`/api/replay/${encodeURIComponent(id)}/snapshot?ts=${encodeURIComponent(ts)}`);
    setReplay(old => old?.id === id
      ? { ...old, ts: result.ts, requestedTs: ts, states: asArray(result.switches), alerts: result.alerts, portRecommendations: result.port_recommendations }
      : old);
  });
  const select = (id: number) => {
    setSelected(id);
    setPage('switch');
  };
  const operations = useOperations({ online, replay: !!replay, profileId, setStatus, run });
  const phase = operations.phase || status?.phase;
  useEffect(() => {
    if (dialog === 'show-gate' && phase !== 'rehearsal' ||
        dialog === 'show-checklist' && phase !== 'setup' && phase !== 'rehearsal') {
      setDialog('');
    }
  }, [dialog, phase]);
  const groupNames = [...new Set(replay ? asArray(replay.states).map(sw => sw.group_tag) :
    [...asArray(operations.groups).map(item => item.name), ...asArray(overview.switches).map(sw => sw.group_tag)])];
  const hasGroup = group !== undefined && groupNames.includes(group);
  useEffect(() => {
    if (groupSelection && !hasGroup && !part && (replay ? replay.ts : operations.ready)) setGroupSelection('');
  }, [groupSelection, hasGroup, part, replay, operations.ready]);
  const now = usePcTime();
  const stopped = !replay && monitoringStopped(online && live.fresh, live.updatedAt, now);
  const legacyReplay = replay && replay.alerts === undefined;
  const alertSummary = summarizeAlerts(replay ? asArray(replay.alerts) : operations.alerts);
  const issueCount = legacyReplay ? replayIssueCount(asArray(replay.states)) : alertSummary.count;
  const critical = legacyReplay ? asArray(replay.states).some(state => (state.alert_counts?.CRITICAL || 0) > 0) : alertSummary.active > 0;
  const ready = replay ? !!replay.ts : operations.ready;
  const replaySummaryUnavailable = !!legacyReplay && ready && !asArray(replay.states).some(state => state.alert_counts != null);
  const activeDetailTab = replay && (detailTab === 'dhcp' || detailTab === 'config') ? 'multicast' : detailTab;
  useDesktopNotifications({ enabled: notifications, alerts: operations.alerts, profileId, ready, paused: !!replay || stopped });
  const viewScope = { overview, settings, part, group, replay: replay ? replay.states ?? emptyReplay : null };
  const overallLabel = hasGroupFilter(viewScope) ? t('cleanup.overall') : '';
  useEffect(() => {
    document.title = `(${replaySummaryUnavailable ? '—' : issueCount}) AVLine-Sight${overallLabel ? ` · ${overallLabel}` : ''}`;
  }, [issueCount, overallLabel, replaySummaryUnavailable]);
  const summary = <OverallStatus count={issueCount} critical={critical} ready={ready} stopped={stopped || replaySummaryUnavailable} replay={!!replay}
    label={overallLabel} active={legacyReplay ? undefined : alertSummary.active} unacked={legacyReplay ? undefined : alertSummary.unacked} />;
  const pageTitle = page === 'details' ? t('cleanup.details') : page === 'switch' ? t('cleanup.switchPorts') : t(`rack.nav.${page}`);
  const props = {
    ...viewScope,
    operations,
    wall,
    expert,
    onTrapHelp: () => setDialog('trap'),
    status,
    selected,
    select,
    openPortMonitor: (target: PortMonitorTarget) => {
      if (hasGroupFilter(viewScope) && !groupSwitches(viewScope).some(sw => sw.id === target.switchId)) setGroupSelection('');
      setMonitorTarget(target);
      setPage('monitor');
    },
    state,
    replayRef: replay?.ts ? { id: replay.id, ts: replay.ts } : undefined,
    replayAlerts: replay?.alerts,
    events,
    setEvents,
    run,
    busy,
    locked,
    openTerminal: expert && !locked ? setTerminal : undefined,
    watch,
    setWatch
  };
  const openChecklistTarget = (target: ChecklistTarget) => {
    setDialog(target.destination === 'trap' || target.destination === 'settings' ? target.destination : '');
    if (target.mac) {
      setGroupSelection('');
      setDeviceTarget(target.mac);
      setPage('devices');
    } else if (target.switch_id) {
      props.openPortMonitor({ switchId: target.switch_id, ifIndex: target.if_index });
    } else if (target.destination === 'logs') {
      setGroupSelection('');
      setPage('logs');
    }
  };
  return <div className={wall ? 'wall-shell' : 'min-h-screen'}>
    <AppHeader {...{ online, phase, onPhase: operations.setPhase, onTrapHelp: props.onTrapHelp,
      wall, setWall, expert, status, busy, locked, profiles, settings, groupSelection, groupNames, replay, markerInput, run, refresh, setDialog, summary,
      updatedAt: replay ? replay.ts : live.updatedAt }}
      openChecklist={gate => {
        setError('');
        setDialog(gate ? 'show-gate' : 'show-checklist');
      }}
      setGroupSelection={value => {
        if (value === groupSelection) return;
        setMonitorTarget(undefined);
        setGroupSelection(value);
      }} />
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

    {error && <div role="alert" className="bg-red-900 p-3 text-white">⚠
      {error}

      <button onClick={() => setError('')}>{t('rack.close')}
      </button>
    </div>}
    {busy && <div role="status" className="bg-amber-100 text-black">{t('rack.busy')}
    </div>}
    <div className={wall ? 'wall-content' : 'flex flex-col md:flex-row'}>
      {!wall && <nav className="app-nav">
        {pages.map(v => <button
          key={v}
          disabled={!!replay && !replayPages.includes(v)}
          aria-current={page === v || page === 'switch' && v === 'monitor' ? 'page' : undefined}
          onClick={() => { setMonitorTarget(undefined); setDeviceTarget(''); setPage(v); }}
        >
          {v === 'details' ? t('cleanup.details') : t(`rack.nav.${v}`)}
        </button>)}
      </nav>}
      <main className={wall ? 'wall-main' : 'min-w-0 flex-1 space-y-4 p-4'}>
        {!wall && <h1 className="text-2xl font-bold">{pageTitle}</h1>}
        <ErrorBoundary key={wall ? 'wall' : page} name={wall ? t('rack.wall') : pageTitle}>
          {(wall || page === 'monitor') && <PortMonitor key={status?.profile.id || 0} {...props} target={wall ? undefined : monitorTarget}
            onClosePortDetail={() => setMonitorTarget(undefined)} />}

          {!wall && expert && page === 'topology' && <Topology key={status?.profile.id || 0} {...props}
            openDevices={() => setPage('devices')} />}

          {!wall && page === 'switch' && <SwitchPorts {...props} />}


          {!wall && page === 'devices' && <Devices key={deviceTarget} {...props} initialQuery={deviceTarget} />}

          {!wall && expert && page === 'details' && <>
            <div role="tablist" aria-label={t('cleanup.details')} className="detail-tabs">
              {(['multicast', 'dhcp', 'health', 'config', 'recommendations'] as const).map(tab => <button key={tab} role="tab" id={`tab-${tab}`}
                aria-controls="detail-panel" aria-selected={activeDetailTab === tab} disabled={!!replay && (tab === 'dhcp' || tab === 'config')} onClick={() => setDetailTab(tab)}>
                {tab === 'config' ? t('configChanges.title') : tab === 'recommendations' ? t('expert.recommendations') : t(`rack.nav.${tab}`)}
              </button>)}
            </div>
            <div role="tabpanel" id="detail-panel" aria-labelledby={`tab-${activeDetailTab}`}>
              {activeDetailTab === 'multicast' && <MulticastView {...props} />}
              {activeDetailTab === 'dhcp' && <DhcpView {...props} />}
              {activeDetailTab === 'health' && <HealthAudit {...props} select={setSelected} />}
              {activeDetailTab === 'config' && <ConfigChanges key={status?.profile.id} {...props} />}
              {activeDetailTab === 'recommendations' && <Recommendations {...props} replayRows={replay?.portRecommendations} />}
            </div>
          </>}

          {!wall && page === 'logs' && <Logs {...props} />}
        </ErrorBoundary>
      </main>
    </div>
    {!wall && expert && terminal > 0 && !locked && <TerminalDrawer id={terminal} close={() => setTerminal(0)} />}

    {!wall && !locked && dialog === 'export' && <ExportDialog close={() => setDialog('')} busy={busy} run={run} />}

    {!wall && !replay && (phase === 'setup' || phase === 'rehearsal') &&
      (dialog === 'show-checklist' || dialog === 'show-gate' && phase === 'rehearsal') &&
      <ShowChecklist key={profileId} gate={dialog === 'show-gate'} busy={busy} locked={locked} actionError={error} close={() => setDialog('')}
        onTarget={openChecklistTarget} onStart={confirmChecklist => run(async () => {
          await submitPhase('show', operations.setPhase, confirmChecklist);
          setDialog('');
        })} />}

    {!wall && dialog === 'trap' && <TrapHelp close={() => setDialog('')} error={status?.trap.error} />}
    {!wall && dialog === 'settings' && settings && <SettingsDialog
      key={profileId}
      locked={locked}
      tools={<SettingsTools {...{ theme, setTheme, notifications, setNotifications, expert, busy, locked, groups: operations.groups, profileId, run, setDialog }}
        setExpert={changeExpert}
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
