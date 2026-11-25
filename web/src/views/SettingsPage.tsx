import { useEffect, useState } from 'react';
import type { Dispatch, ReactNode, SetStateAction } from 'react';
import { api, json } from '../api';
import { asArray } from '../format';
import { t } from '../i18n';
import type { TranslationKey } from '../i18n';
import { pcTimestamp } from '../pcTime';
import type { ReplaySession, Settings, Status, SwitchGroup, SwitchSummary } from '../types';
import { BrowserDisplaySettings, DataTransfers, GroupOrder } from '../components/SettingsTools';
import { TrapListenSetting } from '../components/TrapListenSetting';
import { CopyCommand, switchTrapCommands } from '../components/TrapHelp';

type Run = (action: () => Promise<void>) => void;
type Scope = 'app' | 'project' | 'browser';
type SectionId = 'monitor' | 'trap' | 'parts' | 'display' | 'data' | 'groups';

// Every threshold with its unit and the direction that raises a finding, as the engine compares it.
const thresholdRows: { key: string; label: TranslationKey; rule: TranslationKey }[] = [
  { key: 'cpu_pct', label: 'ui.m066', rule: 'settingsPage.atLeastPct' },
  { key: 'mem_free_pct', label: 'ui.m050', rule: 'settingsPage.atMostPct' },
  { key: 'temp_c', label: 'ui.m056', rule: 'settingsPage.atLeastC' },
  { key: 'poe_pct', label: 'ui.m173', rule: 'settingsPage.atLeastPct' },
  { key: 'util_pct', label: 'ui.m174', rule: 'settingsPage.atLeastPct' },
  { key: 'sfp_rx_dbm', label: 'ui.m175', rule: 'settingsPage.belowDbm' },
  { key: 'sfp_drop_db', label: 'ui.m176', rule: 'settingsPage.dropDb' },
  { key: 'err_delta', label: 'ui.m177', rule: 'settingsPage.errors' },
];
const sections: { id: SectionId; label: TranslationKey }[] = [
  { id: 'monitor', label: 'settingsPage.monitor' }, { id: 'trap', label: 'settingsPage.trap' },
  { id: 'parts', label: 'settingsPage.parts' }, { id: 'display', label: 'settingsPage.display' },
  { id: 'data', label: 'settingsPage.data' }, { id: 'groups', label: 'settingsPage.groups' },
];
const commaList = (value: string) => value.split(',').map(item => item.trim()).filter(Boolean);
const validPing = (seconds: number) => Number.isInteger(seconds) && seconds >= 30 && seconds <= 3600;

export function ScopeTag({ scope }: { scope: Scope }) {
  const label: TranslationKey = scope === 'app' ? 'settingsPage.scopeApp' : scope === 'project' ? 'settingsPage.scopeProject' : 'settingsPage.scopeBrowser';
  return <span className={`settings-scope settings-scope-${scope}`}>{t(label)}</span>;
}

function Card({ id, title, scope, children }: { id: SectionId; title: TranslationKey; scope?: Scope; children: ReactNode }) {
  return <section id={`settings-${id}`} className="panel settings-card" aria-label={t(title)}>
    <div className="settings-card-head"><h2>{t(title)}</h2>{scope && <ScopeTag scope={scope} />}</div>
    {children}
  </section>;
}

// The whole settings body as the server takes it; a section changes only its own part.
export function settingsBody(base: Settings, patch: Partial<Settings>) {
  return {
    thresholds: base.thresholds, setup_ping_interval_sec: base.setup_ping_interval_sec,
    rehearsal_ping_interval_sec: base.rehearsal_ping_interval_sec, expert_display_default: base.expert_display_default ?? false,
    views: base.views, ...patch,
  };
}

function SaveBar({ dirty, busy, locked, onSave, onReset, error, children }: {
  dirty: boolean; busy: boolean; locked: boolean; onSave: () => void; onReset?: () => void; error?: string; children?: ReactNode;
}) {
  return <div className="settings-savebar">
    {children}
    <button className="button-primary" data-action="save-section" disabled={busy || locked || !dirty} onClick={onSave}>{t('settingsPage.save')}</button>
    {onReset && <button disabled={busy || !dirty} onClick={onReset}>{t('settingsPage.reset')}</button>}
    {dirty && <span className="settings-unsaved">● {t('settingsPage.unsaved')}</span>}
    {error && <p role="alert">⚠ {error}</p>}
  </div>;
}

