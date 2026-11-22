import type { Dispatch, SetStateAction } from 'react';
import { useEffect, useRef, useState } from 'react';
import { asArray } from './format';
import type { Event, Overview, SwitchState } from './types';
import { LiveSocket } from './ws';
import { LIVE_EVENT_LIMIT, mergeOverviewTick, pushRecentEvent } from './events';

export function useLiveFeed({ setOverview, setState, setEvents, setOnline }: {
  setOverview: Dispatch<SetStateAction<Overview>>;
  setState: Dispatch<SetStateAction<SwitchState | undefined>>;
  setEvents: Dispatch<SetStateAction<Event[]>>;
  setOnline: Dispatch<SetStateAction<boolean>>;
}) {
  const socket = useRef<LiveSocket>();
  const [updatedAt, setUpdatedAt] = useState<number>();
  const [fresh, setFresh] = useState(false);
  useEffect(() => {
    const live = new LiveSocket(message => {
      if (['alerts_changed', 'phase_changed', 'groups_changed', 'lock_changed'].includes(message.type)) {
        window.dispatchEvent(new CustomEvent('live-control', { detail: message }));
      }
      if (message.type === 'overview_tick') {
        setOverview(previous => mergeOverviewTick(previous, message.data));
        setUpdatedAt(Date.now());
        setFresh(true);
      }
      if (message.type === 'switch_state') setState(message.data);
      if (message.type !== 'event_pushed') return;
      setEvents(old => [message.data, ...asArray(old).filter(item => item.id !== message.data.id)].slice(0, LIVE_EVENT_LIMIT));
      setOverview(previous => pushRecentEvent(previous, message.data));
    }, connected => {
      setOnline(connected);
      // Reconnection alone cannot make an old snapshot current; wait for a new overview tick.
      if (!connected) setFresh(false);
    });
    socket.current = live;
    return () => { live.close(); socket.current = undefined; };
  }, [setEvents, setOnline, setOverview, setState]);
  return { socket, updatedAt, fresh };
}
