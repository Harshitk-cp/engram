import { useState } from "react";
import { Api } from "../api";
import { useAsync } from "../hooks";
import { Layout } from "../components/Layout";
import { AsyncView } from "../components/ui";
import { relativeTime } from "../format";

export default function Keys() {
  const state = useAsync(() => Api.listKeys(), []);
  const [name, setName] = useState("");
  const [scopes, setScopes] = useState<string[]>(["read", "write"]);
  const [newKey, setNewKey] = useState<string | null>(null);
  const [msg, setMsg] = useState<string | null>(null);

  function toggleScope(s: string) {
    setScopes((cur) => (cur.includes(s) ? cur.filter((x) => x !== s) : [...cur, s]));
  }

  async function create(e: React.FormEvent) {
    e.preventDefault();
    setMsg(null);
    try {
      const created = await Api.createKey(name || "console key", scopes);
      setNewKey(created.api_key ?? null);
      setName("");
      state.reload();
    } catch (err) {
      setMsg(err instanceof Error ? err.message : "Create failed");
    }
  }

  async function revoke(id: string) {
    if (!window.confirm("Revoke this key? Apps using it will lose access immediately.")) return;
    try {
      await Api.revokeKey(id);
      state.reload();
    } catch (err) {
      setMsg(err instanceof Error ? err.message : "Revoke failed");
    }
  }

  return (
    <Layout title="API Keys">
      <div className="page-head">
        <h2>API Keys</h2>
        <p>Keys authenticate your agents and apps against the data-plane API for this org.</p>
      </div>

      <div className="card" style={{ marginBottom: 18 }}>
        <form onSubmit={create}>
          <div className="row wrap" style={{ alignItems: "flex-end", gap: 14 }}>
            <div style={{ flex: 1, minWidth: 220 }}>
              <label>Key name</label>
              <input placeholder="e.g. production-api" value={name} onChange={(e) => setName(e.target.value)} />
            </div>
            <div>
              <label>Scopes</label>
              <div className="row">
                {["read", "write", "admin"].map((s) => (
                  <button type="button" key={s} className={scopes.includes(s) ? "sm" : "secondary sm"} onClick={() => toggleScope(s)}>
                    {s}
                  </button>
                ))}
              </div>
            </div>
            <button type="submit">Create key</button>
          </div>
        </form>
        {msg && <div className="error-box" style={{ marginTop: 12 }}>{msg}</div>}
        {newKey && (
          <div className="notice" style={{ marginTop: 14 }}>
            <strong>Copy this key now — it won't be shown again:</strong>
            <div className="mono" style={{ marginTop: 8, padding: 10, background: "var(--bg-primary)", borderRadius: 8, wordBreak: "break-all" }}>{newKey}</div>
          </div>
        )}
      </div>

      <AsyncView state={state} empty={(keys) => keys.length === 0}>
        {(keys) => (
          <div className="card" style={{ padding: 0 }}>
            <table className="table">
              <thead><tr><th>Name</th><th>Prefix</th><th>Scopes</th><th>Created by</th><th>Last used</th><th></th></tr></thead>
              <tbody>
                {keys.map((k) => (
                  <tr key={k.id}>
                    <td>{k.name || "—"}</td>
                    <td className="mono">{k.key_prefix}…</td>
                    <td>{k.scopes.map((s) => <span key={s} className="pill" style={{ marginRight: 4 }}>{s}</span>)}</td>
                    <td className="muted">{k.created_by_email || <span className="conf">system</span>}</td>
                    <td className="muted">{k.last_used_at ? relativeTime(k.last_used_at) : "never"}</td>
                    <td><button className="danger sm" onClick={() => revoke(k.id)}>Revoke</button></td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </AsyncView>
    </Layout>
  );
}
