import { useState } from "react";
import { Api } from "../api";
import { useAsync } from "../hooks";
import { Layout } from "../components/Layout";
import { AsyncView } from "../components/ui";
import { tierOf } from "../format";

const TYPES = ["fact", "preference", "decision", "constraint"];

export default function Canon() {
  const canon = useAsync(() => Api.listCanon(), []);
  const agents = useAsync(() => Api.listAgents(200, 0), []);
  const [content, setContent] = useState("");
  const [type, setType] = useState("fact");
  const [agentId, setAgentId] = useState("");
  const [busy, setBusy] = useState(false);
  const [msg, setMsg] = useState<string | null>(null);

  async function create(e: React.FormEvent) {
    e.preventDefault();
    setBusy(true);
    setMsg(null);
    try {
      await Api.createCanon(agentId, content.trim(), type);
      setContent("");
      setMsg("Canon fact added — visible to every agent in this org.");
      canon.reload();
    } catch (err) {
      setMsg(err instanceof Error ? err.message : "Failed to add");
    } finally {
      setBusy(false);
    }
  }

  async function remove(id: string) {
    if (!window.confirm("Remove this org-wide fact?")) return;
    try {
      await Api.deleteCanon(id);
      canon.reload();
    } catch (err) {
      setMsg(err instanceof Error ? err.message : "Delete failed");
    }
  }

  return (
    <Layout title="Canon">
      <div className="page-head">
        <h2>Canon — org memory</h2>
        <p>Authoritative, tenant-wide knowledge (policies, catalog, shared facts) every agent in this org can recall.</p>
      </div>

      <div className="card" style={{ marginBottom: 18 }}>
        <form onSubmit={create}>
          <label>New canon fact</label>
          <textarea
            rows={2}
            placeholder="e.g. Refunds are allowed within 30 days of purchase."
            value={content}
            onChange={(e) => setContent(e.target.value)}
          />
          <div className="row wrap" style={{ gap: 10, marginTop: 10, alignItems: "flex-end" }}>
            <div>
              <label>Type</label>
              <select value={type} onChange={(e) => setType(e.target.value)}>
                {TYPES.map((t) => <option key={t} value={t}>{t}</option>)}
              </select>
            </div>
            <div>
              <label>Attributed agent</label>
              <select value={agentId} onChange={(e) => setAgentId(e.target.value)}>
                <option value="">Select an agent…</option>
                {agents.data?.agents.map((a) => (
                  <option key={a.id} value={a.id}>{a.name || a.external_id || a.id}</option>
                ))}
              </select>
            </div>
            <button type="submit" disabled={busy || !content.trim() || !agentId}>
              {busy ? "Adding…" : "Add canon fact"}
            </button>
          </div>
          <p className="muted" style={{ marginTop: 8 }}>
            Canon rows are tenant-wide; an agent is recorded as the author for provenance.
          </p>
        </form>
        {msg && <div className="notice" style={{ marginTop: 12 }}>{msg}</div>}
      </div>

      <AsyncView state={canon} empty={(c) => c.length === 0}>
        {(items) => (
          <div className="card" style={{ padding: 0 }}>
            <table className="table">
              <thead><tr><th>Fact</th><th>Type</th><th>Tier</th><th>Confidence</th><th></th></tr></thead>
              <tbody>
                {items.map((m) => {
                  const t = tierOf(m.confidence);
                  return (
                    <tr key={m.id}>
                      <td style={{ maxWidth: 480 }}>{m.content}</td>
                      <td><span className="pill">{m.type}</span></td>
                      <td><span className={`pill ${t}`}>{t}</span></td>
                      <td className="conf">{m.confidence.toFixed(2)}</td>
                      <td><button className="danger sm" onClick={() => remove(m.id)}>Remove</button></td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
        )}
      </AsyncView>
    </Layout>
  );
}
