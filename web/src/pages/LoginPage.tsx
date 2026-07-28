import { useEffect } from "react";
import { useAuth } from "../auth/AuthContext";

export function LoginPage({ navigate }: { navigate: (to: string, replace?: boolean) => void }) {
  const { session, login } = useAuth();

  useEffect(() => {
    if (session) navigate("/", true);
  }, [navigate, session]);

  if (session) return null;

  return (
    <main className="login-layout">
      <section className="login-story" aria-labelledby="login-story-title">
        <div className="brand brand--large">
          <span className="brand-mark" aria-hidden="true">AR</span>
          <div><strong>Agent Room</strong><span>Human control plane</span></div>
        </div>
        <div>
          <p className="eyebrow">One place to supervise every AI worker</p>
          <h1 id="login-story-title">Know what matters. Inspect the evidence. Act safely.</h1>
          <p>Agent Room turns real worker activity into a trustworthy view of work, attention, and control.</p>
        </div>
      </section>
      <section className="login-panel" aria-labelledby="sign-in-title">
        <div className="login-card">
          <p className="eyebrow">Protected control plane</p>
          <h2 id="sign-in-title">Sign in</h2>
          <p>Continue through the configured identity provider. Agent Room never receives your provider password.</p>
          <button className="button button--primary button--wide" type="button" onClick={login}>
            Continue securely
          </button>
          <small>Authorization code flow with PKCE · session protected by secure cookies</small>
        </div>
      </section>
    </main>
  );
}
