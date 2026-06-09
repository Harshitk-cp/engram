import { Api } from "../api";
import { useAsync } from "../hooks";
import { Layout } from "../components/Layout";
import { AsyncView } from "../components/ui";

export default function Audit() {
  const state = useAsync(() => Api.auditVerify(), []);

  return (
    <Layout title="Audit trail">
      <div className="page-head row-between wrap">
        <div>
          <h2>Audit trail</h2>
          <p>Every belief change is recorded in a tamper-evident, hash-chained log.</p>
        </div>
        <div className="row">
          <button className="secondary" onClick={() => state.reload()}>Re-verify</button>
          <a className="btn" href="/v1/audit/export">Export (signed)</a>
        </div>
      </div>

      <AsyncView state={state}>
        {(s) => (
          <>
            <div className={`card ${s.valid ? "" : ""}`} style={{ borderColor: s.valid ? "var(--success)" : "var(--danger)" }}>
              <div className="row" style={{ gap: 12 }}>
                <span
                  className="badge"
                  style={{
                    background: s.valid ? "rgba(74,222,128,0.12)" : "rgba(251,113,133,0.12)",
                    color: s.valid ? "var(--success)" : "var(--danger)",
                    borderColor: s.valid ? "rgba(74,222,128,0.3)" : "rgba(251,113,133,0.3)",
                  }}
                >
                  {s.valid ? "✓ Chain intact" : "✕ Chain broken"}
                </span>
                <span className="secondary">
                  {s.valid
                    ? `${s.checked.toLocaleString()} records verified`
                    : `tamper detected at record #${s.break_seq}`}
                </span>
              </div>
            </div>

            <div className="stat-grid" style={{ marginTop: 16 }}>
              <Stat label="Records in chain" value={s.head_seq.toLocaleString()} />
              <Stat label="Verified now" value={s.checked.toLocaleString()} />
              <Stat label="Export signing" value={s.signed ? "HMAC ✓" : "unsigned"} />
            </div>

            <div className="card" style={{ marginTop: 16 }}>
              <div className="stat-label" style={{ marginBottom: 8 }}>Chain head (anchor this value)</div>
              <div className="mono" style={{ wordBreak: "break-all" }}>{s.head_hash || "— (empty chain)"}</div>
              <p className="muted" style={{ marginTop: 10 }}>
                The head hash fingerprints the entire history. Record it externally (or use the signed export);
                if any past record is altered, re-verification fails at that record.
              </p>
            </div>
          </>
        )}
      </AsyncView>
    </Layout>
  );
}

function Stat({ label, value }: { label: string; value: React.ReactNode }) {
  return (
    <div className="stat">
      <div className="stat-value">{value}</div>
      <div className="stat-label">{label}</div>
    </div>
  );
}
