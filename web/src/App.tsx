import { useState } from "react";
import { Link, Route, Routes } from "react-router-dom";
import { clearToken, getToken, setToken } from "./api";
import Catalogue from "./pages/Catalogue";
import Login from "./pages/Login";
import Signup from "./pages/Signup";
import TemplateDetailPage from "./pages/TemplateDetail";

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
            <button className="linkbutton" type="button" onClick={logOut}>
              log out
            </button>
          )}
        </nav>
      </header>

      <main className="main">
        <Routes>
          <Route path="/" element={<Catalogue />} />
          <Route path="/templates/:id" element={<TemplateDetailPage />} />
          <Route path="/login" element={<Login onAuthenticated={logIn} />} />
          <Route path="/signup" element={<Signup onAuthenticated={logIn} />} />
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
