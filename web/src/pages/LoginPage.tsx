import { useEffect } from "react";
import { useAuth } from "../auth/AuthContext";

export function LoginPage({ navigate }: { navigate: (to: string, replace?: boolean) => void }) {
  const { session, login } = useAuth();

  useEffect(() => {
    if (session) navigate("/", true);
  }, [navigate, session]);

  if (session) return null;

  return (
    <main className="threshold">
      <div className="threshold-atmosphere" aria-hidden="true">
        <span className="threshold-orbit threshold-orbit--outer" />
        <span className="threshold-orbit threshold-orbit--middle" />
        <span className="threshold-orbit threshold-orbit--inner" />
        <span className="threshold-core" />
        <span className="threshold-star threshold-star--one" />
        <span className="threshold-star threshold-star--two" />
        <span className="threshold-star threshold-star--three" />
      </div>

      <header className="threshold-header">
        <div className="threshold-brand" aria-label="Agent Room">
          <span className="threshold-sigil" aria-hidden="true">AR</span>
          <span>Agent Room</span>
        </div>
        <p>Room&nbsp;01&nbsp;&nbsp;/&nbsp;&nbsp;Signal quiet</p>
      </header>

      <section className="threshold-copy" aria-labelledby="threshold-title">
        <p className="threshold-kicker">The human remains in the loop</p>
        <h1 id="threshold-title" aria-label="The work continues. The room remembers.">
          The work continues.
          <span>The room remembers.</span>
        </h1>
        <p className="threshold-intro">
          Signals gather here. Evidence stays attached. The right interruption
          waits for the right human.
        </p>
        <div className="threshold-entry">
          <button className="threshold-button" type="button" onClick={login}>
            <span>Enter quietly</span>
            <span aria-hidden="true">↗</span>
          </button>
          <small>If you are expected, the door already knows.</small>
        </div>
      </section>

      <footer className="threshold-footer">
        <span>Attention, not noise</span>
        <span aria-hidden="true">·</span>
        <span>Evidence, not claims</span>
        <span aria-hidden="true">·</span>
        <span>Control, with consequence</span>
      </footer>
    </main>
  );
}
