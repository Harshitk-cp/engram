// Engram neural-network brand mark (identical to engram-docs logo + engram-web nav):
// gradient ring with one top node, two bottom nodes, and their connections.
export function LogoMark({ size = 26 }: { size?: number }) {
  const id = "engram-grad";
  const g = `url(#${id})`;
  return (
    <svg width={size} height={size} viewBox="0 0 32 32" fill="none" xmlns="http://www.w3.org/2000/svg">
      <defs>
        <linearGradient id={id} x1="0" y1="0" x2="32" y2="32" gradientUnits="userSpaceOnUse">
          <stop stopColor="#a78bfa" />
          <stop offset="1" stopColor="#6366f1" />
        </linearGradient>
      </defs>
      <circle cx="16" cy="16" r="14" stroke={g} strokeWidth="2" />
      <circle cx="16" cy="10" r="3" fill={g} />
      <circle cx="10" cy="20" r="2.5" fill={g} opacity="0.7" />
      <circle cx="22" cy="20" r="2.5" fill={g} opacity="0.7" />
      <line x1="16" y1="13" x2="11" y2="18" stroke={g} strokeWidth="1.5" opacity="0.5" />
      <line x1="16" y1="13" x2="21" y2="18" stroke={g} strokeWidth="1.5" opacity="0.5" />
      <line x1="12" y1="20" x2="20" y2="20" stroke={g} strokeWidth="1" opacity="0.3" />
    </svg>
  );
}

export function Logo() {
  return (
    <span className="brand">
      <LogoMark />
      <span className="wordmark">engram</span>
    </span>
  );
}
