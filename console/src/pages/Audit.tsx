import { useCallback, useEffect, useRef, useState } from "react";
import { Api, AuditStatus, ChainEntry } from "../api";
import { useAsync } from "../hooks";
import { Layout } from "../components/Layout";
import { AsyncView } from "../components/ui";
import { dateTime } from "../format";

function shortHash(h?: string) {
  if (!h) return "—";
  return h.length > 16 ? `${h.slice(0, 10)}…${h.slice(-6)}` : h;
}

const sleep = (ms: number) => new Promise((r) => setTimeout(r, ms));

export default function Audit() {
  const agents = useAsync(() => Api.listAgents(200, 0), []);
  const [agentId, setAgentId] = useState("");
  // Newest-first: an audit log is read most-recent-down, and the chain can be
  // very long, so we show the latest window rather than the whole thing.
  const chain = useAsync(() => Api.auditChain(agentId || undefined, 50), [agentId]);

  const [status, setStatus] = useState<AuditStatus | null>(null);
  const [verifying, setVerifying] = useState(false);
  const [flash, setFlash] = useState(false);
  const [copied, setCopied] = useState(false);
  const mounted = useRef(true);

  useEffect(() => {
    Api.auditVerify().then((s) => mounted.current && setStatus(s)).catch(() => {});
    return () => { mounted.current = false; };
  }, []);

  const verify = useCallback(async () => {
    setVerifying(true);
    const t0 = Date.now();
    const result = await Api.auditVerify().catch(() => null);
    const elapsed = Date.now() - t0;
    if (elapsed < 650) await sleep(650 - elapsed); // let the sweep read even on a fast server
    if (!mounted.current) return;
    if (result) setStatus(result);
    setVerifying(false);
    setFlash(true);
    setTimeout(() => mounted.current && setFlash(false), 1300);
  }, []);

  const copyHead = async () => {
    if (!status?.head_hash) return;
    await navigator.clipboard.writeText(status.head_hash);
    setCopied(true);
    setTimeout(() => setCopied(false), 1400);
  };

  const intact = status?.valid;
  const heroClass = verifying ? "verifying" : intact === false ? "broken" : intact ? "ok" : "";

  return (
    <Layout title="Audit trail">
      {/* ── Trust hero ── */}
      <div className={`audit-hero ${heroClass} ${flash ? "flash" : ""}`}>
        {verifying && <div className="audit-sweep" />}
        <div className="audit-hero-main">
          <div className={`audit-shield ${verifying ? "scanning" : intact === false ? "broken" : intact ? "ok" : ""}`}>
            {verifying ? "⟳" : intact === false ? "✕" : "✓"}
          </div>
          <div>
            <h2 style={{ marginBottom: 4 }}>
              {verifying
                ? "Recomputing hash chain…"
                : status == null
                ? "Audit trail"
                : intact
                ? "Tamper-evident chain intact"
                : "Tampering detected"}
            </h2>
            <p className="secondary" style={{ margin: 0 }}>
              {status == null
                ? "Every belief change is sealed into a per-tenant SHA-256 hash chain."
                : intact
                ? `${status.checked.toLocaleString()} records verified end-to-end — the whole chain re-hashed on the server.`
                : `Chain breaks at record #${status.break_seq}.`}
            </p>
          </div>
        </div>
        <div className="row">
          <button onClick={verify} disabled={verifying}>{verifying ? "Verifying…" : "Verify chain"}</button>
          <a className="btn secondary" href="/v1/audit/export">Export (signed)</a>
        </div>
      </div>

      {/* ── Terminal verdict ── */}
      <div className="audit-term" aria-live="polite">
        <span className="audit-term-prompt">$</span>
        <span className="audit-term-cmd">GET /v1/audit/verify</span>
        <span className="audit-term-arrow">→</span>
        {verifying ? (
          <span className="audit-term-dim">recomputing SHA-256 chain…</span>
        ) : status == null ? (
          <span className="audit-term-dim">click “Verify chain” to recompute</span>
        ) : intact ? (
          <span className="audit-term-ok">{`{ "valid": true, "entries": ${status.checked} }`}</span>
        ) : (
          <span className="audit-term-bad">{`{ "valid": false, "broken_at": ${status.break_seq} }`}</span>
        )}
      </div>

      {status && (
        <div className="stat-grid" style={{ marginTop: 16 }}>
          <div className="stat"><div className="stat-value">{status.head_seq.toLocaleString()}</div><div className="stat-label">Records in chain</div></div>
          <div className="stat"><div className="stat-value">{status.checked.toLocaleString()}</div><div className="stat-label">Verified now</div></div>
          <div className="stat"><div className="stat-value">{status.signed ? "HMAC ✓" : "unsigned"}</div><div className="stat-label">Export signing</div></div>
          <div className="stat audit-head">
            <div className="stat-label" style={{ marginTop: 0, marginBottom: 6 }}>Chain head</div>
            <button className="link-btn mono" title="Copy full head hash" onClick={copyHead}>
              {copied ? "copied ✓" : shortHash(status.head_hash) || "— empty"}
            </button>
          </div>
        </div>
      )}

      {/* ── Chain records ── */}
      <div className="page-head row-between wrap" style={{ marginTop: 30, marginBottom: 6 }}>
        <div>
          <h3 style={{ margin: 0 }}>Recent records</h3>
          {status && (
            <p className="muted" style={{ margin: "4px 0 0" }}>
              Latest {Math.min(50, status.head_seq)} of {status.head_seq.toLocaleString()} in the chain
            </p>
          )}
        </div>
        <select value={agentId} onChange={(e) => setAgentId(e.target.value)} style={{ width: "auto", minWidth: 180 }}>
          <option value="">All agents</option>
          {(agents.data?.agents ?? []).map((a) => (
            <option key={a.id} value={a.id}>{a.name || a.id}</option>
          ))}
        </select>
      </div>

      <AsyncView state={chain}>
        {(list) =>
          list.length === 0 ? (
            <div className="card muted">No audit records yet. Mutations (reinforce, decay, contradict, redact) are sealed here as they happen.</div>
          ) : (
            <div className="chain2">
              {list.map((e) => <ChainBlock key={e.id} e={e} />)}
            </div>
          )
        }
      </AsyncView>
    </Layout>
  );
}

function ChainBlock({ e }: { e: ChainEntry }) {
  const delta =
    e.old_confidence != null && e.new_confidence != null
      ? { up: e.new_confidence >= e.old_confidence, text: `${e.old_confidence.toFixed(2)} → ${e.new_confidence.toFixed(2)}` }
      : null;
  return (
    <div className="chain2-row">
      <div className="chain2-rail">
        <span className="chain2-seq">#{e.seq}</span>
        <span className="chain2-node" />
      </div>
      <div className="chain2-card">
        <div className="chain2-top">
          <span className={`tag ${e.mutation_type}`}>{e.mutation_type}</span>
          {delta && <span className={`chain2-delta ${delta.up ? "up" : "down"}`}>{delta.text} {delta.up ? "↑" : "↓"}</span>}
          <span className="chain2-time">{dateTime(e.created_at)}</span>
        </div>
        {e.reason && <div className="chain2-reason">{e.reason}</div>}
        <div className="chain2-hashes">
          <span className="chain2-h"><em>prev</em> <code>{shortHash(e.prev_hash)}</code></span>
          <span className="chain2-arrow">⛓</span>
          <span className="chain2-h"><em>hash</em> <code>{shortHash(e.row_hash)}</code></span>
          {e.memory_id && <span className="chain2-mem">memory {e.memory_id.slice(0, 8)}</span>}
        </div>
      </div>
    </div>
  );
}
