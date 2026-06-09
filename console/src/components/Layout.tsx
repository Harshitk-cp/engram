import { useEffect, useRef, useState } from "react";
import { NavLink, useNavigate } from "react-router-dom";
import { useAuth } from "../auth";
import { initials } from "../format";
import { Logo } from "./Logo";

function Icon({ path }: { path: string }) {
  return (
    <svg width="17" height="17" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.7" strokeLinecap="round" strokeLinejoin="round">
      <path d={path} />
    </svg>
  );
}
const icons = {
  agents: "M16 21v-2a4 4 0 0 0-4-4H6a4 4 0 0 0-4 4v2 M9 11a4 4 0 1 0 0-8 4 4 0 0 0 0 8 M22 21v-2a4 4 0 0 0-3-3.87 M16 3.13a4 4 0 0 1 0 7.75",
  keys: "M21 2l-2 2m-7.61 7.61a5.5 5.5 0 1 1-7.778 7.778 5.5 5.5 0 0 1 7.777-7.777zm0 0L15.5 7.5m0 0l3 3L22 7l-3-3m-3.5 3.5L19 4",
  settings: "M12 15a3 3 0 1 0 0-6 3 3 0 0 0 0 6z M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 1 1-2.83 2.83l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-4 0v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 1 1-2.83-2.83l.06-.06a1.65 1.65 0 0 0 .33-1.82 1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1 0-4h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 1 1 2.83-2.83l.06.06a1.65 1.65 0 0 0 1.82.33H9a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 4 0v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 1 1 2.83 2.83l-.06.06a1.65 1.65 0 0 0-.33 1.82V9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 0 4h-.09a1.65 1.65 0 0 0-1.51 1z",
  audit: "M9 12l2 2 4-4 M7.835 4.697a3.42 3.42 0 0 0 1.946-.806 3.42 3.42 0 0 1 4.438 0 3.42 3.42 0 0 0 1.946.806 3.42 3.42 0 0 1 3.138 3.138 3.42 3.42 0 0 0 .806 1.946 3.42 3.42 0 0 1 0 4.438 3.42 3.42 0 0 0-.806 1.946 3.42 3.42 0 0 1-3.138 3.138 3.42 3.42 0 0 0-1.946.806 3.42 3.42 0 0 1-4.438 0 3.42 3.42 0 0 0-1.946-.806 3.42 3.42 0 0 1-3.138-3.138 3.42 3.42 0 0 0-.806-1.946 3.42 3.42 0 0 1 0-4.438 3.42 3.42 0 0 0 .806-1.946 3.42 3.42 0 0 1 3.138-3.138z",
  canon: "M4 19.5A2.5 2.5 0 0 1 6.5 17H20 M6.5 2H20v20H6.5A2.5 2.5 0 0 1 4 19.5v-15A2.5 2.5 0 0 1 6.5 2z",
};

function useClickOutside(onClose: () => void) {
  const ref = useRef<HTMLDivElement>(null);
  useEffect(() => {
    const h = (e: MouseEvent) => {
      if (ref.current && !ref.current.contains(e.target as Node)) onClose();
    };
    document.addEventListener("mousedown", h);
    return () => document.removeEventListener("mousedown", h);
  }, [onClose]);
  return ref;
}

function OrgSwitcher() {
  const { me, switchOrg } = useAuth();
  const [open, setOpen] = useState(false);
  const ref = useClickOutside(() => setOpen(false));
  if (!me) return null;
  const active = me.orgs.find((o) => o.tenant_id === me.active_tenant_id) || me.orgs[0];
  return (
    <div className="menu-wrap" ref={ref}>
      <button className="user-btn" onClick={() => setOpen((v) => !v)}>
        <span className="badge">org</span>
        <span style={{ maxWidth: 160, overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }}>
          {active ? active.tenant_name : "No org"}
        </span>
      </button>
      {open && (
        <div className="menu">
          <div className="menu-label">Organizations</div>
          {me.orgs.map((o) => (
            <button
              key={o.tenant_id}
              className={`menu-item ${o.tenant_id === me.active_tenant_id ? "active" : ""}`}
              onClick={async () => {
                setOpen(false);
                if (o.tenant_id !== me.active_tenant_id) {
                  await switchOrg(o.tenant_id);
                  window.location.reload();
                }
              }}
            >
              <span style={{ flex: 1 }}>{o.tenant_name}</span>
              <span className="conf">{o.role}</span>
            </button>
          ))}
        </div>
      )}
    </div>
  );
}

function UserMenu() {
  const { me, logout } = useAuth();
  const nav = useNavigate();
  const [open, setOpen] = useState(false);
  const ref = useClickOutside(() => setOpen(false));
  if (!me) return null;
  return (
    <div className="menu-wrap" ref={ref}>
      <button className="user-btn" onClick={() => setOpen((v) => !v)}>
        <span className="avatar">{initials(me.user.name || me.user.email)}</span>
        <span style={{ maxWidth: 120, overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }}>
          {me.user.name || me.user.email}
        </span>
      </button>
      {open && (
        <div className="menu">
          <div className="menu-label">{me.user.email}</div>
          <button className="menu-item" onClick={() => { setOpen(false); nav("/settings"); }}>Settings</button>
          <button className="menu-item" onClick={async () => { await logout(); nav("/login"); }}>Sign out</button>
        </div>
      )}
    </div>
  );
}

export function Layout({ title, children }: { title?: React.ReactNode; children: React.ReactNode }) {
  return (
    <div className="shell">
      <aside className="sidebar">
        <div className="brand"><Logo /></div>
        <div className="nav-section">Workspace</div>
        <NavLink to="/agents" className="nav-link"><Icon path={icons.agents} /> Agents</NavLink>
        <NavLink to="/canon" className="nav-link"><Icon path={icons.canon} /> Canon</NavLink>
        <NavLink to="/keys" className="nav-link"><Icon path={icons.keys} /> API Keys</NavLink>
        <NavLink to="/audit" className="nav-link"><Icon path={icons.audit} /> Audit</NavLink>
        <NavLink to="/settings" className="nav-link"><Icon path={icons.settings} /> Settings</NavLink>
        <div className="sidebar-spacer" />
        <a className="nav-link" href="https://docs.hakuya.ai" target="_blank" rel="noreferrer">Docs ↗</a>
      </aside>
      <div className="main">
        <header className="topbar">
          <div className="crumbs">{title}</div>
          <div className="row">
            <OrgSwitcher />
            <UserMenu />
          </div>
        </header>
        <div className="content">{children}</div>
      </div>
    </div>
  );
}
