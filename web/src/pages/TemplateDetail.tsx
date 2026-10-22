import { useEffect, useMemo, useState } from "react";
import { Link, useParams } from "react-router-dom";
import { getTemplate, messageOf } from "../api";
import FileTree, { buildTree } from "../components/FileTree";
import type { TemplateDetail, TemplateFileData } from "../types";

// The answer of one detail request. It carries the id it belongs to, so a
// slow answer for an old template never overwrites a newer one.
interface DetailResult {
  id: string;
  template: TemplateDetail | null;
  error: string | null;
}

export default function TemplateDetailPage() {
  const { id } = useParams<{ id: string }>();
  const [result, setResult] = useState<DetailResult | null>(null);
  const [selectedPath, setSelectedPath] = useState<string | null>(null);
  const [copied, setCopied] = useState<boolean>(false);

  useEffect(() => {
    if (id === undefined) {
      return;
    }

    let cancelled = false;

    const load = async (): Promise<void> => {
      try {
        const template = await getTemplate(id);
        if (!cancelled) {
          setResult({ id, template, error: null });
          const first = template.files[0];
          setSelectedPath(first === undefined ? null : first.path);
        }
      } catch (cause: unknown) {
        if (!cancelled) {
          setResult({ id, template: null, error: messageOf(cause) });
        }
      }
    };

    void load();

    return () => {
      cancelled = true;
    };
  }, [id]);

  const current = result !== null && result.id === id ? result : null;
  const template = current === null ? null : current.template;
  const error = current === null ? null : current.error;
  const loading = current === null;

  const tree = useMemo(() => (template === null ? [] : buildTree(template.files)), [template]);

  const selected = useMemo(
    () => template?.files.find((file) => file.path === selectedPath) ?? null,
    [template, selectedPath],
  );

  const applyCommand = `eznit template apply ${id ?? "<id>"}`;

  async function copyCommand(): Promise<void> {
    if (navigator.clipboard === undefined) {
      return;
    }
    try {
      await navigator.clipboard.writeText(applyCommand);
      setCopied(true);
    } catch {
      setCopied(false);
    }
  }

  function select(file: TemplateFileData): void {
    setSelectedPath(file.path);
  }

  if (loading) {
    return (
      <section className="page">
        <p className="empty">Loading the template…</p>
      </section>
    );
  }

  if (error !== null) {
    return (
      <section className="page">
        <p className="error">{error}</p>
        <p>
          <Link to="/">Back to the catalogue</Link>
        </p>
      </section>
    );
  }

  if (template === null) {
    return (
      <section className="page">
        <p className="empty">There is no such template.</p>
      </section>
    );
  }

  return (
    <section className="page">
      <div className="pagehead">
        <h1>{template.name}</h1>
        <p className="lead">{template.description}</p>
        <div className="tags">
          <span className="badge">{template.kind}</span>
          <span className="badge">v{template.version}</span>
          <span className="badge">{template.is_public ? "public" : "private"}</span>
          <span className="badge">by {template.owner}</span>
        </div>
      </div>

      <div className="applybox">
        <div>
          <span className="applylabel">Apply it with</span>
          <code className="applycommand">{applyCommand}</code>
        </div>
        <button type="button" className="secondary" onClick={copyCommand}>
          {copied ? "copied" : "copy"}
        </button>
      </div>

      <div className="detail">
        <aside className="detailside">
          <h2 className="sidehead">Files</h2>
          <FileTree nodes={tree} selected={selectedPath} onSelect={select} />
        </aside>

        <div className="detailmain">
          {selected === null ? (
            <p className="empty">Pick a file to read it.</p>
          ) : (
            <>
              <div className="filehead">
                <h2 className="sidehead">{selected.path}</h2>
                <span className="badge">{selected.content.length} characters</span>
                {selected.is_executable ? <span className="badge">executable</span> : null}
              </div>
              <pre className="filecontent">
                <code>{selected.content === "" ? "(the file is empty)" : selected.content}</code>
              </pre>
            </>
          )}
        </div>
      </div>

      <div className="variables">
        <h2 className="sidehead">The questions it asks</h2>
        {template.variables.length === 0 ? (
          <p className="empty">
            This template asks nothing. Applying it writes the files as they are.
          </p>
        ) : (
          <ul className="variablelist">
            {template.variables.map((variable) => (
              <li key={variable.key}>
                <code className="variablekey">{`{{${variable.key}}}`}</code>
                <span className="variableprompt">{variable.prompt}</span>
                <span className="variablemeta">
                  {variable.required ? "required" : "optional"}
                  {variable.default_value === "" ? "" : ` — default: ${variable.default_value}`}
                </span>
              </li>
            ))}
          </ul>
        )}
      </div>
    </section>
  );
}
