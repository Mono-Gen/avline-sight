import { useCallback, useEffect, useRef, useState } from 'react';
import { api, json, message } from './api';
import { AppHeader } from './components/AppHeader';
import { AppSidebar } from './components/AppSidebar';
import { ConfigCredentialsDialog } from './components/ConfigCredentialsDialog';
import { ConfigImportDialog } from './components/ConfigImportDialog';
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
import type { PortMonitorTarget, SwitchFilter } from './views/props';
import { emptySwitchFilter } from './views/props';
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
  const [expertOverride, setExpertOverride] = useState<boolean | undefined>(() => {
    const saved = get('expert-display', '');
    return saved === 'true' ? true : saved === 'false' ? false : undefined;
  });
  const [overview, setOverview] = useState(empty);
  const [status, setStatus] = useState<Status>();
  const [profiles, setProfiles] = useState<Profile[]>([]);
  const [settings, setSettings] = useState<Settings>();
  const expert = expertOverride ?? settings?.expert_display_default ?? false;
  const pages = expert ? [...normalPages, ...expertPages] : normalPages;
  const [settingsVersion, setSettingsVersion] = useState(0);
  const [monitorFilter, setMonitorFilter] = useState<SwitchFilter>(emptySwitchFilter);
  const markerInput = useRef<HTMLInputElement>(null);
  const [online, setOnline] = useState(false);
  const [events, setEvents] = useState<Event[]>([]);
  const [selected, setSelected] = useState(0);
  const [state, setState] = useState<SwitchState>();
  const [watch, setWatch] = useState<WatchItem[]>([]);
  const [busy, setBusy] = useState(false);
  const busyRef = useRef(false);
  const [error, setError] = useState('');
  const [startupNoticeDismissed, setStartupNoticeDismissed] = useState(false);
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
    setExpertOverride(enabled);
    put('expert-display', String(enabled));
    if (!enabled) {
      setTerminal(0);
      if (page === 'details' || page === 'topology') setPage('monitor');
    }
  };
  const [dialog, setDialog] = useState('');
  useEffect(() => {
    if (expert || !settings) return;
    setTerminal(0);
    setPage(current => current === 'details' || current === 'topology' ? 'monitor' : current);
  }, [expert, settings]);
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
  const canOpenShowChecklist = phase === 'setup' || phase === 'rehearsal';
  useEffect(() => {
    if ((dialog === 'show-gate' || dialog === 'show-checklist') && !canOpenShowChecklist) {
      setDialog('');
    }
  }, [dialog, canOpenShowChecklist]);
  useEffect(() => setMonitorFilter(emptySwitchFilter), [profileId, replayId]);
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
  const viewScope = { overview, settings, ...emptySwitchFilter, replay: replay ? replay.states ?? emptyReplay : null };
  const monitorScope = { ...viewScope, ...monitorFilter };
  const overallLabel = !wall && page === 'monitor' && hasGroupFilter(monitorScope) ? t('cleanup.overall') : '';
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
      if (hasGroupFilter(monitorScope) && !groupSwitches(monitorScope).some(sw => sw.id === target.switchId)) setMonitorFilter(emptySwitchFilter);
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
      setDeviceTarget(target.mac);
      setPage('devices');
    } else if (target.switch_id) {
      props.openPortMonitor({ switchId: target.switch_id, ifIndex: target.if_index });
    } else if (target.destination === 'logs') {
      setPage('logs');
    }
  };
  return <div className={wall ? 'wall-shell' : 'flex min-h-screen flex-col'}>
    <AppHeader {...{ online, phase, onPhase: operations.setPhase, onTrapHelp: props.onTrapHelp,
      wall, setWall, expert, status, busy, locked, replay, markerInput, run, summary,
      updatedAt: replay ? replay.ts : live.updatedAt }}
      openChecklist={gate => {
        setError('');
        setDialog(gate ? 'show-gate' : 'show-checklist');
      }} />
    {stopped && <MonitorStopped updatedAt={live.updatedAt} />}
    {status?.config_startup_error && !startupNoticeDismissed && <div role="alert" className="panel p-3">
      ⚠ {t('configFile.startupError', { reason: status.config_startup_error })}
      <button onClick={() => setStartupNoticeDismissed(true)}>{t('rack.close')}</button>
    </div>}
    {!replay && (status?.credentials_pending ?? 0) > 0 && <div role="status" className="panel p-3">
      ⚠ {t('configFile.credentialsPending', { count: status?.credentials_pending })}
      {!wall && <button disabled={busy} onClick={() => setDialog('config-credentials')}>{t('configFile.enterCredentials')}</button>}
    </div>}
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
    <div className={wall ? 'wall-content' : 'app-workspace flex flex-1 flex-col md:flex-row'}>
      {!wall && <AppSidebar {...{ profiles, status, busy, locked, expert, run, refresh }} replay={!!replay}
        setExpert={changeExpert} openSettings={() => setDialog('settings')}>
        {pages.map(v => <button
          key={v}
          disabled={!!replay && !replayPages.includes(v)}
          aria-current={page === v || page === 'switch' && v === 'monitor' ? 'page' : undefined}
          onClick={() => { setMonitorTarget(undefined); setDeviceTarget(''); setPage(v); }}
        >
          {v === 'details' ? t('cleanup.details') : t(`rack.nav.${v}`)}
        </button>)}
      </AppSidebar>}
      <main className={wall ? 'wall-main' : 'min-w-0 flex-1 space-y-4 p-4'}>
        {!wall && <h1 className="text-2xl font-bold">{pageTitle}</h1>}
        <ErrorBoundary key={wall ? 'wall' : page} name={wall ? t('rack.wall') : pageTitle}>
          {(wall || page === 'monitor') && <PortMonitor key={status?.profile.id || 0} {...props}
            {...(wall ? emptySwitchFilter : monitorFilter)} target={wall ? undefined : monitorTarget}
            onFilterChange={filter => {
              setMonitorTarget(undefined);
              setMonitorFilter(filter);
            }}
            onClosePortDetail={() => setMonitorTarget(undefined)} />}

          {!wall && expert && page === 'topology' && <Topology key={status?.profile.id || 0} {...props}
            openDevices={() => setPage('devices')} />}

          {!wall && page === 'switch' && <SwitchPorts {...props} />}


          {!wall && page === 'devices' && <Devices key={JSON.stringify([profileId, replayId, deviceTarget])} {...props} initialQuery={deviceTarget} />}

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
    {!wall && dialog === 'config-import' && <ConfigImportDialog locked={locked} close={() => setDialog('settings')}
      onApplied={async () => {
        setSelected(0);
        setMonitorTarget(undefined);
        setMonitorFilter(emptySwitchFilter);
        setPage('monitor');
        setDialog('');
        await refresh().catch(reason => setError(message(reason)));
      }} />}
    {!wall && dialog === 'config-credentials' && <ConfigCredentialsDialog {...{ profileId, busy, locked, run, refresh }}
      close={() => setDialog('')} />}

    {!wall && !replay && canOpenShowChecklist && (dialog === 'show-checklist' || dialog === 'show-gate') &&
      <ShowChecklist key={profileId} gate={dialog === 'show-gate'} busy={busy} locked={locked} actionError={error} close={() => setDialog('')}
        onTarget={openChecklistTarget} onStart={confirmChecklist => run(async () => {
          await submitPhase('show', operations.setPhase, confirmChecklist);
          setDialog('');
        })} />}

    {!wall && dialog === 'trap' && <TrapHelp close={() => setDialog('')} error={status?.trap.error} />}
    {!wall && dialog === 'settings' && settings && <SettingsDialog
      key={profileId}
      profileId={profileId}
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
