export class ApiError extends Error {
  constructor(
    public status: number,
    public code: string,
    message: string,
  ) {
    super(message);
    this.name = "ApiError";
  }
}

const SESSION_EXPIRED = "espm:session-expired";

export function onSessionExpired(fn: () => void) {
  window.addEventListener(SESSION_EXPIRED, fn);
  return () => window.removeEventListener(SESSION_EXPIRED, fn);
}

async function request<T>(method: string, url: string, body?: unknown): Promise<T> {
  const res = await fetch(url, {
    method,
    credentials: "same-origin",
    headers: body === undefined ? undefined : { "Content-Type": "application/json" },
    body: body === undefined ? undefined : JSON.stringify(body),
  });

  if (res.status === 401) {
    window.dispatchEvent(new Event(SESSION_EXPIRED));
  }
  if (!res.ok) {
    let code = "internal";
    let message = res.statusText;
    try {
      const j = await res.json();
      code = j.error ?? code;
      message = j.message ?? j.error ?? message;
    } catch {
      // non-JSON error body
    }
    throw new ApiError(res.status, code, message);
  }
  if (res.status === 204) return undefined as T;
  return (await res.json()) as T;
}

export const api = {
  get: <T>(url: string) => request<T>("GET", url),
  post: <T>(url: string, body?: unknown) => request<T>("POST", url, body),
  put: <T>(url: string, body?: unknown) => request<T>("PUT", url, body),
  del: <T>(url: string) => request<T>("DELETE", url),
};
