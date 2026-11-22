import { getLang, t } from './i18n';
export const DEFAULT_TIMEOUT_MS = 15000;
class ApiError extends Error {}

async function consumeResponse<T>(path: string, options: RequestInit, consume: (response: Response) => Promise<T>, timeoutMs = DEFAULT_TIMEOUT_MS): Promise<T> {
  const controller = new AbortController();
  const abort = () => controller.abort();
  if (options.signal?.aborted) abort();
  options.signal?.addEventListener('abort', abort, { once: true });
  const timer = window.setTimeout(() => controller.abort(), timeoutMs);
  try {
    const headers = new Headers(options.headers);
    headers.set('Accept-Language', getLang());
    const r = await fetch(path, {
      ...options,
      headers,
      signal: controller.signal
    });
    if (!r.ok) {
      let reason = `HTTP ${r.status}`;
      try {
        const b = await r.json() as {
          error?: string;
        };
        reason = b.error || reason;
      } catch {
        reason = `HTTP ${r.status}`;
      }
      throw new ApiError(t('rack.error', { reason }));
    }
    return await consume(r);
  } finally {
    clearTimeout(timer);
    options.signal?.removeEventListener('abort', abort);
  }
}
export function response(path: string, options: RequestInit = {}): Promise<Response> {
  return consumeResponse(path, options, async result => result);
}
export function responseBlob(path: string): Promise<Blob> {
  return consumeResponse(path, {}, result => result.blob(), 35000);
}

export async function api<T>(path: string, options: RequestInit = {}, timeoutMs = DEFAULT_TIMEOUT_MS): Promise<T> {
  try {
    return await consumeResponse(path, options, async result => {
      if (result.status === 204) return undefined as T;
      const text = await result.text();
      return (text ? JSON.parse(text) : undefined) as T;
    }, timeoutMs);
  } catch (error) {
    if (!options.signal?.aborted) window.dispatchEvent(new CustomEvent('api-error', { detail: message(error) }));
    throw error;
  }
}
export const json = (method: string, body?: unknown): RequestInit => ({
  method,
  headers: { 'Content-Type': 'application/json' },
  body: body === undefined ? undefined : JSON.stringify(body)
});
export const wsUrl = (path: string) => `${location.protocol === 'https:' ? 'wss:' : 'ws:'}//${location.host}${path}`;
export function download(blob: Blob, name: string) {
  const url = URL.createObjectURL(blob);
  const a = document.createElement('a');
  a.href = url;
  a.download = name;
  a.click();
  setTimeout(() => URL.revokeObjectURL(url), 1000);
}
export function upload(file: File) {
  const f = new FormData();
  f.append('file', file);
  return {
    method: 'POST',
    body: f
  };
}
export function message(e: unknown) {
  const reason = e instanceof Error ? e.message : t('rack.unknown');
  return e instanceof ApiError ? reason : t('rack.error', { reason });
}
