export interface MeResponse {
  username: string;
  access_token_left_time: number;
}

export async function fetchMe(): Promise<MeResponse | null> {
  try {
    const resp = await fetch('/api/me');
    if (resp.ok) return resp.json() as Promise<MeResponse>;
    return null;
  } catch {
    return null;
  }
}

export async function login(username: string, password: string): Promise<string> {
  const body = new URLSearchParams({ username, password });
  const resp = await fetch('/login', {
    method: 'POST',
    headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
    body,
  });
  if (resp.ok) {
    const data = await resp.json() as { username: string };
    return data.username;
  }
  const text = await resp.text();
  throw new Error(text || 'Login failed');
}

export async function refreshToken(): Promise<number> {
  const resp = await fetch('/refresh-token', { method: 'POST' });
  if (!resp.ok) throw new Error('Failed to refresh token');
  const data = await resp.json() as { access_token_left_time: number };
  return data.access_token_left_time;
}