function MonitorSection({ value, save, busy, locked, onDirty }: {
  value: Settings; save: (patch: Partial<Settings>) => void; busy: boolean; locked: boolean; onDirty: (dirty: boolean) => void;
}) {
  const [thresholds, setThresholds] = useState<Record<string, number>>({ ...value.thresholds });
  // The server default (60 s) when the stored value is missing, the same for the field, the change mark and "undo".
  const savedSetup = value.setup_ping_interval_sec ?? 60;
  const savedRehearsal = value.rehearsal_ping_interval_sec ?? 60;
  const [setupPing, setSetupPing] = useState(savedSetup);
  const [rehearsalPing, setRehearsalPing] = useState(savedRehearsal);
  const [error, setError] = useState('');
  const changed = (key: string) => thresholds[key] !== value.thresholds?.[key];
  const dirty = Object.keys(thresholds).some(changed) || setupPing !== savedSetup || rehearsalPing !== savedRehearsal;
  useEffect(() => onDirty(dirty), [dirty]); // eslint-disable-line react-hooks/exhaustive-deps
  const number = (text: string) => text === '' ? NaN : Number(text);
  const row = (key: string, label: string, rule: string, current: number, set: (value: number) => void, before: number | undefined, extra?: object) =>
    <div key={key} className={`settings-row${current !== before ? ' settings-changed' : ''}`}>
      <label className="settings-label" htmlFor={`settings-${key}`}>{label}</label>
      <span><input id={`settings-${key}`} type="number" step="any" className="settings-number" {...extra}
        value={Number.isFinite(current) ? current : ''} onChange={event => set(number(event.target.value))} />
        <span className="settings-unit">{rule}</span>
        {current !== before && <span className="settings-was">{t('settingsPage.was', { value: before ?? '—' })}</span>}</span>
    </div>;
  const known = thresholdRows.filter(item => item.key in thresholds);
  const others = Object.keys(thresholds).filter(key => !thresholdRows.some(item => item.key === key));
  return <fieldset className="settings-fieldset" disabled={busy || locked}>
    {known.map(item => row(item.key, t(item.label), t(item.rule), thresholds[item.key], next => setThresholds({ ...thresholds, [item.key]: next }), value.thresholds?.[item.key]))}
    {others.map(key => row(key, key, '', thresholds[key], next => setThresholds({ ...thresholds, [key]: next }), value.thresholds?.[key]))}
    {row('setup_ping', t('cleanup.setupPing'), t('settingsPage.seconds'), setupPing, setSetupPing, savedSetup, { min: 30, max: 3600, step: 1 })}
    {row('rehearsal_ping', t('cleanup.rehearsalPing'), t('settingsPage.seconds'), rehearsalPing, setRehearsalPing, savedRehearsal, { min: 30, max: 3600, step: 1 })}
    <p className="settings-hint">{t('cleanup.pingBounds')}</p>
    <SaveBar dirty={dirty} busy={busy} locked={locked} error={error}
      onReset={() => { setThresholds({ ...value.thresholds }); setSetupPing(savedSetup); setRehearsalPing(savedRehearsal); setError(''); }}
      onSave={() => {
        if (Object.values(thresholds).some(item => !Number.isFinite(item))) { setError(t('ui.m178')); return; }
        if (![setupPing, rehearsalPing].every(validPing)) { setError(t('cleanup.pingInvalid')); return; }
        setError('');
        save({ thresholds, setup_ping_interval_sec: setupPing, rehearsal_ping_interval_sec: rehearsalPing });
      }} />
  </fieldset>;
}

type ViewDraft = { id: string; name: string; vlans: string; tags: string; categories: string[] };
// Compared after parsing, so "10, 20" and "10,20" are the same saved value.
const parseViews = (views: ViewDraft[]) => views.map(view => ({ ...view, tags: commaList(view.tags), vlans: commaList(view.vlans).map(Number) }));
const drafts = (value: Settings): ViewDraft[] => asArray(value.views).map(view => ({
  id: view.id, name: view.name, vlans: asArray(view.vlans).join(', '), tags: asArray(view.tags).join(', '), categories: asArray(view.categories),
}));

