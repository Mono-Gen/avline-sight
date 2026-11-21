export async function response(path: string, options: RequestInit = {}): Promise<Response> {
  const controller = new AbortController();
  const timer = window.setTimeout(() => controller.abort(), 15000);
  try {
    const r = await fetch(path, {
      ...options,
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
        /* HTTP状態を使用する。 */
      }
      throw new Error(`操作に失敗しました（${reason}）。接続や入力を確認して再試行してください。`);
    }
    return r;
  } finally {
    clearTimeout(timer);
  }
}
export async function api<T>(path: string, options: RequestInit = {}): Promise<T> {
  try {
    const r = await response(path, options);
    if (r.status === 204) return undefined as T;
    const text = await r.text();
    return (text ? JSON.parse(text) : undefined) as T;
  } catch (e) {
    window.dispatchEvent(new CustomEvent('api-error', { detail: message(e) }));
    throw e;
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
  return e instanceof Error && e.message.includes('操作に失敗しました') ? e.message : `操作に失敗しました（${e instanceof Error ? e.message : '原因不明'}）。接続や入力を確認して再試行してください。`;
}
