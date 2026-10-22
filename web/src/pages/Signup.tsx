import { useState } from "react";
import type { FormEvent } from "react";
import { Link, useNavigate } from "react-router-dom";
import { login, messageOf, setToken, signup } from "../api";

interface SignupProps {
  onAuthenticated: (token: string) => void;
}

export default function Signup({ onAuthenticated }: SignupProps) {
  const navigate = useNavigate();
  const [name, setName] = useState<string>("");
  const [email, setEmail] = useState<string>("");
  const [password, setPassword] = useState<string>("");
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState<boolean>(false);

  async function submit(event: FormEvent<HTMLFormElement>): Promise<void> {
    event.preventDefault();
    setBusy(true);
    setError(null);

    try {
      await signup(email, password, name);
      const response = await login(email, password);
      setToken(response.token);
      onAuthenticated(response.token);
      navigate("/");
    } catch (cause: unknown) {
      setError(messageOf(cause));
    } finally {
      setBusy(false);
    }
  }

  return (
    <section className="page narrow">
      <div className="pagehead">
        <h1>Sign up</h1>
        <p className="lead">Create an account to publish templates and manage your files.</p>
      </div>

      <form className="form" onSubmit={submit}>
        <label className="field">
          <span>Name</span>
          <input
            type="text"
            name="name"
            required
            autoComplete="name"
            value={name}
            onChange={(event) => setName(event.target.value)}
          />
        </label>

        <label className="field">
          <span>Email</span>
          <input
            type="email"
            name="email"
            required
            autoComplete="email"
            value={email}
            onChange={(event) => setEmail(event.target.value)}
          />
        </label>

        <label className="field">
          <span>Password</span>
          <input
            type="password"
            name="password"
            required
            autoComplete="new-password"
            value={password}
            onChange={(event) => setPassword(event.target.value)}
          />
        </label>

        {error !== null ? <p className="error">{error}</p> : null}

        <button className="primary" type="submit" disabled={busy}>
          {busy ? "Signing up…" : "Sign up"}
        </button>
      </form>

      <p>
        Already have an account? <Link to="/login">Log in</Link>
      </p>
    </section>
  );
}
