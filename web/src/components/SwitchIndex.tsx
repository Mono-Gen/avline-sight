import { t } from '../i18n';
import type { Alert } from '../types';
import { alertTone, hasCriticalActive, summarizeAlerts } from '../wallAlerts';

export type IndexSwitch = { id: number; name: string; group_tag: string; reachable: boolean };
type Tone = ReturnType<typeof alertTone>;

// State is told apart by shape, not colour alone: ongoing critical = round "!", ongoing warning = orange triangle,
// recovered/unacknowledged = yellow triangle, healthy = square, unknown = grey.
export function StateMark({ tone }: { tone: Tone }) {
  return <span className={`state-mark state-mark-${tone}`} aria-hidden="true">{tone === 'critical' ? '!' : ''}</span>;
}

// Group order: registered project groups first, other names alphabetically, ungrouped last (same rule as the rack).
export function orderedGroups(switches: readonly IndexSwitch[], known: readonly string[]) {
  const present = new Set(switches.map(sw => sw.group_tag));
  const groups = known.filter(name => present.has(name));
  groups.push(...[...present].filter(name => !known.includes(name)).sort());
  if (groups.includes('')) groups.push(groups.splice(groups.indexOf(''), 1)[0]);
  return groups;
}

// Index below "Port monitor". The heading row (fold toggle, fault marks) shows on every page; the list only on the port monitor.
export function SwitchIndex({ switches, groups, alerts, alertsKnown = true, focus, open, showList, onFocus, onToggle }: {
  switches: readonly IndexSwitch[];
  groups: readonly string[];
  alerts: readonly Alert[];
  // False when the alert list is not available (legacy replay): marks are hidden and switches shown as unknown.
  alertsKnown?: boolean;
  focus: number;
  open: boolean;
  showList: boolean;
  onFocus: (switchId: number) => void;
  onToggle: () => void;
}) {
  const total = summarizeAlerts([...alerts]);
  const recovered = total.count - total.active;
  const bySwitch = new Map<number, Alert[]>();
  for (const alert of alerts) {
    if (!bySwitch.has(alert.switch_id)) bySwitch.set(alert.switch_id, []);
    bySwitch.get(alert.switch_id)?.push(alert);
  }
  const expanded = open && showList;
  return <div className="switch-index">
    <div className="switch-index-head">
      {showList && <button type="button" className="switch-index-fold" aria-expanded={open}
        aria-label={t(open ? 'focus.foldIndex' : 'focus.openIndex')} onClick={onToggle}>{open ? '▾' : '▸'}</button>}
      <span className="switch-index-title">{t('focus.index')}</span>
      {/* Fault marks and counts stay visible when folded and on other pages. */}
      {alertsKnown && <span className="switch-index-marks" aria-label={t('focus.marks', { active: total.active, recovered })}>
        {total.active > 0 && <span className="switch-index-mark"><StateMark tone={hasCriticalActive(total) ? 'critical' : 'caution'} />{total.active}</span>}
        {recovered > 0 && <span className="switch-index-mark"><StateMark tone="warning" />{recovered}</span>}
      </span>}
    </div>
    {expanded && <ul className="switch-index-list">
      <li><button type="button" aria-current={focus === 0 ? 'true' : undefined} onClick={() => onFocus(0)}>
        <span className="state-mark state-mark-all" aria-hidden="true" />{t('focus.all')}</button></li>
      {orderedGroups(switches, groups).map(name => {
        const rows = switches.filter(sw => sw.group_tag === name);
        const groupCount = summarizeAlerts(rows.flatMap(sw => bySwitch.get(sw.id) || [])).count;
        return <li key={name || '-'}>
          <span className="switch-index-group">{name || t('rack.ungrouped')}{groupCount > 0 && ` (${groupCount})`}</span>
          <ul>{rows.map(sw => {
            const counts = summarizeAlerts(bySwitch.get(sw.id) || []);
            const tone = alertsKnown ? alertTone(counts, sw.reachable) : 'unknown';
            return <li key={sw.id}><button type="button" aria-current={focus === sw.id ? 'true' : undefined} onClick={() => onFocus(sw.id)}>
              <StateMark tone={tone} /><span className="switch-index-name">{sw.name}</span>
              {counts.count > 0 && <span className="switch-index-count">{counts.count}</span>}
            </button></li>;
          })}</ul>
        </li>;
      })}
    </ul>}
  </div>;
}
