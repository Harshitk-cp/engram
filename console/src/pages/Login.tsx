import { useState } from "react";
import { Link, useNavigate, useSearchParams } from "react-router-dom";
import { useAuth } from "../auth";
import { Logo } from "../components/Logo";

const oauthErrors: Record<string, string> = {
  provider_not_configured: "That sign-in provider isn't configured.",
  state_mismatch: "Sign-in expired, please try again.",
  token_exchange: "Could not complete sign-in with the provider.",
  profile: "Could not read your profile from the provider.",
  login_failed: "Sign-in failed. Please try again.",
};

export default function Login() {
  const { config, login } = useAuth();
  const nav = useNavigate();
  const [params] = useSearchParams();
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(
    params.get("error") ? oauthErrors[params.get("error")!] || "Sign-in failed." : null
  );

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError(null);
    try {
      await login(email.trim(), password);
      nav("/agents", { replace: true });
    } catch (err) {
      setError(err instanceof Error ? err.message : "Sign in failed");
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="auth">
      <div className="auth-card">
        <Logo />
        <h1 style={{ marginTop: 18 }}>Sign in</h1>
        <p className="secondary">Welcome back to the agent memory console.</p>

        {(config?.google || config?.github || config?.workos) && (
          <div className="oauth-btns" style={{ marginTop: 18 }}>
            {config?.google && (
              <a className="btn oauth-btn" href="/auth/oauth/google/start">Continue with Google</a>
            )}
            {config?.github && (
              <a className="btn oauth-btn" href="/auth/oauth/github/start">Continue with GitHub</a>
            )}
            {config?.workos && (
              <a className="btn oauth-btn" href="/auth/sso/start">Continue with SSO (SAML/OIDC)</a>
            )}
            <div className="divider">or</div>
          </div>
        )}

        <form className="auth-form" onSubmit={submit}>
          <div>
            <label>Email</label>
            <input type="email" value={email} onChange={(e) => setEmail(e.target.value)} placeholder="you@company.com" autoFocus />
          </div>
          <div>
            <label>Password</label>
            <input type="password" value={password} onChange={(e) => setPassword(e.target.value)} placeholder="••••••••" />
          </div>
          {error && <div className="error-box">{error}</div>}
          <button type="submit" disabled={busy || !email || !password}>
            {busy ? "Signing in…" : "Sign in"}
          </button>
        </form>

        <div className="auth-foot">
          No account? <Link to="/signup">Create one</Link>
        </div>
      </div>
    </div>
  );
}
