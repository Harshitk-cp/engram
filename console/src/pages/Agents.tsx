import { useState } from "react";
import { Link, useNavigate } from "react-router-dom";
import { Api } from "../api";
import { useAsync } from "../hooks";
import { Layout } from "../components/Layout";
import { AsyncView, Pager } from "../components/ui";

const PAGE = 24;

export default function Agents() {
  const nav = useNavigate();
  const [offset, setOffset] = useState(0);
  const [query, setQuery] = useState("");
  const state = useAsync(() => Api.listAgents(PAGE, offset), [offset]);

  const [creating, setCreating] = useState(false);
  const [extId, setExtId] = useState("");
  const [name, setName] = useState("");
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState<string | null>(null);

  async function create(e: React.FormEvent) {
    e.preventDefault();
    setBusy(true);
    setErr(null);
    try {
      const agent = await Api.createAgent(name.trim() || extId.trim(), extId.trim() || undefined);
      nav(`/agents/${agent.id}`);
    } catch (e2) {
      setErr(e2 instanceof Error ? e2.message : "Failed to create agent");
    } finally {
      setBusy(false);
    }
  }

  return (
    <Layout title="Agents">
      <div className="page-head row-between wrap">
        <div>
          <h2>Agents</h2>
          <p>Each agent has its own memory: beliefs, confidence, and contradictions.</p>
        </div>
        <button onClick={() => setCreating((v) => !v)}>{creating ? "Cancel" : "New agent"}</button>
      </div>

      {creating && (
        <div className="card" style={{ marginBottom: 16 }}>
          <form onSubmit={create}>
            <div className="row wrap" style={{ alignItems: "flex-end", gap: 14 }}>
              <div style={{ flex: 1, minWidth: 200 }}>
                <label>Display name</label>
                <input placeholder="e.g. Claude Desktop Agent" value={name} onChange={(e) => setName(e.target.value)} autoFocus />
              </div>
              <div style={{ flex: 1, minWidth: 200 }}>
                <label>External ID (optional — your stable identifier)</label>
                <input placeholder="auto-generated from name if blank" value={extId} onChange={(e) => setExtId(e.target.value)} />
              </div>
              <button type="submit" disabled={busy || !(name.trim() || extId.trim())}>{busy ? "Creating…" : "Create"}</button>
            </div>
            {err && <div className="error-box" style={{ marginTop: 12 }}>{err}</div>}
          </form>
        </div>
      )}

      <div className="filters">
        <input className="search" placeholder="Filter agents on this page…" value={query} onChange={(e) => setQuery(e.target.value)} />
      </div>

      <AsyncView state={state} empty={(d) => d.agents.length === 0}>
        {(data) => {
          const agents = data.agents.filter(
            (a) =>
              !query ||
              a.name.toLowerCase().includes(query.toLowerCase()) ||
              (a.external_id || "").toLowerCase().includes(query.toLowerCase())
          );
          return (
            <>
              <div className="cards">
                {agents.map((a) => (
                  <Link key={a.id} to={`/agents/${a.id}`} className="card clickable">
                    <div className="card-title">{a.name || "Unnamed agent"}</div>
                    <div className="muted mono" style={{ marginTop: 6 }}>{a.external_id || a.id}</div>
                  </Link>
                ))}
              </div>
              <Pager offset={offset} limit={PAGE} total={data.total} onPage={setOffset} />
            </>
          );
        }}
      </AsyncView>
    </Layout>
  );
}
