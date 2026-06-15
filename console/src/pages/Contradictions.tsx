import { useState } from "react";
import { Link, useParams } from "react-router-dom";
import { Api } from "../api";
import { useAsync } from "../hooks";
import { Layout } from "../components/Layout";
import { AsyncView } from "../components/ui";
import { displayContent } from "../format";

export default function Contradictions() {
  const { agentId = "" } = useParams();
  const state = useAsync(() => Api.contradictions(agentId), [agentId]);
  const [busy, setBusy] = useState<string | null>(null);
  const [msg, setMsg] = useState<string | null>(null);

  async function resolve(keepId: string, demoteId: string) {
    const reason = window.prompt("Reason (recorded in the audit trail):");
    if (!reason) return;
    setBusy(demoteId);
    try {
      await Api.resolveContradiction(keepId, demoteId, reason);
      setMsg("Contradiction resolved — the demoted belief was archived and recorded.");
      state.reload();
    } catch (e) {
      setMsg(e instanceof Error ? e.message : "Failed");
    } finally {
      setBusy(null);
    }
  }

  return (
    <Layout title={<><Link to="/agents">Agents</Link> / <Link to={`/agents/${agentId}`}>Agent</Link> / Contradictions</>}>
      <div className="page-head">
        <h2>Contradictions</h2>
        <p>Conflicting beliefs detected in this agent's memory. Resolving one archives the loser and records why.</p>
      </div>
      {msg && <div className="notice" style={{ marginBottom: 14 }}>{msg}</div>}

      <AsyncView state={state} empty={(d) => d.pairs.length === 0}>
        {(d) => (
          <div className="grid">
            {d.pairs.map((p, i) => (
              <div key={p.belief_id + p.other_id + i} className="review-item">
                <div className="row" style={{ alignItems: "stretch", gap: 16, flexWrap: "wrap" }}>
                  <div style={{ flex: 1, minWidth: 220 }}>
                    <div className="conf" style={{ marginBottom: 4 }}>A · {p.belief_confidence.toFixed(2)}</div>
                    <div className="belief">{displayContent(p.belief_content)}</div>
                    <button className="sm" disabled={busy === p.other_id} onClick={() => resolve(p.belief_id, p.other_id)}>Keep A</button>
                  </div>
                  <div className="vs" style={{ alignSelf: "center" }}>vs</div>
                  <div style={{ flex: 1, minWidth: 220 }}>
                    <div className="conf" style={{ marginBottom: 4 }}>B · {p.other_confidence.toFixed(2)}</div>
                    <div className="belief">{displayContent(p.other_content)}</div>
                    <button className="secondary sm" disabled={busy === p.other_id} onClick={() => resolve(p.other_id, p.belief_id)}>Keep B</button>
                  </div>
                </div>
              </div>
            ))}
          </div>
        )}
      </AsyncView>
    </Layout>
  );
}
