import { AsyncState } from "../hooks";

export function Spinner() {
  return <div className="spinner" />;
}

export function Loading() {
  return (
    <div className="center">
      <Spinner />
    </div>
  );
}

export function ErrorBox({ message }: { message: string }) {
  return <div className="error-box">{message}</div>;
}

export function Empty({ title, hint }: { title: string; hint?: string }) {
  return (
    <div className="empty">
      <div className="empty-icon">
        <svg width="40" height="40" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.4">
          <circle cx="12" cy="12" r="9" />
          <path d="M8 12h8" />
        </svg>
      </div>
      <div style={{ fontWeight: 600, color: "var(--text-secondary)" }}>{title}</div>
      {hint && <div style={{ marginTop: 6 }}>{hint}</div>}
    </div>
  );
}

// AsyncView renders loading/error/empty/content states for a useAsync result.
export function AsyncView<T>({
  state,
  children,
  empty,
}: {
  state: AsyncState<T>;
  children: (data: T) => React.ReactNode;
  empty?: (data: T) => boolean;
}) {
  if (state.loading && state.data == null) return <Loading />;
  if (state.error) return <ErrorBox message={state.error} />;
  if (state.data == null) return <Loading />;
  if (empty && empty(state.data)) return <Empty title="Nothing here yet" />;
  return <>{children(state.data)}</>;
}

export function Stat({ label, value, warn, hint }: { label: string; value: React.ReactNode; warn?: boolean; hint?: string }) {
  return (
    <div className={`stat ${warn ? "warn" : ""}`} title={hint}>
      <div className="stat-value">{value}</div>
      <div className="stat-label">{label}{hint && <span className="stat-info" aria-hidden> ⓘ</span>}</div>
    </div>
  );
}

const TIERS = ["hot", "warm", "cold", "archive"] as const;

export function TierBars({ counts, total }: { counts: Record<string, number>; total: number }) {
  return (
    <div className="tiers">
      {TIERS.map((t) => {
        const c = counts[t] ?? 0;
        const pct = total > 0 ? Math.round((c / total) * 100) : 0;
        return (
          <div key={t} className="tier-row">
            <span className={`tier-label ${t}`}>{t}</span>
            <div className="bar">
              <div className={`bar-fill ${t}`} style={{ width: `${pct}%` }} />
            </div>
            <span className="tier-count">{c}</span>
          </div>
        );
      })}
    </div>
  );
}

export function Pager({
  offset,
  limit,
  total,
  onPage,
}: {
  offset: number;
  limit: number;
  total: number;
  onPage: (offset: number) => void;
}) {
  const from = total === 0 ? 0 : offset + 1;
  const to = Math.min(offset + limit, total);
  return (
    <div className="pager">
      <span>
        {from}–{to} of {total}
      </span>
      <button className="secondary sm" disabled={offset === 0} onClick={() => onPage(Math.max(0, offset - limit))}>
        Prev
      </button>
      <button className="secondary sm" disabled={to >= total} onClick={() => onPage(offset + limit)}>
        Next
      </button>
    </div>
  );
}
