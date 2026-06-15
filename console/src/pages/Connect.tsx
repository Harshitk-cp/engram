import { useState } from "react";
import { Api } from "../api";
import { useAsync } from "../hooks";
import { Layout } from "../components/Layout";

const SCOPES = ["read", "write", "admin"];

export default function Connect() {
  const agents = useAsync(() => Api.listAgents(200, 0), []);

  const [mode, setMode] = useState<"existing" | "new">("existing");
  const [agentId, setAgentId] = useState("");
  const [newName, setNewName] = useState("");
  const [scopes, setScopes] = useState<string[]>(["read", "write"]);
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState<string | null>(null);
  const [result, setResult] = useState<{ apiKey: string; agentId: string; agentName: string } | null>(null);
  const [copied, setCopied] = useState(false);

  const toggleScope = (s: string) =>
    setScopes((prev) => (prev.includes(s) ? prev.filter((x) => x !== s) : [...prev, s]));

  const generate = async () => {
    setErr(null);
    setBusy(true);
    try {
      let id = agentId;
      let name = "";
      if (mode === "new") {
        const created = await Api.createAgent(newName.trim());
        id = created.id;
        name = created.name;
      } else {
        if (!id) throw new Error("Pick an agent");
        name = (agents.data?.agents ?? []).find((a) => a.id === id)?.name || id;
      }
      if (scopes.length === 0) throw new Error("Select at least one scope");
      const key = await Api.createKey(`mcp-${name}`, scopes);
      if (!key.api_key) throw new Error("Key was created but the secret was not returned");
      setResult({ apiKey: key.api_key, agentId: id, agentName: name });
    } catch (e) {
      setErr(e instanceof Error ? e.message : "Failed to generate connection");
    } finally {
      setBusy(false);
    }
  };

  const apiUrl = window.location.origin;
  const config = result
    ? JSON.stringify(
        {
          mcpServers: {
            engram: {
              command: "engram-mcp",
              args: ["--transport", "stdio"],
              env: {
                ENGRAM_API_URL: apiUrl,
                ENGRAM_API_KEY: result.apiKey,
                ENGRAM_AGENT_ID: result.agentId,
              },
            },
          },
        },
        null,
        2
      )
    : "";

  const copy = async () => {
    await navigator.clipboard.writeText(config);
    setCopied(true);
    setTimeout(() => setCopied(false), 1500);
  };

  return (
    <Layout title="Connect">
      <div className="page-head">
        <h2>Connect via MCP</h2>
        <p>Provision an agent and a scoped key in one step, then paste the generated config into Claude Desktop, Cursor, or any MCP host.</p>
      </div>

      {!result ? (
        <div className="card grid" style={{ gap: 18, maxWidth: 560 }}>
          {/* Agent */}
          <div>
            <label>Agent</label>
            <div className="row" style={{ marginBottom: 8 }}>
              <button type="button" className={mode === "existing" ? "sm" : "secondary sm"} onClick={() => setMode("existing")}>Use existing</button>
              <button type="button" className={mode === "new" ? "sm" : "secondary sm"} onClick={() => setMode("new")}>Create new</button>
            </div>
            {mode === "existing" ? (
              <select value={agentId} onChange={(e) => setAgentId(e.target.value)}>
                <option value="">Select an agent…</option>
                {(agents.data?.agents ?? []).map((a) => (
                  <option key={a.id} value={a.id}>{a.name || a.id}</option>
                ))}
              </select>
            ) : (
              <input placeholder="New agent name, e.g. support-bot" value={newName} onChange={(e) => setNewName(e.target.value)} />
            )}
          </div>

          {/* Scopes */}
          <div>
            <label>Key scopes</label>
            <div className="row">
              {SCOPES.map((s) => (
                <button type="button" key={s} className={scopes.includes(s) ? "sm" : "secondary sm"} onClick={() => toggleScope(s)}>{s}</button>
              ))}
            </div>
            <p className="muted" style={{ marginTop: 8, fontSize: "0.8rem" }}>
              <code>read</code> recall/inspect · <code>write</code> remember/feedback · <code>admin</code> required for <code>verify_audit</code> and admin tools.
            </p>
          </div>

          {err && <div className="error-box">{err}</div>}
          <div>
            <button disabled={busy || (mode === "existing" ? !agentId : !newName.trim())} onClick={generate}>
              {busy ? "Generating…" : "Generate connection"}
            </button>
          </div>
        </div>
      ) : (
        <div className="grid" style={{ gap: 16, maxWidth: 720 }}>
          <div className="card" style={{ borderColor: "var(--success)" }}>
            <div className="row" style={{ gap: 10 }}>
              <span className="badge" style={{ background: "rgba(74,222,128,0.12)", color: "var(--success)", borderColor: "rgba(74,222,128,0.3)" }}>✓ Ready</span>
              <span className="secondary">Agent <strong>{result.agentName}</strong> + a {scopes.join("+")} key are provisioned.</span>
            </div>
            <p className="muted" style={{ marginTop: 10, fontSize: "0.82rem" }}>
              ⚠ The API key below is shown <strong>once</strong> — copy the config now. You can revoke it anytime from API Keys.
            </p>
          </div>

          <div className="card">
            <div className="row-between" style={{ marginBottom: 10 }}>
              <div className="stat-label" style={{ margin: 0 }}>claude_desktop_config.json</div>
              <button className="secondary sm" onClick={copy}>{copied ? "Copied ✓" : "Copy"}</button>
            </div>
            <pre className="config-block">{config}</pre>
            <p className="muted" style={{ marginTop: 10, fontSize: "0.8rem" }}>
              Build the binary with <code>go build -o engram-mcp ./cmd/engram-mcp</code> and use its absolute path for <code>command</code> if it isn't on your PATH. Restart the MCP host after saving.
            </p>
          </div>

          <div className="row">
            <button className="secondary" onClick={() => { setResult(null); setScopes(["read", "write"]); }}>Generate another</button>
          </div>
        </div>
      )}
    </Layout>
  );
}
