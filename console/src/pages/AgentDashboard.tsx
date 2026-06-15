import { useState } from "react";
import { Link, useParams } from "react-router-dom";
import { Api } from "../api";
import { useAsync } from "../hooks";
import { Layout } from "../components/Layout";
import { AsyncView, Stat, TierBars } from "../components/ui";

export default function AgentDashboard() {
  const { agentId = "" } = useParams();
  const state = useAsync(() => Api.dashboard(agentId), [agentId]);
  const [reembedMsg, setReembedMsg] = useState<string | null>(null);
  const [reembedding, setReembedding] = useState(false);

  async function reembed() {
    if (!window.confirm("Re-embed every memory for this agent with the current embedding model? Use this after switching to a different model of the same dimension.")) return;
    setReembedding(true); setReembedMsg(null);
    try {
      const r = await Api.reembedAgent(agentId);
      setReembedMsg(`Re-embedded ${r.reembedded} memories with the current model.`);
    } catch (e) {
      setReembedMsg(e instanceof Error ? e.message : "Re-embed failed");
    } finally { setReembedding(false); }
  }

  return (
    <Layout title={<><Link to="/agents">Agents</Link> / Knowledge health</>}>
      <AsyncView state={state}>
        {(d) => (
          <>
            <div className="page-head row-between wrap">
              <div>
                <h2>Knowledge Health</h2>
                <p className="mono">{agentId}</p>
              </div>
              <div className="row wrap">
                <Link className="btn secondary" to={`/agents/${agentId}/memories`}>Browse beliefs</Link>
                <Link className="btn secondary" to={`/agents/${agentId}/timemachine`}>Time machine</Link>
                <Link className="btn secondary" to={`/agents/${agentId}/contradictions`}>Contradictions</Link>
              <Link className="btn secondary" to={`/agents/${agentId}/quarantine`}>Quarantine</Link>
                <button className="secondary" disabled={reembedding} onClick={reembed}>{reembedding ? "Re-embedding…" : "Re-embed"}</button>
                <Link className="btn" to={`/agents/${agentId}/review`}>
                  Review queue {d.needs_review_count > 0 && <span className="badge">{d.needs_review_count}</span>}
                </Link>
              </div>
            </div>
            {reembedMsg && <div className="notice" style={{ marginBottom: 14 }}>{reembedMsg}</div>}

            <div className="stat-grid">
              <Stat label="Total beliefs" value={d.total_memories.toLocaleString()} />
              <Stat label="Needs review" value={d.needs_review_count} warn={d.needs_review_count > 0} />
              <Stat label="Contradictions" value={d.contradiction_count} warn={d.contradiction_count > 0} />
              <Stat
                label="Learning velocity"
                value={d.learning_velocity != null ? d.learning_velocity.toFixed(2) : "—"}
                hint="Net direction of confidence movement over the last 7 days, from −1 to +1. Positive = the agent is acquiring/strengthening beliefs; negative = beliefs are being walked back. “—” means no learning signals (feedback/outcomes) in the window yet."
              />
              <Stat
                label="Stability"
                value={d.stability_score != null ? d.stability_score.toFixed(2) : "—"}
                hint="Of the beliefs touched in the last 7 days, the share that held up (helpful/successful/reinforced) vs. were overturned (contradicted/outdated/unhelpful), from 0 to 1. 1.0 = settled knowledge; low = volatile. “—” means no belief-affecting signals yet."
              />
            </div>

            <div className="card" style={{ marginTop: 20 }}>
              <div className="row-between" style={{ marginBottom: 14 }}>
                <h3>Confidence tiers</h3>
                <span className="muted">hot &gt; 0.85 · warm &gt; 0.70 · cold &gt; 0.40 · archive ≤ 0.40</span>
              </div>
              <TierBars counts={d.tier_counts} total={d.total_memories} />
            </div>
          </>
        )}
      </AsyncView>
    </Layout>
  );
}
