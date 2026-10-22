import type { Envelope, ListResponse, LoginResponse, TemplateDetail } from "./types";

const TOKEN_KEY = "eznit.token";

// The token lives in localStorage. The catalogue and the detail page are
// behind the session guard, so the token is the only thing the API asks for.
export function getToken(): string | null {
  return window.localStorage.getItem(TOKEN_KEY);
}

export function setToken(token: string): void {
  window.localStorage.setItem(TOKEN_KEY, token);
}

export function clearToken(): void {
  window.localStorage.removeItem(TOKEN_KEY);
}

export class ApiError extends Error {
  readonly status: number;

  constructor(status: number, message: string) {
    super(message);
    this.name = "ApiError";
    this.status = status;
  }
}

// messageOf turns any thrown value into something a page can show.
export function messageOf(cause: unknown): string {
  if (cause instanceof Error) {
    return cause.message;
  }
  return String(cause);
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const headers = new Headers(init?.headers);
  headers.set("Accept", "application/json");
  if (init?.body !== undefined) {
    headers.set("Content-Type", "application/json");
  }

  const token = getToken();
  if (token !== null) {
    headers.set("Authorization", `Bearer ${token}`);
  }

  const response = await fetch(`/v1${path}`, { ...init, headers });

  // The API always answers with the envelope, but a proxy or a crash can
  // answer with something else, and a JSON parse error must not hide the
  // status code.
  let envelope: Envelope<T> = {};
  try {
    envelope = (await response.json()) as Envelope<T>;
  } catch {
    envelope = {};
  }

  if (!response.ok) {
    throw new ApiError(
      response.status,
      envelope.error ?? `The request failed (${response.status}).`,
    );
  }

  return envelope.data as T;
}

export function login(email: string, password: string): Promise<LoginResponse> {
  return request<LoginResponse>("/login", {
    method: "POST",
    body: JSON.stringify({ email, password }),
  });
}

export function listTemplates(kind: string): Promise<ListResponse> {
  const query = kind === "" ? "" : `?kind=${encodeURIComponent(kind)}`;
  return request<ListResponse>(`/templates${query}`);
}

export function getTemplate(id: string): Promise<TemplateDetail> {
  return request<TemplateDetail>(`/templates/${encodeURIComponent(id)}`);
}

export async function signup(email: string, password: string, name: string): Promise<void> {
  const response = await fetch(`${API}/v1/signup`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ email, password, name }),
  });
  if (!response.ok) {
    throw await parseError(response);
  }
}
