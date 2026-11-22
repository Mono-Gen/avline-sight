import type { Dispatch, SetStateAction } from 'react';
import { useEffect, useRef } from 'react';
import { asArray } from './format';
import type { Event, Overview, Settings, SwitchState } from './types';
import { LiveSocket } from './ws';

import { LIVE_EVENT_LIMIT, mergeOverviewTick, pushRecentEvent } from './events';

export function useLiveFeed({ muted, replay, settings, setOverview, setState, setEvents, setOnline }: {
  muted: boolean;
  replay: boolean;
  settings?: Settings;
  setOverview: Dispatch<SetStateAction<Overview>>;
  setState: Dispatch<SetStateAction<SwitchState | undefined>>;
  setEvents: Dispatch<SetStateAction<Event[]>>;
  setOnline: Dispatch<SetStateAction<boolean>>;
}) {
  const socket = useRef<LiveSocket>();
  const preferences = useRef({ muted, replay, settings });
  preferences.current = { muted, replay, settings };
  useEffect(() => {
    const live = new LiveSocket(message => {
      if (['alerts_changed', 'phase_changed', 'work_changed', 'groups_changed', 'lock_changed'].includes(message.type)) {
        window.dispatchEvent(new CustomEvent('live-control', { detail: message }));
      }
      if (message.type === 'overview_tick') setOverview(previous => mergeOverviewTick(previous, message.data));
      if (message.type === 'switch_state') setState(message.data);
      if (message.type !== 'event_pushed') return;
      setEvents(old => [message.data, ...asArray(old).filter(item => item.id !== message.data.id)].slice(0, LIVE_EVENT_LIMIT));
      setOverview(previous => pushRecentEvent(previous, message.data));
      const current = preferences.current;
      if (message.data.severity !== 'CRITICAL' || current.replay || current.muted || !current.settings?.alert_sound) return;
      try {
        const context = new AudioContext();
        const oscillator = context.createOscillator();
        oscillator.connect(context.destination);
        oscillator.onended = () => { void context.close(); };
        oscillator.start();
        oscillator.stop(context.currentTime + 0.15);
      } catch {
        return;
      }
    }, setOnline);
    socket.current = live;
    return () => {
      live.close();
      socket.current = undefined;
    };
  }, [setEvents, setOnline, setOverview, setState]);
  return socket;
}
