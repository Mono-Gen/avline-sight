import type React from 'react';
import { eventTitle, t } from '../i18n';
import { lampStates } from './RackRow';
import { asArray } from '../format';
import { frontPanelRows } from '../frontPanelLayout';
import type { Alert, ModelDefinitions, Port } from '../types';
import { lampState } from '../rackModel';
import { matchingPortAlerts } from '../wallAlerts';
import { portMarks } from './PortVlan';

export function FrontPanel({ ports, selected, onSelect, model, definitions, alerts = [], switchId = 0, reachable = true, showNote = false }: {
  ports: Port[];
  model?: string;
  definitions?: ModelDefinitions;
  selected: number;
  onSelect: (port: Port) => void;
  alerts?: Alert[];
  switchId?: number;
  reachable?: boolean;
  // The built-in definition note is technical; only the expert display shows it.
  showNote?: boolean;
}) {
  const definition = model ? definitions?.[model] : undefined;
  const rows = frontPanelRows(ports, definition);
  // Short numbers are unambiguous only when every slash-named port shares one prefix; stacks (1/0/1 and 2/0/1) keep full names.
  const prefixes = new Set(asArray(ports).filter(port => port.name.includes('/')).map(port => port.name.slice(0, port.name.lastIndexOf('/'))));
  const shortNames = prefixes.size <= 1;
  // One column count for every row (the longest defined row), so columns line up and an extra row wraps in the same grid.
  const defined = rows.filter(row => !row.label);
  const columns = Math.max(1, ...(defined.length ? defined : rows).map(row => row.slots.length));
  return (
    <div className="front-panel rounded bg-slate-700 p-3">
      {rows.filter(row => row.slots.length > 0).map((row, index) => (
        <div key={index} className="mb-1">
          {row.label && <p className="text-sm text-white">{row.label}</p>}
          <div className="front-row" style={{ '--slots': columns } as React.CSSProperties}>
            {row.slots.map((port, position) => {
              if (!port) return <span key={`gap-${position}`} aria-hidden="true" />;
              const state = lampState(port, matchingPortAlerts(alerts, switchId, port.if_index), reachable);
              // Keep an unacked PoE fault visible even when another alert is active (active first).
              const isPoeFault = (alert: Alert) => alert.code === 'POE_PORT_FAULT' || alert.code === 'POE_FAULT';
              const fault = state.issues.find(alert => alert.active && isPoeFault(alert)) ?? state.issues.find(isPoeFault);
              const shortName = shortNames ? port.name.slice(port.name.lastIndexOf('/') + 1) || port.name : port.name;
              const linkUp = port.oper === 'up';
              // For an unreachable switch, show the lamp state in words instead of the last known link state.
              const linkLabel = port.excluded ? t('ui.m133') : !reachable ? t(state.label) : linkUp ? t('ui.m134') : t('ui.m135');
              const warningLabel = state.issues.length > 0 ? t('ui.m137') : '';
              const powered = (port.poe?.power_w || 0) > 0;
              const poeLabel = fault ? `${t('ui.m138')} · ${eventTitle(fault)}` : powered ? t('ui.m139') : '';
              const marks = portMarks(port);
              const description = [`${port.name} ${port.type}`, linkLabel, t(state.label), warningLabel, poeLabel, ...marks.map(m => m.label)]
                .filter(Boolean).join(' · ');
              // Same visual language as the all-switches rack: number + state symbol; the words are in the tooltip and legend.
              const tone = state.label === 'rack.unused' && port.oper === 'down' ? 'unused' : state.tone;
              // An excluded port shows ×, but an alert still on it keeps its ! or ▲.
              const symbol = port.excluded && !['critical', 'caution', 'warning'].includes(tone) ? '×' : lampStates[tone as keyof typeof lampStates]?.symbol ?? '?';
              return (
                <button
                  key={port.if_index}
                  title={description}
                  aria-label={description}
                  aria-current={selected === port.if_index ? 'true' : undefined}
                  className={`hardware-port lamp-${tone} ${port.excluded ? 'excluded' : ''} text-sm`}
                  onClick={() => onSelect(port)}
                >
                  <span className="front-port-number">{shortName}</span>
                  <span className="front-port-marks" aria-hidden="true">
                    <span>{symbol}</span>
                    {poeLabel && <span>{fault ? '⛔' : '⚡'}</span>}
                  </span>
                  {marks.length > 0 && <span className={`port-role-marks${marks.some(m => m.mark === 'B') ? ' port-role-blocked' : ''}`} aria-hidden="true">{marks.map(m => m.mark).join(' ')}</span>}
                </button>
              );
            })}
          </div>
        </div>
      ))}
      {showNote && definition?.note && <p className="text-sm text-white">{definition.note}</p>}
    </div>
  );
}
