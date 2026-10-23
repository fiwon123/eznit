import { useEffect, useMemo, useState } from "react";
import { Link } from "react-router-dom";
import { ApiError, listTemplates, messageOf } from "../api";
import TemplateCard from "../components/TemplateCard";
import { KINDS } from "../types";
import type { TemplateData } from "../types";

// The answer of one catalogue request. It carries the kind it belongs to, so
// a slow answer for an old kind never overwrites a newer one, and the loading
// state is derived from it instead of being set from inside the effect.
interface CatalogueResult {
  kind: string;
  templates: TemplateData[];
  error: string | null;
  status: number | null;
}

// A stable empty list: a fresh [] on every render would make the useMemo
// below rebuild on every render too.
const NO_TEMPLATES: TemplateData[] = [];

export default function Catalogue() {
  const [kind, setKind] = useState<string>("");
  const [query, setQuery] = useState<string>("");
  const [result, setResult] = useState<CatalogueResult | null>(null);

  useEffect(() => {
    let cancelled = false;

    const load = async (): Promise<void> => {
      try {
        const response = await listTemplates(kind);
        if (!cancelled) {
          setResult({ kind, templates: response.data, error: null, status: null });
        }
      } catch (cause: unknown) {
        if (!cancelled) {
          setResult({
            kind,
            templates: [],
            error: messageOf(cause),
            status: cause instanceof ApiError ? cause.status : null,
          });
        }
      }
    };

    void load();

    return () => {
      cancelled = true;
    };
  }, [kind]);

  const current = result !== null && result.kind === kind ? result : null;
  const templates = current === null ? NO_TEMPLATES : current.templates;
  const error = current === null ? null : current.error;
  const status = current === null ? null : current.status;
  const loading = current === null;

  // The search runs on the browser: the API filters by kind, and the text a
  // user types is not worth a round trip per keystroke.
  const visible = useMemo(() => {
    const needle = query.trim().toLowerCase();
    if (needle === "") {
      return templates;
    }
    return templates.filter((template) =>
      [template.name, template.description, template.owner, template.kind]
        .join(" ")
        .toLowerCase()
        .includes(needle),
    );
  }, [templates, query]);

  return (
    <section className="page">
      <div className="pagehead">
        <h1>The catalogue</h1>
        <p className="lead">
          The templates published on this instance. Open one to see the files it holds and the
          questions it asks, then apply it with the CLI.
        </p>
      </div>

      <div className="filters">
        <label className="field">
          <span>Search</span>
          <input
            type="search"
            value={query}
            placeholder="name, description, owner"
            onChange={(event) => setQuery(event.target.value)}
          />
        </label>

        <label className="field">
          <span>Kind</span>
          <select value={kind} onChange={(event) => setKind(event.target.value)}>
            <option value="">all kinds</option>
            {KINDS.map((known) => (
              <option key={known} value={known}>
                {known}
              </option>
            ))}
          </select>
        </label>
      </div>

      {status === 401 ? (
        <p className="notice">
          The catalogue is behind a session. <Link to="/login">Log in</Link> to see the templates.
        </p>
      ) : null}

      {error !== null && status !== 401 ? <p className="error">{error}</p> : null}

      {loading ? <p className="empty">Loading the catalogue…</p> : null}

      {!loading && error === null && visible.length === 0 ? (
        <p className="empty">
          Nothing here yet. Publish one with <code>eznit template publish &lt;folder&gt;</code>.
        </p>
      ) : null}

      <div className="grid">
        {visible.map((template) => (
          <TemplateCard key={template.id} template={template} />
        ))}
      </div>
    </section>
  );
}
