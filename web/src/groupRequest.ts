import { json } from './api';

export function groupRequest(profileId: number | undefined, method: string, body?: unknown): RequestInit {
  const options = json(method, body);
  const headers = new Headers(options.headers);
  if (profileId !== undefined) headers.set('X-AVLine-Profile', String(profileId));
  return { ...options, headers };
}
