// The JSON shapes the API speaks. They mirror internal/domain/templates/dto.go
// and internal/domain/users/dto.go: the field names below are the wire
// contract, so a rename on the Go side has to happen here too.

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

export interface TemplateFileData {
  id: string;
  path: string;
  content: string;
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

export interface ListResponse {
  data: TemplateData[];
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
