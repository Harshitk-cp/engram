import { useEffect, useMemo, useState } from "react";
import { useAuth } from "../auth";
import { Layout } from "../components/Layout";
import { initials } from "../format";
import { Api, EngineSettings } from "../api";
import { useAsync } from "../hooks";

// Field metadata drives the tuning panel: range, step and a plain-language
// description of what each parameter does to the cognitive engine.
type FieldKey =
  | "decay_base_rate"
  | "decay_floor"
  | "archive_threshold"
  | "competition_weight"
  | "reinforcement_log_odds"
  | "contradiction_log_odds";
interface FieldMeta {
  key: FieldKey;
  label: string;
  desc: string;
  min: number;
  max: number;
  step: number;
  fmt?: (v: number) => string;
}

const FIELDS: FieldMeta[] = [
  {
    key: "decay_base_rate",
    label: "Decay rate (λ)",
    desc: "How fast unused memories lose confidence, per hour. Higher = forgets faster.",
    min: 0,
    max: 0.02,
    step: 0.0005,
    fmt: (v) => v.toFixed(4),
  },
  {
    key: "decay_floor",
    label: "Decay floor",
    desc: "The lowest confidence decay alone can drive a memory to — it never fully disappears from forgetting.",
    min: 0,
    max: 0.9,
    step: 0.01,
    fmt: (v) => v.toFixed(2),
  },
  {
    key: "archive_threshold",
    label: "Archive threshold",
    desc: "Memories that fall below this confidence are archived out of active recall.",
    min: 0,
    max: 0.9,
    step: 0.01,
    fmt: (v) => v.toFixed(2),
  },
  {
    key: "competition_weight",
    label: "Competition weight",
    desc: "How strongly similar, higher-confidence memories suppress a competing one (interference). 0 disables it.",
    min: 0,
    max: 3,
    step: 0.1,
    fmt: (v) => v.toFixed(2),
  },
  {
    key: "reinforcement_log_odds",
    label: "Reinforcement Δ",
    desc: "Confidence boost (in log-odds) when a memory is recalled or confirmed helpful. Higher = learns faster.",
    min: 0,
    max: 2,
    step: 0.05,
    fmt: (v) => v.toFixed(2),
  },
  {
    key: "contradiction_log_odds",
    label: "Contradiction Δ",
    desc: "Confidence penalty (in log-odds) when a memory is contradicted. Higher = unlearns faster.",
    min: 0,
    max: 2,
    step: 0.05,
    fmt: (v) => v.toFixed(2),
  },
];

