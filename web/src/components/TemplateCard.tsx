import { Link } from "react-router-dom";
import type { TemplateData } from "../types";

interface TemplateCardProps {
  template: TemplateData;
}

export default function TemplateCard({ template }: TemplateCardProps) {
  const files = template.file_count === 1 ? "1 file" : `${template.file_count} files`;

  return (
    <article className="card">
      <header className="cardhead">
        <h2 className="cardtitle">
          <Link to={`/templates/${template.id}`}>{template.name}</Link>
        </h2>
        <span className="badge">{template.kind}</span>
      </header>

      <p className="cardbody">
        {template.description === "" ? "No description." : template.description}
      </p>

      <footer className="cardfoot">
        <span>{files}</span>
        <span>v{template.version}</span>
        <span>{template.is_public ? "public" : "private"}</span>
        <span>by {template.owner}</span>
      </footer>
    </article>
  );
}
