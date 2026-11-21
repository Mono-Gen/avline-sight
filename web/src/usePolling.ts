import type { DependencyList } from 'react';
import { useCallback, useEffect, useRef, useState } from 'react';
import { message } from './api';
import { createPoller } from './polling';

export const STATE_REFRESH_MS = 5000;
export const METRICS_REFRESH_MS = 10000;
export const TOPOLOGY_REFRESH_MS = 15000;

export function usePolling<T>(
  load: (signal: AbortSignal) => Promise<T>,
  intervalMs: number,
  dependencies: DependencyList = [],
  enabled = true,
  delayMs = 0,
) {
  const [data, setData] = useState<T>();
  const [pending, setPending] = useState(false);
  const [error, setError] = useState('');
  const [updatedAt, setUpdatedAt] = useState<Date>();
  const loader = useRef(load);
  loader.current = load;
  const poller = useRef<ReturnType<typeof createPoller<T>>>();
  useEffect(() => {
    setData(undefined);
    setError('');
    setPending(false);
    setUpdatedAt(undefined);
    if (!enabled) return;
    let current: ReturnType<typeof createPoller<T>> | undefined;
    const stop = () => {
      current?.stop();
      if (poller.current === current) poller.current = undefined;
      current = undefined;
    };
    const start = () => {
      if (current || (typeof document !== 'undefined' && document.hidden)) return;
      current = createPoller<T>({
        load: signal => loader.current(signal),
        onData: value => {
          setData(value);
          setError('');
          setUpdatedAt(new Date());
        },
        onError: reason => setError(message(reason)),
        onStart: () => setPending(true),
        onFinish: () => setPending(false),
        intervalMs,
        delayMs,
      });
      poller.current = current;
    };
    const visibility = () => {
      if (document.hidden) {
        stop();
        setPending(false);
      } else start();
    };
    start();
    document.addEventListener('visibilitychange', visibility);
    return () => {
      document.removeEventListener('visibilitychange', visibility);
      stop();
    };
  }, [enabled, intervalMs, delayMs, ...dependencies]);
  const refresh = useCallback(() => poller.current?.refresh() || Promise.resolve(), []);
  return { data, pending, error, updatedAt, refresh };
}
