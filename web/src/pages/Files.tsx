import { useEffect, useState } from "react";
import type { ChangeEvent } from "react";
import { Link } from "react-router-dom";
import {
  deleteFile,
  downloadFile,
  fileNameOf,
  getToken,
  listFiles,
  MAX_UPLOAD_BYTES,
  messageOf,
  uploadFile,
} from "../api";
import type { FileData } from "../types";

const MAX_UPLOAD_MB = MAX_UPLOAD_BYTES / (1024 * 1024);

export default function Files() {
  // The token is read at render time. The top bar re-renders when the session
  // changes, and this route is reached from it, so the value here is the one
  // the request below will use.
  const token = getToken();

  const [files, setFiles] = useState<FileData[]>([]);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState<boolean>(token !== null);
  const [busy, setBusy] = useState<boolean>(false);

  useEffect(() => {
    if (token === null) {
      return;
    }

    let cancelled = false;
    (async () => {
      try {
        const response = await listFiles();
        if (!cancelled) {
          setFiles(response.data ?? []);
          setError(null);
        }
      } catch (cause: unknown) {
        if (!cancelled) {
          setError(messageOf(cause));
        }
      } finally {
        if (!cancelled) {
          setLoading(false);
        }
      }
    })();

    return () => {
      cancelled = true;
    };
  }, [token]);

  // reload re-reads the list after a change, so the page shows what the API
  // stored instead of a guess at it.
  async function reload(): Promise<void> {
    const response = await listFiles();
    setFiles(response.data ?? []);
  }

  async function onUpload(event: ChangeEvent<HTMLInputElement>): Promise<void> {
    const input = event.currentTarget;
    const file = input.files?.[0];
    if (file === undefined) {
      return;
    }

    if (file.size > MAX_UPLOAD_BYTES) {
      setError(`${file.name} is larger than the ${MAX_UPLOAD_MB} MB an upload may carry.`);
      input.value = "";
      return;
    }

    setBusy(true);
    try {
      await uploadFile(file);
      await reload();
      setError(null);
    } catch (cause: unknown) {
      setError(messageOf(cause));
    } finally {
      setBusy(false);
      input.value = "";
    }
  }

  async function onDownload(file: FileData): Promise<void> {
    try {
      await downloadFile(file);
      setError(null);
    } catch (cause: unknown) {
      setError(messageOf(cause));
    }
  }

  async function onDelete(file: FileData): Promise<void> {
    if (!window.confirm(`Delete ${fileNameOf(file)}? This cannot be undone.`)) {
      return;
    }

    setBusy(true);
    try {
      await deleteFile(file.id);
      await reload();
      setError(null);
    } catch (cause: unknown) {
      setError(messageOf(cause));
    } finally {
      setBusy(false);
    }
  }

  return (
    <section className="page">
      <div className="pagehead">
        <h1>Your files</h1>
        <p className="lead">
          Upload, download and delete your own files. They stay on this server.
        </p>
      </div>

      {token === null ? (
        <p className="notice">
          <Link to="/login">Log in</Link> to see your files.
        </p>
      ) : null}

      {token !== null ? (
        <div className="uploadrow">
          <label className="uploadlabel">
            <span>Upload a file</span>
            <input
              className="fileinput"
              type="file"
              onChange={(event) => void onUpload(event)}
              disabled={busy}
            />
          </label>
          {busy ? <span className="empty">working…</span> : null}
        </div>
      ) : null}

      {error !== null ? (
        <p className="error" role="alert">
          {error}
        </p>
      ) : null}

      {loading ? <p className="empty">Loading files…</p> : null}

      {!loading && token !== null && files.length === 0 ? (
        <p className="empty">No files yet. Upload one above.</p>
      ) : null}

      {token !== null && files.length > 0 ? (
        <div className="tablewrap">
          <table className="table">
            <thead>
              <tr>
                <th>Name</th>
                <th>Type</th>
                <th>Version</th>
                <th>
                  <span className="visuallyhidden">Actions</span>
                </th>
              </tr>
            </thead>
            <tbody>
              {files.map((file) => (
                <tr key={file.id}>
                  <td>{fileNameOf(file)}</td>
                  <td>{file.ext === "" ? "none" : file.ext}</td>
                  <td>v{file.version}</td>
                  <td>
                    <div className="tableactions">
                      <button
                        type="button"
                        className="linkbutton"
                        onClick={() => void onDownload(file)}
                        aria-label={`Download ${fileNameOf(file)}`}
                      >
                        download
                      </button>
                      <button
                        type="button"
                        className="linkbutton danger"
                        onClick={() => void onDelete(file)}
                        aria-label={`Delete ${fileNameOf(file)}`}
                        disabled={busy}
                      >
                        delete
                      </button>
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      ) : null}
    </section>
  );
}
