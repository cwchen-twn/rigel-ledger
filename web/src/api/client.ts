export async function apiFetch(url: string, options?: RequestInit): Promise<Response> {
  const response = await fetch(url, options);
  if (response.status === 401) {
    sessionStorage.setItem('redirectAfterLogin', window.location.pathname);
    window.location.href = '/login';
    throw new Error('Unauthorized');
  }
  return response;
}

// Parses a structured API error response {"error": "CODE"} and returns the code.
// Falls back to 'UNKNOWN_ERROR' if the response is not structured JSON.
export async function getApiError(resp: Response): Promise<string> {
  try {
    const data = await resp.json() as { error?: string };
    return data.error ?? 'UNKNOWN_ERROR';
  } catch {
    return 'UNKNOWN_ERROR';
  }
}