function PartsSection({ value, save, busy, locked, onDirty }: {
  value: Settings; save: (patch: Partial<Settings>) => void; busy: boolean; locked: boolean; onDirty: (dirty: boolean) => void;
}) {
  const [views, setViews] = useState<ViewDraft[]>(() => drafts(value));
  const [error, setError] = useState('');
  const dirty = JSON.stringify(parseViews(views)) !== JSON.stringify(parseViews(drafts(value)));
  useEffect(() => onDirty(dirty), [dirty]); // eslint-disable-line react-hooks/exhaustive-deps
  const change = (index: number, key: 'name' | 'vlans' | 'tags', text: string) =>
    setViews(old => old.map((view, position) => position === index ? { ...view, [key]: text } : view));
  return <fieldset className="settings-fieldset" disabled={busy || locked}>
    <p className="settings-hint">{t('settingsPage.partsHelp')}</p>
    <table className="settings-parts">
      <thead><tr><th>{t('ui.m017')}</th><th>{t('ui.m183')}</th><th>{t('ui.m184')}</th><th /></tr></thead>
      <tbody>{views.map((view, index) => <tr key={view.id}>
        <td><input aria-label={t('ui.m017')} value={view.name} onChange={event => change(index, 'name', event.target.value)} /></td>
        <td><input aria-label={t('ui.m183')} value={view.vlans} onChange={event => change(index, 'vlans', event.target.value)} /></td>
        <td><input aria-label={t('ui.m184')} value={view.tags} onChange={event => change(index, 'tags', event.target.value)} /></td>
        <td><button className="button-danger" disabled={busy || locked} onClick={() => setViews(old => old.filter((_, position) => position !== index))}>{t('ui.m185')}</button></td>
      </tr>)}</tbody>
    </table>
    <SaveBar dirty={dirty} busy={busy} locked={locked} error={error} onReset={() => { setViews(drafts(value)); setError(''); }}
      onSave={() => {
        const parsed = parseViews(views);
        if (parsed.some(view => !view.name.trim() || view.vlans.some(vlan => !Number.isInteger(vlan) || vlan < 1 || vlan > 4094))) {
          setError(t('ui.m178')); return;
        }
        setError('');
        save({ views: parsed });
      }}>
      <button disabled={busy || locked} onClick={() => setViews(old => [...old, { id: `view-${Date.now()}-${old.length}`, name: '', vlans: '', tags: '', categories: [] }])}>{t('ui.m186')}</button>
      <span className="settings-spacer" />
    </SaveBar>
  </fieldset>;
}

function ExpertDefault({ value, save, busy, locked, onDirty }: {
  value: Settings; save: (patch: Partial<Settings>) => void; busy: boolean; locked: boolean; onDirty: (dirty: boolean) => void;
}) {
  const saved = value.expert_display_default ?? false;
  const [expert, setExpert] = useState(saved);
  const dirty = expert !== saved;
  useEffect(() => onDirty(dirty), [dirty]); // eslint-disable-line react-hooks/exhaustive-deps
  return <div className="settings-row">
    <span className="settings-label">{t('settingsPage.expertDefault')} <ScopeTag scope="project" /></span>
    <span className="settings-inline">
      <label><input type="checkbox" checked={expert} disabled={busy || locked} onChange={event => setExpert(event.target.checked)} /> {t('settingsPage.expertStart')}</label>
      <SaveBar dirty={dirty} busy={busy} locked={locked} onSave={() => save({ expert_display_default: expert })} />
    </span>
  </div>;
}

export interface SettingsPageProps {
  value: Settings; profileId?: number; run: Run; busy: boolean; locked: boolean; replay: boolean;
  webAddr?: string; trap?: Status['trap']; switches: readonly SwitchSummary[]; groups: SwitchGroup[];
  theme: string; setTheme: (value: string) => void; notifications: boolean; setNotifications: (value: boolean) => void;
  setDialog: (value: string) => void; setReplay: Dispatch<SetStateAction<ReplaySession | null>>;
  // Reloads the settings (after a section is saved), the status (after the trap address changes) and the groups.
  onSaved: () => Promise<unknown>; refreshStatus: () => Promise<unknown>; refreshGroups: () => Promise<unknown>;
}

