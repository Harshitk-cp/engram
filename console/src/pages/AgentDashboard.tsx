import { Link, useParams } from "react-router-dom";
import { Api } from "../api";
import { useAsync } from "../hooks";
import { Layout } from "../components/Layout";
import { AsyncView, Stat, TierBars } from "../components/ui";

export default function AgentDashboard() {
  const { agentId = "" } = useParams();
  const state = useAsync(() => Api.dashboard(agentId), [agentId]);

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
                <Link className="btn" to={`/agents/${agentId}/review`}>
                  Review queue {d.needs_review_count > 0 && <span className="badge">{d.needs_review_count}</span>}
                </Link>
              </div>
            </div>

            <div className="stat-grid">
              <Stat label="Total beliefs" value={d.total_memories.toLocaleString()} />
              <Stat label="Needs review" value={d.needs_review_count} warn={d.needs_review_count > 0} />
              <Stat label="Contradictions" value={d.contradiction_count} warn={d.contradiction_count > 0} />
              <Stat label="Learning velocity" value={d.learning_velocity != null ? d.learning_velocity.toFixed(2) : "—"} />
              <Stat label="Stability" value={d.stability_score != null ? d.stability_score.toFixed(2) : "—"} />
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
