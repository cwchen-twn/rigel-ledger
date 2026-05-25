export async function apiFetch(url: string, options?: RequestInit): Promise<Response> {
  const response = await fetch(url, options);
  if (response.status === 401) {
    sessionStorage.setItem('redirectAfterLogin', window.location.pathname);
    window.location.href = '/login';
    throw new Error('Unauthorized');
  }
  return response;
}
