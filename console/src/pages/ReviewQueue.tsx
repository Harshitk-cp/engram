import { useState } from "react";
import { Link, useParams } from "react-router-dom";
import { Api } from "../api";
import { useAsync } from "../hooks";
import { Layout } from "../components/Layout";
import { AsyncView } from "../components/ui";

export default function ReviewQueue() {
  const { agentId = "" } = useParams();
  const state = useAsync(() => Api.reviewQueue(agentId), [agentId]);
  const [busy, setBusy] = useState<string | null>(null);
  const [msg, setMsg] = useState<string | null>(null);

  async function resolve(keepId: string, demoteId: string) {
    const reason = window.prompt("Reason for resolving this contradiction (recorded in the audit trail):");
    if (!reason) return;
    setBusy(demoteId);
    setMsg(null);
    try {
      await Api.resolveContradiction(keepId, demoteId, reason);
      setMsg("Resolved — the demoted belief was archived and the action recorded.");
      state.reload();
    } catch (e) {
      setMsg(e instanceof Error ? e.message : "Resolve failed");
    } finally {
      setBusy(null);
    }
  }

  return (
    <Layout title={<><Link to="/agents">Agents</Link> / <Link to={`/agents/${agentId}`}>Agent</Link> / Review</>}>
      <div className="page-head">
        <h2>Review queue</h2>
        <p>Beliefs flagged for review, paired with the beliefs they conflict with.</p>
      </div>
      {msg && <div className="notice" style={{ marginBottom: 14 }}>{msg}</div>}

      <AsyncView state={state} empty={(d) => d.items.length === 0}>
        {(d) => (
          <div className="grid">
            {d.items.map((item) => (
              <div key={item.memory.id} className="review-item">
                <div className="row-between">
                  <div className="row">
                    <span className={`pill ${item.tier}`}>{item.tier}</span>
                    <span className="conf">confidence {item.memory.confidence.toFixed(2)}</span>
                  </div>
                  <Link className="link" to={`/memories/${item.memory.id}`}>History →</Link>
                </div>
                <div className="belief">{item.memory.content}</div>

                {item.contradictions && item.contradictions.length > 0 ? (
                  item.contradictions.map((c) => (
                    <div key={c.memory_id} className="contradiction">
                      <div className="vs">conflicts with</div>
                      <div className="belief">
                        {c.content ?? c.memory_id}
                        {c.confidence != null && <span className="conf"> · {c.confidence.toFixed(2)}</span>}
                      </div>
                      <div className="resolve-actions">
                        <button className="sm" disabled={busy === c.memory_id} onClick={() => resolve(item.memory.id, c.memory_id)}>
                          Keep current
                        </button>
                        <button className="secondary sm" disabled={busy === c.memory_id} onClick={() => resolve(c.memory_id, item.memory.id)}>
                          Keep the other
                        </button>
                      </div>
                    </div>
                  ))
                ) : (
                  <div className="muted">Flagged for review (no linked contradiction).</div>
                )}
              </div>
            ))}
          </div>
        )}
      </AsyncView>
    </Layout>
  );
}
