// The JSON shapes the API speaks. They mirror internal/domain/templates/dto.go,
// internal/domain/users/dto.go and internal/domain/files/dto.go: the field names
// below are the wire contract, so a rename on the Go side has to happen here
// too.

export interface TemplateData {
  id: string;
  name: string;
  slug: string;
  description: string;
  kind: string;
  version: number;
  is_public: boolean;
  owner: string;
  owner_email: string;
  file_count: number;
  created_at: string;
  updated_at: string;
}

// A file of a template. A text file carries content; a binary file carries
// its size and no content — the bytes live on the server and are fetched from
// the file's own route.
export interface TemplateFileData {
  id: string;
  path: string;
  kind?: string;
  size?: number;
  content?: string;
  content_base64?: string;
  is_executable: boolean;
}

export interface TemplateVariableData {
  id: string;
  key: string;
  prompt: string;
  default_value: string;
  required: boolean;
  position: number;
}

export interface TemplateDetail extends TemplateData {
  files: TemplateFileData[];
  variables: TemplateVariableData[];
}

// One entry of the files catalogue. It mirrors internal/domain/files/dto.go:
// the files resource sends the id, the name, the extension and the version —
// the path and the content type stay on the server.
export interface FileData {
  id: string;
  name: string;
  ext: string;
  version: number;
  is_public: boolean;
}

export interface SharedFileData {
  id: string;
  name: string;
  ext: string;
  version: number;
  is_public: boolean;
  owner: string;
  owner_email: string;
}

export interface ListResponse<T = TemplateData> {
  data: T[];
  total: number;
}

export interface LoginResponse {
  token: string;
}

// The envelope every API response wears: pkg/types.Envelope.
export interface Envelope<T> {
  status_code?: number;
  data?: T;
  message?: string;
  error?: string;
  timestamp?: string;
}

// The kinds a template may carry. The API enforces the same list with a
// CHECK constraint, so a kind outside this list never reaches the catalogue.
export const KINDS = ["opencode", "codex", "copilot", "cursor", "windsurf", "generic"] as const;

export type Kind = (typeof KINDS)[number];
