import type React from 'react';
import { asArray } from '../format';
import type { Event, Overview, ReplayRef, Settings, Status, SwitchState, WatchItem } from '../types';
import type { Operations } from '../useOperations';
export interface PortMonitorTarget {
  switchId: number;
  ifIndex?: number;
  portName?: string;
}
// Logs opened from a port: that switch and port only, all severities.
export interface LogTarget {
  switchId: number;
  ifIndex: number;
  switchName: string;
  portName: string;
}
export interface SwitchFilter {
  group?: string;
  part: string;
  switchId?: number;
}

export const emptySwitchFilter: SwitchFilter = { part: '' };

export interface ViewProps extends SwitchFilter {
  overview: Overview;
  operations?: Operations;
  status?: Status;
  settings?: Settings;
  selected: number;
  select: (id: number) => void;
  openPortMonitor?: (target: PortMonitorTarget) => void;
  openLogs?: (target: LogTarget) => void;
  openDevices?: () => void;
  logTarget?: LogTarget;
  clearLogTarget?: () => void;
  state?: SwitchState;
  replay: SwitchState[] | null;
  replayRef?: ReplayRef;
  events: Event[];
  setEvents: React.Dispatch<React.SetStateAction<Event[]>>;
  run: (f: () => Promise<void>) => void;
  busy: boolean;
  locked: boolean;
  expert?: boolean;
  openTerminal?: (id: number) => void;
  watch: WatchItem[];
  setWatch: (w: WatchItem[]) => void;
}

type ViewScope = Pick<ViewProps, 'group' | 'part' | 'switchId' | 'settings' | 'replay' | 'overview'>;

export function partPreset({ settings, part }: Pick<ViewScope, 'settings' | 'part'>) {
  return asArray(settings?.views).find(view => view.id === part);
}

export function matchesSwitch(filter: SwitchFilter, tags: readonly string[], id: number | undefined, group: string) {
  return !id || ((filter.switchId === undefined || filter.switchId === id) &&
    (filter.group === undefined || filter.group === group) && (!tags.length || tags.includes(group)));
}

export function groupSwitches(scope: ViewScope) {
  const tags = asArray(partPreset(scope)?.tags);
  return asArray(scope.replay ?? scope.overview?.switches).filter(sw => matchesSwitch(scope, tags, sw.id, sw.group_tag));
}

export function hasGroupFilter(scope: SwitchFilter) {
  return scope.group !== undefined || !!scope.part || scope.switchId !== undefined;
}

export function inGroupScope(row: { switch_id?: number }, visibleIds: ReadonlySet<number>, filtered: boolean) {
  return !filtered || !row.switch_id || visibleIds.has(row.switch_id);
}
