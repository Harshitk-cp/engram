import { useAuth } from "../auth";
import { Layout } from "../components/Layout";
import { initials } from "../format";

export default function Settings() {
  const { me } = useAuth();
  if (!me) return null;
  return (
    <Layout title="Settings">
      <div className="page-head">
        <h2>Settings</h2>
        <p>Your account and organizations.</p>
      </div>

      <div className="card" style={{ maxWidth: 560 }}>
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
      <div className="card" style={{ maxWidth: 560, padding: 0 }}>
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
    </Layout>
  );
}
