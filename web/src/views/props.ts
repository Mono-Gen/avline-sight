import type React from 'react';
import type { Event, Overview, ReplayRef, Settings, Status, SwitchState, WatchItem } from '../types';
import type { Operations } from '../useOperations';
export interface ViewProps {
  overview: Overview;
  operations?: Operations;
  status?: Status;
  settings?: Settings;
  part: string;
  selected: number;
  select: (id: number) => void;
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
