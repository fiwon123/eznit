import { useState } from "react";
import { ApiError, downloadFileShared, fileNameOf, messageOf } from "../api";
import type { SharedFileData } from "../types";

interface SharedFileCardProps {
  file: SharedFileData;
}

export default function SharedFileCard({ file }: SharedFileCardProps) {
  const [error, setError] = useState<string | null>(null);

  async function onDownload(): Promise<void> {
    setError(null);
    try {
      await downloadFileShared(file);
    } catch (cause: unknown) {
      if (cause instanceof ApiError) {
        setError(cause.message);
        return;
      }
      setError(messageOf(cause));
    }
  }

  const name = fileNameOf({
    id: file.id,
    name: file.name,
    ext: file.ext,
    version: file.version,
  } as any);

  return (
    <article className="card">
      <header className="cardhead">
        <h2 className="cardtitle">{name}</h2>
        <span className="badge">shared file</span>
      </header>
      {/* The API leaves the address off a visitor's catalogue: a public list
          that carried it would be a list of the instance's accounts. */}
      <p className="cardbody">
        {file.owner_email === "" ? "Shared on this instance" : `Shared by ${file.owner_email}`}
      </p>
      <footer className="cardfoot">
        <button type="button" className="linkbutton" onClick={() => void onDownload()}>
          download
        </button>
      </footer>
      {error !== null ? <p className="error">{error}</p> : null}
    </article>
  );
}