function EngineTuning() {
  const { me } = useAuth();
  const remote = useAsync(() => Api.getSettings(), []);
  const [form, setForm] = useState<EngineSettings | null>(null);
  const [saving, setSaving] = useState(false);
  const [err, setErr] = useState<string | null>(null);
  const [saved, setSaved] = useState(false);

  // Seed the editable form whenever the server values (re)load.
  useEffect(() => {
    if (remote.data) setForm({ ...remote.data.settings });
  }, [remote.data]);

  const defaults = remote.data?.defaults ?? null;
  const activeRole = me?.orgs.find((o) => o.tenant_id === me.active_tenant_id)?.role;
  const canEdit = activeRole === "owner" || activeRole === "admin";

  const dirty = useMemo(() => {
    if (!form || !remote.data) return false;
    const r = remote.data.settings;
    if (FIELDS.some((f) => form[f.key] !== r[f.key])) return true;
    if (!!form.firewall_enabled !== !!r.firewall_enabled) return true;
    const a = (form.quarantine_provenances ?? []).slice().sort().join(",");
    const b = (r.quarantine_provenances ?? []).slice().sort().join(",");
    return a !== b;
  }, [form, remote.data]);

  const set = (key: FieldKey, v: number) => {
    setSaved(false);
    setForm((prev) => (prev ? { ...prev, [key]: v } : prev));
  };

  const save = async () => {
    if (!form) return;
    setErr(null);
    setSaving(true);
    try {
      const res = await Api.updateSettings(form);
      setForm({ ...res.settings });
      remote.reload();
      setSaved(true);
      setTimeout(() => setSaved(false), 2000);
    } catch (e) {
      setErr(e instanceof Error ? e.message : "Failed to save settings");
    } finally {
      setSaving(false);
    }
  };

  const resetToDefaults = () => {
    if (defaults) {
      setSaved(false);
      setForm({ ...defaults });
    }
  };

  const revert = () => {
    if (remote.data) setForm({ ...remote.data.settings });
    setSaved(false);
    setErr(null);
  };

  if (remote.loading && !form) return <div className="card" style={{ maxWidth: 720 }}>Loading engine settings…</div>;
  if (remote.error) return <div className="card error-box" style={{ maxWidth: 720 }}>{remote.error}</div>;
  if (!form) return null;

  return (
    <div className="card" style={{ maxWidth: 720 }}>
      <div className="row-between" style={{ marginBottom: 6 }}>
        <div>
          <div style={{ fontWeight: 600, fontSize: "1.02rem" }}>Engine tuning</div>
          <p className="muted" style={{ margin: "4px 0 0", fontSize: "0.84rem" }}>
            Per-organization parameters for the cognitive engine. Applied to all agents in this org. Out-of-range values are clamped on save.
          </p>
        </div>
      </div>

      {!canEdit && (
        <p className="muted" style={{ fontSize: "0.8rem", marginTop: 8 }}>
          You have <strong>{activeRole || "member"}</strong> access — these values are read-only. An owner or admin can change them.
        </p>
      )}

      <div className="grid" style={{ gap: 20, marginTop: 18 }}>
        {FIELDS.map((f) => {
          const val = form[f.key];
          const def = defaults ? defaults[f.key] : val;
          const isDefault = val === def;
          const fmt = f.fmt ?? ((v: number) => String(v));
          return (
            <div key={f.key}>
              <div className="row-between" style={{ alignItems: "baseline" }}>
                <label style={{ margin: 0 }}>{f.label}</label>
                <div className="row" style={{ gap: 8, alignItems: "baseline" }}>
                  <code style={{ fontSize: "0.92rem" }}>{fmt(val)}</code>
                  {!isDefault && (
                    <span className="muted" style={{ fontSize: "0.72rem" }}>default {fmt(def)}</span>
                  )}
                </div>
              </div>
              <input
                type="range"
                min={f.min}
                max={f.max}
                step={f.step}
                value={val}
                disabled={!canEdit}
                onChange={(e) => set(f.key, parseFloat(e.target.value))}
                style={{ width: "100%", marginTop: 6 }}
              />
              <p className="muted" style={{ margin: "4px 0 0", fontSize: "0.78rem" }}>{f.desc}</p>
            </div>
          );
        })}
      </div>

      {/* Provenance Firewall */}
      <div style={{ marginTop: 26, paddingTop: 20, borderTop: "1px solid var(--border)" }}>
        <div className="row-between" style={{ alignItems: "baseline" }}>
          <label style={{ margin: 0 }}>Provenance Firewall</label>
          <label className="row" style={{ gap: 8, fontSize: "0.85rem", cursor: canEdit ? "pointer" : "default" }}>
            <input
              type="checkbox"
              checked={!!form.firewall_enabled}
              disabled={!canEdit}
              onChange={(e) => { setSaved(false); setForm((p) => p ? { ...p, firewall_enabled: e.target.checked } : p); }}
            />
            {form.firewall_enabled ? "Enabled" : "Disabled"}
          </label>
        </div>
        <p className="muted" style={{ margin: "4px 0 10px", fontSize: "0.78rem" }}>
          Hold untrusted writes in quarantine — out of recall and belief logic — until reviewed. Defends against memory poisoning (OWASP ASI06). Callers can also quarantine any single write explicitly.
        </p>
        <div style={{ opacity: form.firewall_enabled ? 1 : 0.45, pointerEvents: form.firewall_enabled && canEdit ? "auto" : "none" }}>
          <div className="stat-label" style={{ margin: "0 0 6px" }}>Auto-quarantine these provenances</div>
          <div className="row" style={{ gap: 8, flexWrap: "wrap" }}>
            {["user", "agent", "tool", "derived", "inferred"].map((p) => {
              const on = (form.quarantine_provenances ?? []).includes(p);
              return (
                <button
                  type="button"
                  key={p}
                  className={on ? "sm" : "secondary sm"}
                  onClick={() => {
                    setSaved(false);
                    setForm((prev) => {
                      if (!prev) return prev;
                      const cur = prev.quarantine_provenances ?? [];
                      const next = cur.includes(p) ? cur.filter((x) => x !== p) : [...cur, p];
                      return { ...prev, quarantine_provenances: next };
                    });
                  }}
                >
                  {p}
                </button>
              );
            })}
          </div>
          <p className="muted" style={{ marginTop: 8, fontSize: "0.76rem" }}>
            Typical: quarantine <code>inferred</code> (and maybe <code>agent</code>) model-generated content while trusting <code>user</code>/<code>tool</code> input. Review the queue from an agent’s Knowledge Health page.
          </p>
        </div>
      </div>

      {err && <div className="error-box" style={{ marginTop: 16 }}>{err}</div>}

      {canEdit && (
        <div className="row" style={{ gap: 10, marginTop: 20 }}>
          <button disabled={!dirty || saving} onClick={save}>
            {saving ? "Saving…" : saved ? "Saved ✓" : "Save changes"}
          </button>
          <button className="secondary" disabled={!dirty || saving} onClick={revert}>
            Revert
          </button>
          <button className="secondary" disabled={saving} onClick={resetToDefaults}>
            Reset to defaults
          </button>
        </div>
      )}
    </div>
  );
}

