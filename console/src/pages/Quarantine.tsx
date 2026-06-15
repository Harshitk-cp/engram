import { useState } from "react";
import { Link, useParams } from "react-router-dom";
import { Api } from "../api";
import { useAsync } from "../hooks";
import { Layout } from "../components/Layout";
import { AsyncView } from "../components/ui";

export default function Quarantine() {
  const { agentId = "" } = useParams();
  const state = useAsync(() => Api.quarantine(agentId), [agentId]);
  const [busy, setBusy] = useState<string | null>(null);
  const [msg, setMsg] = useState<string | null>(null);

  async function release(id: string) {
    const note = window.prompt("Optional note for the audit trail (why this is trusted):") ?? "";
    setBusy(id);
    setMsg(null);
    try {
      await Api.releaseQuarantine(id, note);
      setMsg("Released — admitted to active memory and recorded in the audit chain.");
      state.reload();
    } catch (e) {
      setMsg(e instanceof Error ? e.message : "Release failed");
    } finally {
      setBusy(null);
    }
  }

  async function reject(id: string) {
    const note = window.prompt("Optional note for the audit trail (why this is rejected):") ?? "";
    if (!window.confirm("Permanently delete this quarantined write? This is recorded in the audit chain.")) return;
    setBusy(id);
    setMsg(null);
    try {
      await Api.rejectQuarantine(id, note);
      setMsg("Rejected — the untrusted write was discarded and recorded.");
      state.reload();
    } catch (e) {
      setMsg(e instanceof Error ? e.message : "Reject failed");
    } finally {
      setBusy(null);
    }
  }

  return (
    <Layout title={<><Link to="/agents">Agents</Link> / <Link to={`/agents/${agentId}`}>Knowledge health</Link> / Quarantine</>}>
      <div className="page-head">
        <h2>Provenance Firewall — quarantine</h2>
        <p>Untrusted writes held out of recall and belief logic. Release to admit them, or reject to discard. Every decision is recorded in the tamper-evident audit chain.</p>
      </div>
      {msg && <div className="notice" style={{ marginBottom: 14 }}>{msg}</div>}

      <AsyncView state={state} empty={(d) => d.items.length === 0}>
        {(d) => (
          <div className="grid">
            <div className="muted" style={{ fontSize: "0.85rem" }}>{d.total} held for review</div>
            {d.items.map((item) => (
              <div key={item.id} className="review-item">
                <div className="row-between">
                  <div className="row" style={{ gap: 8 }}>
                    <span className="pill">{item.provenance}</span>
                    {item.type && <span className="pill">{item.type}</span>}
                  </div>
                  <span className="muted" style={{ fontSize: "0.78rem" }}>
                    {item.quarantined_at ? new Date(item.quarantined_at).toLocaleString() : ""}
                  </span>
                </div>
                <div className="belief">{item.content}</div>
                {item.quarantine_reason && (
                  <div className="muted" style={{ fontSize: "0.8rem", marginTop: 6 }}>⚠ {item.quarantine_reason}</div>
                )}
                <div className="resolve-actions" style={{ marginTop: 10 }}>
                  <button className="sm" disabled={busy === item.id} onClick={() => release(item.id)}>Release</button>
                  <button className="secondary sm" disabled={busy === item.id} onClick={() => reject(item.id)}>Reject</button>
                </div>
              </div>
            ))}
          </div>
        )}
      </AsyncView>
    </Layout>
  );
}
