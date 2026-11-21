import type React from 'react';
import type { Event, Overview, Settings, Status, SwitchState, WatchItem } from '../types';
export interface ViewProps {
  overview: Overview;
  status?: Status;
  settings?: Settings;
  part: string;
  selected: number;
  select: (id: number) => void;
  state?: SwitchState;
  replay: SwitchState[] | null;
  events: Event[];
  setEvents: React.Dispatch<React.SetStateAction<Event[]>>;
  run: (f: () => Promise<void>) => void;
  busy: boolean;
  locked: boolean;
  openTerminal: (id: number) => void;
  watch: WatchItem[];
  setWatch: (w: WatchItem[]) => void;
}
