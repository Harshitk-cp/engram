import { useState } from "react";
import { Link, useParams } from "react-router-dom";
import { Api } from "../api";
import { useAsync } from "../hooks";
import { Layout } from "../components/Layout";
import { AsyncView } from "../components/ui";
import { tierOf } from "../format";

// datetime-local <-> ISO helpers (the input works in local time).
function toLocalInput(d: Date): string {
  const pad = (n: number) => String(n).padStart(2, "0");
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`;
}

const JUMPS: [string, number][] = [
  ["1h ago", 3600e3],
  ["1d ago", 86400e3],
  ["7d ago", 7 * 86400e3],
  ["30d ago", 30 * 86400e3],
];

export default function TimeMachine() {
  const { agentId = "" } = useParams();
  const [at, setAt] = useState<Date>(new Date());
  const iso = at.toISOString();
  const state = useAsync(() => Api.snapshot(agentId, iso), [agentId, iso]);

  return (
    <Layout title={<><Link to="/agents">Agents</Link> / <Link to={`/agents/${agentId}`}>Agent</Link> / Time machine</>}>
      <div className="page-head">
        <h2>Time machine</h2>
        <p>Reconstruct what this agent believed at any past moment — confidence is replayed from the audit log.</p>
      </div>

      <div className="card" style={{ marginBottom: 18 }}>
        <div className="row wrap" style={{ gap: 12, alignItems: "flex-end" }}>
          <div>
            <label>As of</label>
            <input
              type="datetime-local"
              value={toLocalInput(at)}
              max={toLocalInput(new Date())}
              onChange={(e) => e.target.value && setAt(new Date(e.target.value))}
            />
          </div>
          <div className="row wrap" style={{ gap: 8 }}>
            <button className="secondary sm" onClick={() => setAt(new Date())}>Now</button>
            {JUMPS.map(([label, ms]) => (
              <button key={label} className="secondary sm" onClick={() => setAt(new Date(Date.now() - ms))}>
                {label}
              </button>
            ))}
          </div>
        </div>
      </div>

      <AsyncView state={state}>
        {(snap) => (
          <>
            <p className="secondary" style={{ marginBottom: 14 }}>
              <strong>{snap.total.toLocaleString()}</strong> belief{snap.total === 1 ? "" : "s"} existed as of{" "}
              {new Date(snap.as_of).toLocaleString()}
              {snap.beliefs.length < snap.total && <> · showing top {snap.beliefs.length} by confidence</>}
            </p>
            {snap.beliefs.length === 0 ? (
              <div className="empty"><div style={{ fontWeight: 600, color: "var(--text-secondary)" }}>The agent had no beliefs yet at this time.</div></div>
            ) : (
              <div className="card" style={{ padding: 0 }}>
                <table className="table">
                  <thead><tr><th>Belief</th><th>Type</th><th>Tier then</th><th>Confidence then</th></tr></thead>
                  <tbody>
                    {snap.beliefs.map((b) => {
                      const t = b.tier ?? tierOf(b.confidence);
                      return (
                        <tr key={b.id}>
                          <td style={{ maxWidth: 460 }}>
                            <Link className="link" to={`/memories/${b.id}`}>{b.content}</Link>
                          </td>
                          <td><span className="pill">{b.type}</span></td>
                          <td><span className={`pill ${t}`}>{t}</span></td>
                          <td className="conf">{b.confidence.toFixed(2)}</td>
                        </tr>
                      );
                    })}
                  </tbody>
                </table>
              </div>
            )}
          </>
        )}
      </AsyncView>
    </Layout>
  );
}
