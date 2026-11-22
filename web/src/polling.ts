interface PollOptions<T> {
  load: (signal: AbortSignal) => Promise<T>;
  onData: (value: T) => void;
  onError: (error: unknown) => void;
  onStart?: () => void;
  onFinish?: () => void;
  intervalMs: number;
  delayMs?: number;
}

export function createPoller<T>(options: PollOptions<T>) {
  let active = true;
  let pending: Promise<void> | null = null;
  let controller: AbortController | undefined;
  const refresh = (): Promise<void> => {
    
    if (!active) return Promise.resolve();
    if (pending) return pending;
    const request = new AbortController();
    controller = request;
    options.onStart?.();
    pending = Promise.resolve()
      .then(() => options.load(request.signal))
      .then(value => {
        if (active && !request.signal.aborted) options.onData(value);
      })
      .catch(error => {
        if (active && !request.signal.aborted) {
          options.onError(error);
          throw error;
        }
      })
      .finally(() => {
        pending = null;
        if (active) options.onFinish?.();
      });
    return pending;
  };
  const scheduledRefresh = () => { void refresh().catch(() => undefined); };
  const initialTimer = options.delayMs ? setTimeout(scheduledRefresh, options.delayMs) : undefined;
  const interval = options.intervalMs > 0 ? setInterval(scheduledRefresh, options.intervalMs) : undefined;
  if (!initialTimer) scheduledRefresh();
  return {
    refresh,
    stop() {
      active = false;
      clearTimeout(initialTimer);
      clearInterval(interval);
      controller?.abort();
    },
  };
}
