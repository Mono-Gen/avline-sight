import { eventTitle, t } from '../i18n';
import { frontPanelRows } from '../frontPanelLayout';
import type { Alert, ModelDefinitions, Port } from '../types';
import { lampState } from '../rackModel';
import { matchingPortAlerts } from '../wallAlerts';

export function FrontPanel({ ports, selected, onSelect, model, definitions, alerts = [], switchId = 0, reachable = true }: {
  ports: Port[];
  model?: string;
  definitions?: ModelDefinitions;
  selected: number;
  onSelect: (port: Port) => void;
  alerts?: Alert[];
  switchId?: number;
  reachable?: boolean;
}) {
  const definition = model ? definitions?.[model] : undefined;
  const rows = frontPanelRows(ports, definition);
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
              const linkLabel = port.excluded ? t('ui.m133') : port.oper === 'up' ? t('ui.m134') : t('ui.m135');
              return (
                <button
                  key={port.if_index}
                  title={`${port.name} ${port.type} · ${linkLabel} · ${t(state.label)}`}
                  aria-current={selected === port.if_index ? 'true' : undefined}
                  className={`hardware-port lamp-${state.tone} ${port.excluded ? 'excluded' : ''} w-20 shrink-0 text-sm`}
                  onClick={() => onSelect(port)}
                >
                  <span className="block truncate">{state.symbol} {port.name}</span>
                  <span className="flex items-center justify-center gap-0.5">
                    <span className="min-w-0 truncate">{linkLabel}</span>
                    {state.issues.length > 0 && <span className="shrink-0" title={t(state.label)}>{t('ui.m137')}</span>}
                    {fault
                      ? <span className="shrink-0" title={eventTitle(fault)}>{t('ui.m138')}</span>
                      : (port.poe?.power_w || 0) > 0 && <span className="shrink-0" title={t('ui.m139')}>⚡</span>}
                  </span>
                </button>
              );
            })}
          </div>
        </div>
      ))}
      {definition?.note && <p className="text-sm text-white">{definition.note}</p>}
    </div>
  );
}
