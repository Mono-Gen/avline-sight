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
              // 他のアラートが発生中でも、未確認のPoE障害は隠さない（発生中を優先）
              const isPoeFault = (alert: Alert) => alert.code === 'POE_PORT_FAULT' || alert.code === 'POE_FAULT';
              const fault = state.issues.find(alert => alert.active && isPoeFault(alert)) ?? state.issues.find(isPoeFault);
              return (
                <button
                  key={port.if_index}
                  title={`${port.name} ${port.type} · ${t(state.label)}`}
                  aria-current={selected === port.if_index ? 'true' : undefined}
                  className={`hardware-port lamp-${state.tone} ${port.excluded ? 'excluded' : ''} w-20 shrink-0 text-sm`}
                  onClick={() => onSelect(port)}
                >
                  {state.symbol} {port.name}
                  <br />
                  {port.excluded ? t('ui.m133') : port.oper === 'up' ? t('ui.m134') : t('ui.m135')}
                  {state.issues.length > 0 && <span title={t(state.label)}> {t('ui.m137')}</span>}
                  {fault
                    ? <span title={eventTitle(fault)}> {t('ui.m138')}</span>
                    : (port.poe?.power_w || 0) > 0 && <span title={t('ui.m139')}> ⚡</span>}
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
