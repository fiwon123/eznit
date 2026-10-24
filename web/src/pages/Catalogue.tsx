import { useEffect, useMemo, useState } from "react";
import { Link } from "react-router-dom";
import { getToken, listSharedFiles, listTemplates, messageOf } from "../api";
import SharedFileCard from "../components/SharedFileCard";
import TemplateCard from "../components/TemplateCard";
import { KINDS } from "../types";
import type { SharedFileData, TemplateData } from "../types";

// The answer of one catalogue request. It carries the kind it belongs to, so
// a slow answer for an old kind never overwrites a newer one, and the loading
// state is derived from it instead of being set from inside the effect.
interface CatalogueResult {
  kind: string;
  // The session the answer belongs to: the catalogue widens to the caller's
  // own private templates when there is one, so an answer from before a log-in
  // or a log-out is not the answer for this page.
  session: string | null;
  templates: TemplateData[];
  error: string | null;
}

// A stable empty list: a fresh [] on every render would make the useMemo
// below rebuild on every render too.
const NO_TEMPLATES: TemplateData[] = [];

export default function Catalogue() {
  // The catalogue is public, so a visitor with no token reads it. The token
  // is read at render time: it only changes the nudge at the top, because a
  // logged-in caller additionally sees their own private templates.
  const token = getToken();

  const [kind, setKind] = useState<string>("");
  const [query, setQuery] = useState<string>("");
  const [result, setResult] = useState<CatalogueResult | null>(null);
  const [sharedFiles, setSharedFiles] = useState<SharedFileData[]>([]);
  const [sharedError, setSharedError] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;

    const load = async (): Promise<void> => {
      try {
        const response = await listTemplates(kind);
        if (!cancelled) {
          setResult({ kind, session: token, templates: response.data, error: null });
        }
      } catch (cause: unknown) {
        if (!cancelled) {
          setResult({ kind, session: token, templates: [], error: messageOf(cause) });
        }
      }
    };

    void load();

    return () => {
      cancelled = true;
    };
  }, [kind, token]);

  useEffect(() => {
    let cancelled = false;

    const load = async (): Promise<void> => {
      try {
        const response = await listSharedFiles();
        if (!cancelled) {
          setSharedFiles(response.data ?? []);
          setSharedError(null);
        }
      } catch (cause: unknown) {
        if (!cancelled) {
          setSharedFiles([]);
          setSharedError(messageOf(cause));
        }
      }
    };

    void load();

    return () => {
      cancelled = true;
    };
  }, []);

  const current =
    result !== null && result.kind === kind && result.session === token ? result : null;
  const templates = current === null ? NO_TEMPLATES : current.templates;
  const error = current === null ? null : current.error;
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

      {token === null ? (
        <p className="notice">
          You are browsing as a visitor, which shows the public templates and shared files.{" "}
          <Link to="/login">Log in</Link> to publish a template or to see your own private ones.
        </p>
      ) : null}

      {error !== null ? <p className="error">{error}</p> : null}

      {loading ? <p className="empty">Loading the catalogue…</p> : null}

      {!loading && error === null && visible.length === 0 ? (
        <p className="empty">
          Nothing here yet. Publish one with <code>eznit template publish &lt;folder&gt;</code>.
        </p>
      ) : null}

      {sharedError !== null ? <p className="error">{sharedError}</p> : null}

      {sharedFiles.length > 0 ? (
        <>
          <h2>Shared files</h2>
          <div className="grid">
            {sharedFiles.map((file) => (
              <SharedFileCard key={file.id} file={file} />
            ))}
          </div>
        </>
      ) : null}

      <h2>Templates</h2>
      <div className="grid">
        {visible.map((template) => (
          <TemplateCard key={template.id} template={template} />
        ))}
      </div>
    </section>
  );
}