export default function Settings() {
  const { me } = useAuth();
  if (!me) return null;
  return (
    <Layout title="Settings">
      <div className="page-head">
        <h2>Settings</h2>
        <p>Your account, organizations, and cognitive engine tuning.</p>
      </div>

      <div className="card" style={{ maxWidth: 720 }}>
        <div className="row" style={{ gap: 14 }}>
          <span className="avatar" style={{ width: 44, height: 44, fontSize: "1rem" }}>
            {initials(me.user.name || me.user.email)}
          </span>
          <div>
            <div style={{ fontWeight: 600, fontSize: "1.05rem" }}>{me.user.name || "—"}</div>
            <div className="secondary">{me.user.email}</div>
          </div>
        </div>
      </div>

      <h3 style={{ margin: "28px 0 12px" }}>Organizations</h3>
      <div className="card" style={{ maxWidth: 720, padding: 0 }}>
        <table className="table">
          <thead><tr><th>Org</th><th>Role</th><th>Active</th></tr></thead>
          <tbody>
            {me.orgs.map((o) => (
              <tr key={o.tenant_id}>
                <td>{o.tenant_name}</td>
                <td><span className="pill">{o.role}</span></td>
                <td>{o.tenant_id === me.active_tenant_id ? <span className="badge">active</span> : ""}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>

      <h3 style={{ margin: "28px 0 12px" }}>Embeddings</h3>
      <EmbeddingInfo />

      <h3 style={{ margin: "28px 0 12px" }}>Cognitive engine</h3>
      <EngineTuning />
    </Layout>
  );
}

function EmbeddingInfo() {
  const info = useAsync(() => Api.embeddingInfo(), []);
  if (info.loading) return <div className="card" style={{ maxWidth: 720 }}>Loading embedding config…</div>;
  if (info.error || !info.data) return <div className="card" style={{ maxWidth: 720 }} />;
  const d = info.data;
  return (
    <div className="card" style={{ maxWidth: 720 }}>
      <div className="row" style={{ gap: 28, flexWrap: "wrap" }}>
        <div><div className="stat-label">Provider</div><div style={{ fontWeight: 600 }}>{d.provider}</div></div>
        <div><div className="stat-label">Model</div><div style={{ fontWeight: 600 }}>{d.model}</div></div>
        <div><div className="stat-label">Dimension</div><div style={{ fontWeight: 600 }}>{d.dimension}</div></div>
        {d.base_url && <div><div className="stat-label">Endpoint</div><div className="mono" style={{ fontSize: "0.82rem" }}>{d.base_url}</div></div>}
      </div>
      <p className="muted" style={{ marginTop: 12, fontSize: "0.8rem" }}>
        The embedding model is a deployment-level choice shared by all tenants (the vector width is fixed in the database). Set it with <code>EMBEDDING_PROVIDER</code> / <code>EMBEDDING_MODEL</code> / <code>EMBEDDING_DIM</code>. OpenAI, any OpenAI-compatible endpoint (Ollama, vLLM, TEI), or a local server are supported.
      </p>
    </div>
  );
}
