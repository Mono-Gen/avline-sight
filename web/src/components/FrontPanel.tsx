import { t } from '../i18n';
import { frontPanelRows } from '../frontPanelLayout';
import type { ModelDefinitions, Port } from '../types';

export function portHasAlert(port: Port): boolean {
  return [port.err_delta, port.discard_delta, port.fcs_delta, port.cos_drop_delta, port.err_since_reset]
    .some(value => value > 0);
}

export function poeHasFault(port: Port): boolean {
  const fault = port.poe?.fault?.trim().toLowerCase();
  return Boolean(fault && fault !== 'none');
}

export function FrontPanel({ ports, selected, onSelect, model, definitions }: {
  ports: Port[];
  model?: string;
  definitions?: ModelDefinitions;
  selected: number;
  onSelect: (port: Port) => void;
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
              const fault = poeHasFault(port);
              const alert = portHasAlert(port);
              const color = alert || fault
                ? '!bg-red-200'
                : port.oper === 'up' ? '!bg-green-200' : '!bg-slate-300';
              return (
                <button
                  key={port.if_index}
                  title={`${port.name} ${port.type}`}
                  aria-pressed={selected === port.if_index}
                  className={`${color} ${port.excluded ? 'excluded' : ''} w-20 shrink-0 text-sm`}
                  onClick={() => onSelect(port)}
                >
                  {port.type === 'sfp' ? '◈' : port.type === 'lag' ? '≡' : '▣'} {port.name}
                  <br />
                  {port.excluded ? t('ui.m133') : port.oper === 'up' ? t('ui.m134') : t('ui.m135')}
                  {alert && <span title={t('ui.m136')}> {t('ui.m137')}</span>}
                  {fault
                    ? <span className="text-red-800" title={port.poe?.fault}> {t('ui.m138')}</span>
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
