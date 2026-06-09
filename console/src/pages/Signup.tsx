import { useState } from "react";
import { Link, useNavigate } from "react-router-dom";
import { useAuth } from "../auth";
import { Logo } from "../components/Logo";

export default function Signup() {
  const { config, register } = useAuth();
  const nav = useNavigate();
  const [name, setName] = useState("");
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError(null);
    try {
      await register(email.trim(), password, name.trim());
      nav("/agents", { replace: true });
    } catch (err) {
      setError(err instanceof Error ? err.message : "Sign up failed");
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="auth">
      <div className="auth-card">
        <Logo />
        <h1 style={{ marginTop: 18 }}>Create your account</h1>
        <p className="secondary">Start managing your agents' memory.</p>

        {(config?.google || config?.github || config?.workos) && (
          <div className="oauth-btns" style={{ marginTop: 18 }}>
            {config?.google && <a className="btn oauth-btn" href="/auth/oauth/google/start">Continue with Google</a>}
            {config?.github && <a className="btn oauth-btn" href="/auth/oauth/github/start">Continue with GitHub</a>}
            {config?.workos && <a className="btn oauth-btn" href="/auth/sso/start">Continue with SSO (SAML/OIDC)</a>}
            <div className="divider">or</div>
          </div>
        )}

        <form className="auth-form" onSubmit={submit}>
          <div>
            <label>Name</label>
            <input value={name} onChange={(e) => setName(e.target.value)} placeholder="Ada Lovelace" autoFocus />
          </div>
          <div>
            <label>Email</label>
            <input type="email" value={email} onChange={(e) => setEmail(e.target.value)} placeholder="you@company.com" />
          </div>
          <div>
            <label>Password</label>
            <input type="password" value={password} onChange={(e) => setPassword(e.target.value)} placeholder="At least 8 characters" />
          </div>
          {error && <div className="error-box">{error}</div>}
          <button type="submit" disabled={busy || !email || password.length < 8}>
            {busy ? "Creating…" : "Create account"}
          </button>
        </form>

        <div className="auth-foot">
          Already have an account? <Link to="/login">Sign in</Link>
        </div>
      </div>
    </div>
  );
}