// One page instead of the former dialog: a table of contents, then each section with a tag saying where it applies.
// Shared settings are saved per section; unsaved changes are marked in the table of contents too.
export function SettingsPage(p: SettingsPageProps) {
  const [current, setCurrent] = useState<SectionId>('monitor');
  const [dirty, setDirty] = useState<Partial<Record<SectionId | 'expert', boolean>>>({});
  const mark = (id: SectionId | 'expert') => (value: boolean) => setDirty(old => old[id] === value ? old : { ...old, [id]: value });
  const save = (patch: Partial<Settings>) => p.run(async () => {
    const request = json('PUT', settingsBody(p.value, patch));
    const headers = new Headers(request.headers);
    if (p.profileId !== undefined) headers.set('X-AVLine-Profile', String(p.profileId));
    await api('/api/settings', { ...request, headers });
    await p.onSaved();
  });
  const go = (id: SectionId) => {
    setCurrent(id);
    document.getElementById(`settings-${id}`)?.scrollIntoView({ behavior: 'smooth', block: 'start' });
  };
  const sectionProps = { value: p.value, save, busy: p.busy, locked: p.locked };
  const listen = p.trap?.addr || '';
  const webPort = (p.webAddr || '').split(':').pop();
  const webShared = !!p.webAddr && !p.webAddr.startsWith('127.');
  return <div className="settings-page">
    <nav className="settings-toc" aria-label={t('settingsPage.toc')}>
      {sections.map(section => <button key={section.id} type="button" aria-current={current === section.id ? 'true' : undefined} onClick={() => go(section.id)}>
        {t(section.label)}{(dirty[section.id] || (section.id === 'display' && dirty.expert)) && <span className="settings-dot" aria-label={t('settingsPage.unsaved')}>●</span>}
      </button>)}
    </nav>
    <div className="settings-sections">
      <Card id="monitor" title="settingsPage.monitor" scope="project">
        <MonitorSection key={JSON.stringify([p.value.thresholds, p.value.setup_ping_interval_sec, p.value.rehearsal_ping_interval_sec])}
          {...sectionProps} onDirty={mark('monitor')} />
      </Card>
      <Card id="trap" title="settingsPage.trap" scope="app">
        {!p.replay && <TrapListenSetting trap={p.trap} busy={p.busy} locked={p.locked} run={p.run} onChanged={p.refreshStatus} onHelp={() => p.setDialog('trap')} />}
        {p.switches.length > 0 && <table className="settings-traps">
          <thead><tr><th>{t('settingsPage.switch')}</th><th>{t('device.lastTrap')}</th></tr></thead>
          <tbody>{p.switches.map(sw => <tr key={sw.id}><td>{sw.name} <span className="settings-hint">{sw.ip}</span></td>
            <td className={sw.last_trap_ts ? undefined : 'settings-warn'}>{sw.last_trap_ts ? pcTimestamp(sw.last_trap_ts, true) : t('device.noTrap')}</td></tr>)}</tbody>
        </table>}
        <details className="settings-details" open={p.switches.some(sw => !sw.last_trap_ts)}>
          <summary>{t('settingsPage.switchSetup')}</summary>
          <CopyCommand command={switchTrapCommands(listen)} />
          <p className="settings-hint">{t('operations.trapSwitchText')}</p>
        </details>
        <p><button type="button" onClick={() => p.setDialog('trap')}>{t('settingsPage.port162')}</button></p>
        {p.webAddr && <div className="settings-row"><span className="settings-label">{t('settingsPage.web')}</span>
          <span>{p.webAddr} <span className="settings-hint">{webShared ? t('settingsPage.webShared', { port: webPort || '' }) : t('rack.loopback')}</span></span></div>}
      </Card>
      <Card id="parts" title="settingsPage.parts" scope="project">
        <PartsSection key={JSON.stringify(p.value.views)} {...sectionProps} onDirty={mark('parts')} />
      </Card>
      <Card id="display" title="settingsPage.display">
        <BrowserDisplaySettings theme={p.theme} setTheme={p.setTheme} notifications={p.notifications} setNotifications={p.setNotifications} busy={p.busy} run={p.run} />
        <ExpertDefault key={String(p.value.expert_display_default)} {...sectionProps} onDirty={mark('expert')} />
        <p className="settings-hint">{t('settingsPage.browserHelp')}</p>
      </Card>
      <Card id="data" title="settingsPage.data" scope="project">
        <DataTransfers busy={p.busy} locked={p.locked} replay={p.replay} groups={p.groups} profileId={p.profileId} run={p.run}
          setDialog={p.setDialog} setReplay={p.setReplay} />
      </Card>
      <Card id="groups" title="settingsPage.groups" scope="project">
        <p className="settings-hint">{t('settingsPage.groupsHelp')}</p>
        <GroupOrder busy={p.busy} locked={p.locked} groups={p.groups} profileId={p.profileId} run={p.run} refresh={p.refreshGroups} />
      </Card>
    </div>
  </div>;
}
