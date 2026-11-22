import type React from 'react';
import { asArray } from '../format';
import type { Event, Overview, ReplayRef, Settings, Status, SwitchState, WatchItem } from '../types';
import type { Operations } from '../useOperations';
export interface PortMonitorTarget {
  switchId: number;
  ifIndex?: number;
  portName?: string;
}
export interface ViewProps {
  overview: Overview;
  operations?: Operations;
  status?: Status;
  settings?: Settings;
  part: string;
  group?: string;
  selected: number;
  select: (id: number) => void;
  openPortMonitor?: (target: PortMonitorTarget) => void;
  state?: SwitchState;
  replay: SwitchState[] | null;
  replayRef?: ReplayRef;
  events: Event[];
  setEvents: React.Dispatch<React.SetStateAction<Event[]>>;
  run: (f: () => Promise<void>) => void;
  busy: boolean;
  locked: boolean;
  openTerminal: (id: number) => void;
  watch: WatchItem[];
  setWatch: (w: WatchItem[]) => void;
}

type ViewScope = Pick<ViewProps, 'group' | 'part' | 'settings' | 'replay' | 'overview'>;

export function groupSwitches({ group, part, settings, replay, overview }: ViewScope) {
  const tags = asArray(asArray(settings?.views).find(view => view.id === part)?.tags);
  return asArray(replay ?? overview.switches).filter(sw =>
    (group === undefined || sw.group_tag === group) && (!tags.length || tags.includes(sw.group_tag)));
}

export function hasGroupFilter({ group, part, settings }: ViewScope) {
  return group !== undefined || !!asArray(asArray(settings?.views).find(view => view.id === part)?.tags).length;
}

export function inGroupScope(row: { switch_id?: number }, visibleIds: ReadonlySet<number>, filtered: boolean) {
  return !filtered || !row.switch_id || visibleIds.has(row.switch_id);
}
