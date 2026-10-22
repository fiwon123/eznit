import { useState } from "react";
import { Link, Route, Routes, useParams } from "react-router-dom";
import { clearToken, getToken, setToken } from "./api";
import Catalogue from "./pages/Catalogue";
import Files from "./pages/Files";
import Login from "./pages/Login";
import TemplateDetailPage from "./pages/TemplateDetail";

// TemplateDetailPageRoute keys the detail page by the template it shows, so
// moving from one template to another is a new page: the download state, the
// copied flag and the refusal all start fresh, and a slow answer for the old
// template cannot land on the new one.
function TemplateDetailPageRoute() {
  const { id } = useParams<{ id: string }>();
  return <TemplateDetailPage key={id ?? ""} />;
}

export default function App() {
  // The token lives in localStorage; this state is only there so the top bar
  // and the pages agree on whether there is a session right now.
  const [token, setTokenState] = useState<string | null>(getToken);

  function logIn(next: string): void {
    setToken(next);
    setTokenState(next);
  }

  function logOut(): void {
    clearToken();
    setTokenState(null);
  }

  return (
    <div className="app">
      <header className="topbar">
        <Link className="brand" to="/">
          eznit
        </Link>
        <nav className="topnav">
          <Link to="/">catalogue</Link>
          {token === null ? (
            <Link to="/login">log in</Link>
          ) : (
            <>
              <Link to="/files">files</Link>
              <button className="linkbutton" type="button" onClick={logOut}>
                log out
              </button>
            </>
          )}
        </nav>
      </header>

      <main className="main">
        <Routes>
          <Route path="/" element={<Catalogue />} />
          {/* Keyed by the id, so moving to another template is a new page: the
              refusal, the copied flag and the download state all start fresh,
              and a slow answer for the old one cannot land here. */}
          <Route path="/templates/:id" element={<TemplateDetailPageRoute />} />
          {/* Keyed by the session, so a log-out — or a log-in as someone else —
              starts from an empty table instead of the last account's rows. */}
          <Route path="/files" element={<Files key={token ?? "anon"} />} />
          <Route path="/login" element={<Login onAuthenticated={logIn} />} />
          <Route
            path="*"
            element={<p className="empty">There is no such page. The catalogue is over here.</p>}
          />
        </Routes>
      </main>

      <footer className="footer">
        <span>eznit — self-hosted file storage</span>
        <code>eznit template list</code>
      </footer>
    </div>
  );
}
