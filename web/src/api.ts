import type {
  Envelope,
  FileData,
  ListResponse,
  LoginResponse,
  SharedFileData,
  TemplateDetail,
  TemplateFileData,
} from "./types";

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
  const envelope = await readEnvelope<T>(response);

  if (!response.ok) {
    throw new ApiError(response.status, reason(envelope, response.status));
  }

  return envelope.data as T;
}

// readEnvelope reads the envelope a response wears, or an empty one when the
// body is not JSON at all — a proxy or a crash can answer with something else,
// and a parse error must not hide the status code.
async function readEnvelope<T>(response: Response): Promise<Envelope<T>> {
  try {
    return (await response.json()) as Envelope<T>;
  } catch {
    return {};
  }
}

function reason(envelope: Envelope<unknown>, status: number): string {
  if (envelope.error !== undefined && envelope.error !== "") {
    return envelope.error;
  }
  return `The request failed (${status}).`;
}

// authHeaders carries the session token the guard reads. The files routes sit
// behind the same guard as the catalogue, so they all send it.
function authHeaders(): Headers {
  const headers = new Headers();
  const token = getToken();
  if (token !== null) {
    headers.set("Authorization", `Bearer ${token}`);
  }
  return headers;
}

export function login(email: string, password: string): Promise<LoginResponse> {
  return request<LoginResponse>("/login", {
    method: "POST",
    body: JSON.stringify({ email, password }),
  });
}

// signup creates the account. The API asks for the password twice — it refuses
// a mismatch itself — and takes no name: the users table has none.
export function signup(email: string, password: string, confirmPassword: string): Promise<void> {
  return request<void>("/signup", {
    method: "POST",
    body: JSON.stringify({
      email,
      password,
      confirm_password: confirmPassword,
    }),
  });
}

export function listTemplates(kind: string): Promise<ListResponse> {
  const query = kind === "" ? "" : `?kind=${encodeURIComponent(kind)}`;
  return request<ListResponse>(`/templates${query}`);
}

export function getTemplate(id: string): Promise<TemplateDetail> {
  return request<TemplateDetail>(`/templates/${encodeURIComponent(id)}`);
}

// saveDownload turns an archive or a file route's answer into a download under
// the given name. It cannot be a plain link: both routes sit behind the guard,
// so the request carries the Authorization header and the bytes arrive here.
async function saveDownload(response: Response, name: string): Promise<void> {
  if (!response.ok) {
    throw new ApiError(response.status, reason(await readEnvelope(response), response.status));
  }

  const url = window.URL.createObjectURL(await response.blob());
  const anchor = document.createElement("a");
  anchor.href = url;
  anchor.download = name;
  document.body.appendChild(anchor);
  anchor.click();
  anchor.remove();

  // The revoke waits a turn of the event loop: revoking in the same tick as the
  // click can cancel a download that has not started yet.
  window.setTimeout(() => window.URL.revokeObjectURL(url), 0);
}

// downloadTemplateArchive fetches the whole template as a zip and hands it to
// the browser.
export async function downloadTemplateArchive(template: TemplateDetail): Promise<void> {
  const response = await fetch(`/v1/templates/${encodeURIComponent(template.id)}/archive`, {
    headers: authHeaders(),
  });

  await saveDownload(response, `${archiveNameOf(template)}.zip`);
}

// downloadTemplateFile fetches one file of a template. The detail page uses it
// for a binary file: the bytes are not inline, and a plain link would not
// carry the token the guard wants.
export async function downloadTemplateFile(
  templateID: string,
  file: TemplateFileData,
): Promise<void> {
  const response = await fetch(
    `/v1/templates/${encodeURIComponent(templateID)}/files/${encodeURIComponent(file.id)}/content`,
    { headers: authHeaders() },
  );

  const parts = file.path.split("/");
  await saveDownload(response, parts[parts.length - 1] ?? file.path);
}

// archiveNameOf is the name the zip is saved under: the template's slug when it
// has one, its name otherwise, with anything that is not safe in a filename
// collapsed to a dash.
export function archiveNameOf(template: TemplateDetail): string {
  const source = template.slug !== "" ? template.slug : template.name;
  const safe = source.replace(/[^a-zA-Z0-9._-]+/g, "-").replace(/^-+|-+$/g, "");
  return safe === "" ? "template" : safe;
}

// The most the API takes in one upload: it caps the body with
// http.MaxBytesReader at 32 MiB, so the browser refuses first and says why.
export const MAX_UPLOAD_BYTES = 32 * 1024 * 1024;

// fileNameOf is the name the file was uploaded under. The service splits
// "notes.md" into the name "notes" and the extension "md", so the dot goes
// back in.
export function fileNameOf(file: FileData): string {
  return file.ext === "" ? file.name : `${file.name}.${file.ext}`;
}

// listFiles is the caller's own uploads. `/files` answers the same list, and
// `/files/me` is the alias that names them explicitly.
export function listFiles(): Promise<ListResponse<FileData>> {
  return request<ListResponse<FileData>>("/files/me");
}

export function listSharedFiles(): Promise<ListResponse<SharedFileData>> {
  return request<ListResponse<SharedFileData>>("/files/shared");
}

export async function shareFile(id: string, isPublic: boolean): Promise<void> {
  const response = await fetch(`/v1/files/${encodeURIComponent(id)}/share`, {
    method: "POST",
    headers: authHeaders(),
    body: JSON.stringify({ is_public: isPublic }),
  });

  if (!response.ok) {
    throw new ApiError(response.status, reason(await readEnvelope(response), response.status));
  }
}

export async function uploadFile(file: File): Promise<void> {
  const form = new FormData();
  form.append("file", file);

  // No Content-Type here: the browser sets it with the multipart boundary.
  const response = await fetch("/v1/files", {
    method: "POST",
    headers: authHeaders(),
    body: form,
  });

  if (!response.ok) {
    throw new ApiError(response.status, reason(await readEnvelope(response), response.status));
  }
}

// downloadFile fetches the bytes and hands them to the browser as a download.
// It cannot be a plain link: the route is behind the guard, so the request
// needs the Authorization header.
export async function downloadFile(file: FileData): Promise<void> {
  const response = await fetch(`/v1/files/${encodeURIComponent(file.id)}/content`, {
    headers: authHeaders(),
  });

  await saveDownload(response, fileNameOf(file));
}

export async function downloadFileShared(file: SharedFileData): Promise<void> {
  const response = await fetch(`/v1/files/${encodeURIComponent(file.id)}/content`, {
    headers: authHeaders(),
  });

  await saveDownload(response, fileNameOf(file as unknown as FileData));
}

export function deleteFile(id: string): Promise<void> {
  return request<void>(`/files/${encodeURIComponent(id)}`, { method: "DELETE" });
}
