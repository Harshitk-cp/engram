import { useParams } from "react-router-dom";
import { Api, Mutation } from "../api";
import { useAsync } from "../hooks";
import { Layout } from "../components/Layout";
import { AsyncView } from "../components/ui";
import { dateTime, displayContent } from "../format";

function Sparkline({ mutations }: { mutations: Mutation[] }) {
  const pts = mutations
    .filter((m) => m.new_confidence != null)
    .map((m) => m.new_confidence as number)
    .reverse();
  if (pts.length < 2) return null;
  const w = 600, h = 60, pad = 4;
  const max = 1, min = 0;
  const step = (w - pad * 2) / (pts.length - 1);
  const y = (v: number) => h - pad - ((v - min) / (max - min)) * (h - pad * 2);
  const d = pts.map((v, i) => `${i === 0 ? "M" : "L"} ${pad + i * step} ${y(v)}`).join(" ");
  return (
    <svg width="100%" viewBox={`0 0 ${w} ${h}`} preserveAspectRatio="none" style={{ display: "block" }}>
      <path d={d} fill="none" stroke="var(--accent-2)" strokeWidth="2" />
    </svg>
  );
}

function delta(m: Mutation): string {
  if (m.old_confidence == null || m.new_confidence == null) return "";
  const arrow = m.new_confidence > m.old_confidence ? "↑" : m.new_confidence < m.old_confidence ? "↓" : "→";
  return `${m.old_confidence.toFixed(2)} ${arrow} ${m.new_confidence.toFixed(2)}`;
}

export default function Timeline() {
  const { memoryId = "" } = useParams();
  const state = useAsync(() => Api.mutations(memoryId), [memoryId]);
  const mem = useAsync(() => Api.memory(memoryId).catch(() => null), [memoryId]);

  return (
    <Layout title="Belief history">
      <div className="page-head">
        <h2>Belief history</h2>
        {mem.data ? (
          <p className="belief" style={{ margin: "8px 0 4px" }}>"{displayContent(mem.data.content)}"</p>
        ) : null}
        <p className="mono">{memoryId}</p>
      </div>

      <AsyncView state={state}>
        {(rawMutations) => {
          // Always anchor the timeline with a synthetic "created" event so beliefs
          // with no recorded changes still show their genesis.
          const genesis: Mutation | null = mem.data
            ? {
                id: "genesis",
                mutation_type: "created",
                source_type: mem.data.provenance ?? "system",
                new_confidence:
                  rawMutations.length > 0
                    ? rawMutations[rawMutations.length - 1].old_confidence
                    : mem.data.confidence,
                reason: "Belief created",
                created_at: mem.data.created_at ?? "",
              }
            : null;
          const mutations = genesis ? [...rawMutations, genesis] : rawMutations;
          return (
          <>
            <div className="card" style={{ marginBottom: 18 }}>
              <div className="stat-label" style={{ marginBottom: 8 }}>Confidence over time</div>
              <Sparkline mutations={mutations} />
            </div>
            <ol className="timeline">
              {mutations.map((m) => (
                <li key={m.id} className="timeline-item">
                  <div className="row-between">
                    <span className={`tag ${m.mutation_type}`}>{m.mutation_type}</span>
                    <span className="muted">{dateTime(m.created_at)}</span>
                  </div>
                  <div className="reason">{m.reason}</div>
                  <div className="conf">
                    {delta(m) && <span>{delta(m)} · </span>}
                    source: {m.source_type}
                    {m.actor_type ? ` · actor: ${m.actor_type}` : ""}
                  </div>
                </li>
              ))}
            </ol>
          </>
          );
        }}
      </AsyncView>
    </Layout>
  );
}
