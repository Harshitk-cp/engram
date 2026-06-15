import { useState } from "react";
import { Link, useParams } from "react-router-dom";
import { Api } from "../api";
import { useAsync } from "../hooks";
import { Layout } from "../components/Layout";
import { AsyncView, Pager } from "../components/ui";
import { tierOf, displayContent } from "../format";

const PAGE = 25;
const TYPES = ["", "fact", "preference", "decision", "constraint"];
const TIERS = ["", "hot", "warm", "cold", "archive"];
// value -> label. "agent" provenance == the assistant.
const SOURCES: [string, string][] = [
  ["", "All sources"],
  ["user", "From the user"],
  ["agent", "From the assistant"],
  ["inferred", "Inferred"],
  ["tool", "From a tool"],
  ["derived", "Derived"],
];
// value -> label. Binding = the memory's scope.
const BINDINGS: [string, string][] = [
  ["", "All scopes"],
  ["private", "Private (this agent)"],
  ["anchored", "Anchored (a subject)"],
  ["session", "Session"],
  ["canon", "Canon (org-wide)"],
];

function sourceLabel(p?: string) {
  if (p === "user") return "user";
  if (p === "agent") return "assistant";
  return p || "—";
}

export default function Memories() {
  const { agentId = "" } = useParams();
  const [tier, setTier] = useState("");
  const [type, setType] = useState("");
  const [provenance, setProvenance] = useState("");
  const [binding, setBinding] = useState("");
  const [offset, setOffset] = useState(0);
  const state = useAsync(
    () => Api.memories(agentId, { tier, type, provenance, binding, limit: PAGE, offset }),
    [agentId, tier, type, provenance, binding, offset]
  );

  return (
    <Layout title={<><Link to="/agents">Agents</Link> / <Link to={`/agents/${agentId}`}>Agent</Link> / Beliefs</>}>
      <div className="page-head">
        <h2>Beliefs</h2>
        <p>Everything this agent believes, with calibrated confidence and tier.</p>
      </div>

      <div className="filters">
        <select value={tier} onChange={(e) => { setOffset(0); setTier(e.target.value); }}>
          {TIERS.map((t) => <option key={t} value={t}>{t ? `Tier: ${t}` : "All tiers"}</option>)}
        </select>
        <select value={type} onChange={(e) => { setOffset(0); setType(e.target.value); }}>
          {TYPES.map((t) => <option key={t} value={t}>{t ? `Type: ${t}` : "All types"}</option>)}
        </select>
        <select value={provenance} onChange={(e) => { setOffset(0); setProvenance(e.target.value); }}>
          {SOURCES.map(([v, label]) => <option key={v} value={v}>{label}</option>)}
        </select>
        <select value={binding} onChange={(e) => { setOffset(0); setBinding(e.target.value); }}>
          {BINDINGS.map(([v, label]) => <option key={v} value={v}>{label}</option>)}
        </select>
      </div>

      <AsyncView state={state} empty={(d) => d.items.length === 0}>
        {(d) => (
          <>
            <div className="card" style={{ padding: 0 }}>
              <table className="table">
                <thead>
                  <tr><th>Belief</th><th>Source</th><th>Type</th><th>Tier</th><th>Confidence</th><th></th></tr>
                </thead>
                <tbody>
                  {d.items.map((m) => {
                    const t = tierOf(m.confidence);
                    return (
                      <tr key={m.id}>
                        <td style={{ maxWidth: 420 }}>{displayContent(m.content)}</td>
                        <td><span className="pill">{sourceLabel(m.provenance)}</span></td>
                        <td><span className="pill">{m.type}</span></td>
                        <td><span className={`pill ${t}`}>{t}</span></td>
                        <td className="conf">{m.confidence.toFixed(2)}</td>
                        <td><Link className="link" to={`/memories/${m.id}`}>History →</Link></td>
                      </tr>
                    );
                  })}
                </tbody>
              </table>
            </div>
            <Pager offset={offset} limit={PAGE} total={d.total} onPage={setOffset} />
          </>
        )}
      </AsyncView>
    </Layout>
  );
}
