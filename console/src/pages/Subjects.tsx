import { useState } from "react";
import { Link } from "react-router-dom";
import { Api } from "../api";
import { useAsync } from "../hooks";
import { Layout } from "../components/Layout";
import { AsyncView } from "../components/ui";

export default function Subjects() {
  const state = useAsync(() => Api.anchors(200), []);
  const [busy, setBusy] = useState<string | null>(null);
  const [msg, setMsg] = useState<string | null>(null);

  async function shred(id: string, name: string) {
    const reason = window.prompt(`Crypto-shred all data for "${name}"?\n\nThis encrypts every memory about this subject (and anything derived from them) under a discarded key — irreversible. The audit trail keeps proof the data existed and was erased.\n\nReason (recorded in the audit chain):`);
    if (!reason) return;
    setBusy(id); setMsg(null);
    try {
      const r = await Api.shredSubject(id, reason);
      setMsg(`Crypto-shredded ${r.shredded} record(s) for "${name}", plus derived beliefs, links and the subject's identity. Recorded in the audit chain.`);
      state.reload();
    } catch (e) {
      setMsg(e instanceof Error ? e.message : "Shred failed");
    } finally { setBusy(null); }
  }

  async function purge(id: string, name: string) {
    if (!window.confirm(`Hard-purge "${name}"?\n\nThis permanently DELETES every memory about this subject, everything derived from them, and the subject record itself. A redaction record remains in the audit chain. This cannot be undone.`)) return;
    setBusy(id); setMsg(null);
    try {
      const r = await Api.purgeSubject(id);
      setMsg(`Purged "${name}" — ${r.memories_purged} memory record(s) and the subject deleted. Recorded in the audit chain.`);
      state.reload();
    } catch (e) {
      setMsg(e instanceof Error ? e.message : "Purge failed");
    } finally { setBusy(null); }
  }

  return (
    <Layout title="Subjects">
      <div className="page-head">
        <h2>Subjects &amp; right-to-erasure</h2>
        <p>
          Subjects (anchors) are the people/accounts memories are bound to. GDPR Art. 17 erasure cascades to derived
          beliefs, entity links, graph edges and the subject's own identity — and every erasure is provable in the{" "}
          <Link className="link" to="/audit">audit chain</Link>.
        </p>
      </div>
      {msg && <div className="notice" style={{ marginBottom: 14 }}>{msg}</div>}

      <AsyncView state={state} empty={(d) => d.length === 0}>
        {(anchors) => (
          <div className="card" style={{ padding: 0 }}>
            <table className="table">
              <thead>
                <tr><th>Subject</th><th>Type</th><th>External ID</th><th>Created</th><th style={{ textAlign: "right" }}>Erasure</th></tr>
              </thead>
              <tbody>
                {anchors.map((a) => (
                  <tr key={a.id}>
                    <td>
                      <div style={{ fontWeight: 600 }}>{a.name}</div>
                      <div className="mono" style={{ fontSize: "0.72rem", opacity: 0.6 }}>{a.id}</div>
                    </td>
                    <td><span className="pill">{a.entity_type}</span></td>
                    <td className="mono" style={{ fontSize: "0.8rem" }}>{a.external_id || "—"}</td>
                    <td className="muted" style={{ fontSize: "0.8rem" }}>{a.created_at ? new Date(a.created_at).toLocaleDateString() : "—"}</td>
                    <td style={{ textAlign: "right", whiteSpace: "nowrap" }}>
                      <button className="secondary sm" disabled={busy === a.id} onClick={() => shred(a.id, a.name)}>Crypto-shred</button>{" "}
                      <button className="danger sm" disabled={busy === a.id} onClick={() => purge(a.id, a.name)}>Purge</button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </AsyncView>

      <p className="muted" style={{ marginTop: 14, fontSize: "0.8rem" }}>
        <strong>Crypto-shred</strong> keeps the rows but makes content permanently unrecoverable (best for an append-only audit trail).{" "}
        <strong>Purge</strong> hard-deletes the rows. Both erase derived data and require an admin role.
      </p>
    </Layout>
  );
}
