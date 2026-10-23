import { useState } from "react";
import type { FormEvent } from "react";
import { Link, useNavigate } from "react-router-dom";
import { login, messageOf, setToken, signup } from "../api";

interface SignupProps {
  onAuthenticated: (token: string) => void;
}

export default function Signup({ onAuthenticated }: SignupProps) {
  const navigate = useNavigate();
  const [email, setEmail] = useState<string>("");
  const [password, setPassword] = useState<string>("");
  const [confirmPassword, setConfirmPassword] = useState<string>("");
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState<boolean>(false);

  async function submit(event: FormEvent<HTMLFormElement>): Promise<void> {
    event.preventDefault();
    setBusy(true);
    setError(null);

    try {
      await signup(email, password, confirmPassword);
      // A new account has no session yet, so the log-in follows rather than
      // throwing the reader back to the form to type it all again.
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
        <p className="lead">
          Create an account to publish templates and manage your files. The token stays in this
          browser and nowhere else.
        </p>
      </div>

      <form className="form" onSubmit={submit}>
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

        <label className="field">
          <span>Confirm password</span>
          <input
            type="password"
            name="confirm_password"
            required
            autoComplete="new-password"
            value={confirmPassword}
            onChange={(event) => setConfirmPassword(event.target.value)}
          />
        </label>

        {error !== null ? (
          <p className="error" role="alert">
            {error}
          </p>
        ) : null}

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
