import { eventTitle, getLang, t } from '../i18n';
import { asArray } from '../format';
import { frontPanelRows } from '../frontPanelLayout';
import type { Alert, ModelDefinitions, Port } from '../types';
import { lampState } from '../rackModel';
import { matchingPortAlerts } from '../wallAlerts';

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
  const english = getLang() === 'en';
  // Short numbers are unambiguous only when every slash-named port shares one prefix; stacks (1/0/1 and 2/0/1) keep full names.
  const prefixes = new Set(asArray(ports).filter(port => port.name.includes('/')).map(port => port.name.slice(0, port.name.lastIndexOf('/'))));
  const shortNames = prefixes.size <= 1;
  return (
    <div className="overflow-x-auto rounded bg-slate-700 p-3">
      {rows.map((row, index) => (
        <div key={index} className="mb-1">
          {row.label && <p className="text-sm text-white">{row.label}</p>}
          <div className="flex w-max gap-1">
            {row.slots.map((port, position) => {
              if (!port) return <span key={`gap-${position}`} className="w-20 shrink-0" aria-hidden="true" />;
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
              const description = [`${port.name} ${port.type}`, linkLabel, t(state.label), warningLabel, poeLabel]
                .filter(Boolean).join(' · ');
              const iconOnly = english && ((!port.excluded && (!linkUp || !reachable)) || Boolean(warningLabel || poeLabel));
              return (
                <button
                  key={port.if_index}
                  title={description}
                  aria-label={description}
                  aria-current={selected === port.if_index ? 'true' : undefined}
                  className={`hardware-port lamp-${state.tone} ${port.excluded ? 'excluded' : ''} w-20 shrink-0 px-1 text-sm`}
                  onClick={() => onSelect(port)}
                >
                  <span className="block whitespace-normal break-words">{shortName}</span>
                  <span className="flex items-center justify-center gap-0.5 text-xs" aria-hidden="true">
                    <span className="shrink-0">
                      {iconOnly ? (port.excluded ? '×' : !reachable ? '?' : linkUp ? '✓' : '—') : linkLabel}
                    </span>
                    {warningLabel && <span className="shrink-0">⚠</span>}
                    {poeLabel && <span className="shrink-0">{fault ? '⛔' : '⚡'}</span>}
                  </span>
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
