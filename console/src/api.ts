// Session-based API client. The console is served same-origin by the Go binary,
// so the httpOnly session cookie is sent automatically. On 401 we broadcast an
// event the auth layer uses to bounce to the login screen.

export class ApiError extends Error {
  status: number;
  constructor(status: number, message: string) {
    super(message);
    this.status = status;
  }
}

async function req<T>(path: string, opts: RequestInit = {}): Promise<T> {
  const res = await fetch(path, {
    credentials: "include",
    headers: { "Content-Type": "application/json", ...(opts.headers as Record<string, string>) },
    ...opts,
  });
  if (res.status === 401) {
    window.dispatchEvent(new CustomEvent("engram:unauthorized"));
  }
  if (res.status === 204) return undefined as T;
  const text = await res.text();
  const body = text ? JSON.parse(text) : undefined;
  if (!res.ok) throw new ApiError(res.status, (body && body.error) || res.statusText);
  return body as T;
}

// ---------- Types ----------
export interface User { id: string; email: string; name: string; avatar_url?: string | null; }
export interface Org { tenant_id: string; tenant_name: string; role: string; }
export interface Me { user: User; active_tenant_id: string | null; orgs: Org[]; }
export interface AuthConfig { password: boolean; google: boolean; github: boolean; workos: boolean; }

export interface Agent { id: string; external_id?: string; name: string; created_at?: string; }
export interface Dashboard {
  agent_id: string; tier_counts: Record<string, number>; total_memories: number;
  needs_review_count: number; contradiction_count: number;
  learning_velocity?: number; stability_score?: number;
}
export interface Memory {
  id: string; content: string; type: string; confidence: number;
  binding?: string; provenance?: string; reinforcement_count?: number;
  created_at?: string; updated_at?: string;
}
export interface MemoryPage { items: Memory[]; total: number; limit: number; offset: number; }
export interface ContradictingBelief { memory_id: string; content?: string; confidence?: number; }
export interface ReviewItem { memory: Memory; tier: string; contradictions?: ContradictingBelief[]; }
export interface Mutation {
  id: string; mutation_type: string; source_type: string;
  old_confidence?: number; new_confidence?: number; reason: string;
  actor_type?: string; created_at: string;
}
export interface ApiKey {
  id: string; name?: string; key_prefix: string; scopes: string[];
  created_at?: string; last_used_at?: string; created_by_email?: string | null;
}
export interface ContradictionPair {
  belief_id: string; belief_content: string; belief_confidence: number;
  other_id: string; other_content: string; other_confidence: number; detected_at: string;
}
export interface BeliefAtTime { id: string; content: string; type: string; confidence: number; created_at: string; }
export interface Snapshot { as_of: string; total: number; beliefs: BeliefAtTime[]; }
export interface AuditStatus {
  valid: boolean; checked: number; break_seq?: number | null;
  head_seq: number; head_hash: string; signed: boolean; verified_at: string;
}
export interface ChainEntry {
  id: string; memory_id?: string; agent_id?: string; mutation_type: string;
  source_type?: string; old_confidence?: number; new_confidence?: number;
  reason: string; content_hash?: string; actor_type?: string; created_at: string;
  seq: number; prev_hash?: string; row_hash?: string;
}
export interface EngineSettings {
  decay_base_rate: number;
  decay_floor: number;
  archive_threshold: number;
  competition_weight: number;
  reinforcement_log_odds: number;
  contradiction_log_odds: number;
  firewall_enabled?: boolean;
  quarantine_provenances?: string[];
}

export interface Anchor {
  id: string;
  name: string;
  entity_type: string;
  external_id?: string;
  aliases?: string[];
  created_at?: string;
  updated_at?: string;
}

export interface QuarantineItem {
  id: string;
  content: string;
  type: string;
  provenance: string;
  source?: string;
  quarantine_reason?: string;
  quarantined_at?: string;
  created_at?: string;
}
export interface EngineSettingsResponse { settings: EngineSettings; defaults: EngineSettings; }

export interface PlanLimits { max_agents: number; max_memories_per_month: number; price_usd: number; }
export interface Usage { period_month: string; memories_written: number; recalls: number; }
export interface PlanOption { plan: string; limits: PlanLimits; purchasable: boolean; }
export interface BillingState {
  plan: string; subscription_status: string; limits: PlanLimits;
  usage: Usage; agent_count: number; billing_enabled: boolean; plans: PlanOption[];
}

// ---------- Auth ----------
export const Auth = {
  config: () => req<AuthConfig>("/auth/config"),
  me: () => req<Me>("/auth/me"),
  login: (email: string, password: string) =>
    req<void>("/auth/login", { method: "POST", body: JSON.stringify({ email, password }) }),
  register: (email: string, password: string, name: string) =>
    req<{ user: User }>("/auth/register", { method: "POST", body: JSON.stringify({ email, password, name }) }),
  logout: () => req<void>("/auth/logout", { method: "POST" }),
  switchOrg: (tenant_id: string) =>
    req<void>("/auth/switch-tenant", { method: "POST", body: JSON.stringify({ tenant_id }) }),
  createOrg: (name: string) =>
    req<Org>("/auth/orgs", { method: "POST", body: JSON.stringify({ name }) }),
};

// ---------- Data plane ----------
export const Api = {
  listAgents: (limit = 50, offset = 0) =>
    req<{ agents: Agent[]; total: number; count: number; limit: number; offset: number }>(
      `/v1/agents/?limit=${limit}&offset=${offset}`
    ),
  // external_id is optional — the server derives a unique one from the name when omitted.
  createAgent: (name: string, external_id?: string) =>
    req<Agent>("/v1/agents/", {
      method: "POST",
      body: JSON.stringify(external_id ? { name, external_id } : { name }),
    }),
  dashboard: (agentId: string) => req<Dashboard>(`/v1/agents/${agentId}/dashboard`),
  memories: (
    agentId: string,
    opts: { tier?: string; type?: string; provenance?: string; binding?: string; limit?: number; offset?: number } = {}
  ) => {
    const q = new URLSearchParams();
    if (opts.tier) q.set("tier", opts.tier);
    if (opts.type) q.set("type", opts.type);
    if (opts.provenance) q.set("provenance", opts.provenance);
    if (opts.binding) q.set("binding", opts.binding);
    q.set("limit", String(opts.limit ?? 25));
    q.set("offset", String(opts.offset ?? 0));
    return req<MemoryPage>(`/v1/agents/${agentId}/memories?${q.toString()}`);
  },
  reviewQueue: (agentId: string) =>
    req<{ items: ReviewItem[]; count: number }>(`/v1/agents/${agentId}/review-queue`),
  contradictions: (agentId: string) =>
    req<{ pairs: ContradictionPair[]; count: number }>(`/v1/agents/${agentId}/contradictions`),
  snapshot: (agentId: string, atISO: string) =>
    req<Snapshot>(`/v1/agents/${agentId}/snapshot?at=${encodeURIComponent(atISO)}`),
  mutations: (memoryId: string) =>
    req<{ mutations: Mutation[] }>(`/v1/memories/${memoryId}/mutations`).then((r) => r.mutations ?? []),
  memory: (memoryId: string) => req<Memory>(`/v1/memories/${memoryId}`).then((r: any) => r.memory ?? r),
  resolveContradiction: (keep_id: string, demote_id: string, reason: string) =>
    req<void>("/v1/admin/contradictions/resolve", {
      method: "POST", body: JSON.stringify({ keep_id, demote_id, reason }),
    }),
  updateMemory: (id: string, body: { confidence?: number; content?: string; reason: string }) =>
    req<Memory>(`/v1/memories/${id}`, { method: "PATCH", body: JSON.stringify(body) }),
  listCanon: () =>
    req<{ memories: Memory[]; count: number }>("/v1/canon/").then((r) => r.memories ?? []),
  createCanon: (agent_id: string, content: string, type: string) =>
    req<unknown>("/v1/canon/", { method: "POST", body: JSON.stringify({ agent_id, content, type }) }),
  deleteCanon: (id: string) => req<unknown>(`/v1/canon/${id}`, { method: "DELETE" }),
  auditVerify: () => req<AuditStatus>("/v1/audit/verify"),
  auditChain: (agentId?: string, limit = 100) =>
    req<{ entries: ChainEntry[]; count: number }>(
      `/v1/audit/chain?limit=${limit}${agentId ? `&agent_id=${agentId}` : ""}`
    ).then((r) => r.entries ?? []),
  listKeys: () => req<{ keys: ApiKey[] }>("/v1/keys/").then((r) => r.keys ?? []),
  createKey: (name: string, scopes: string[]) =>
    req<ApiKey & { api_key?: string }>("/v1/keys/", { method: "POST", body: JSON.stringify({ name, scopes }) }),
  revokeKey: (id: string) => req<void>(`/v1/keys/${id}`, { method: "DELETE" }),

  // Embedding configuration (deployment-level, read-only)
  embeddingInfo: () =>
    req<{ provider: string; model: string; dimension: number; base_url?: string }>("/v1/embedding/info"),
  reembedAgent: (agentId: string) =>
    req<{ reembedded: number }>(`/v1/admin/agents/${agentId}/reembed`, { method: "POST" }),

  // Engine settings (per-tenant tuning)
  getSettings: () => req<EngineSettingsResponse>("/v1/settings/"),
  updateSettings: (s: EngineSettings) =>
    req<EngineSettingsResponse>("/v1/settings/", { method: "PUT", body: JSON.stringify(s) }),

  // Subjects (anchors) — GDPR right-to-erasure
  anchors: (limit = 200) =>
    req<{ anchors: Anchor[]; count: number }>(`/v1/anchors/?limit=${limit}`).then((r) => r.anchors ?? []),
  anchorMemories: (id: string) =>
    req<{ memories: Memory[]; count: number }>(`/v1/anchors/${id}/memories`).then((r) => r.memories ?? []),
  shredSubject: (id: string, reason: string) =>
    req<{ shredded: number }>(`/v1/admin/anchors/${id}/shred`, {
      method: "POST", body: JSON.stringify({ reason }),
    }),
  purgeSubject: (id: string) =>
    req<{ deleted: boolean; memories_purged: number }>(`/v1/anchors/${id}?purge=true`, { method: "DELETE" }),

  // Provenance Firewall — quarantine review queue
  quarantine: (agentId: string, limit = 50, offset = 0) =>
    req<{ items: QuarantineItem[]; total: number; limit: number; offset: number }>(
      `/v1/agents/${agentId}/quarantine?limit=${limit}&offset=${offset}`
    ),
  releaseQuarantine: (id: string, note?: string) =>
    req<{ released: boolean }>(`/v1/quarantine/${id}/release`, {
      method: "POST", body: JSON.stringify({ note: note ?? "" }),
    }),
  rejectQuarantine: (id: string, note?: string) =>
    req<void>(`/v1/quarantine/${id}/reject`, { method: "POST", body: JSON.stringify({ note: note ?? "" }) }),

  // Billing (managed cloud, Razorpay)
  billing: () => req<BillingState>("/v1/billing/"),
  // Creates a Razorpay subscription; the browser opens the Checkout modal with these.
  checkout: (plan: string) =>
    req<{ subscription_id: string; key_id: string }>("/v1/billing/checkout", {
      method: "POST",
      body: JSON.stringify({ plan }),
    }),
  // Confirms the payment signature returned by the Checkout modal's success handler.
  verifyPayment: (p: {
    razorpay_payment_id: string;
    razorpay_subscription_id: string;
    razorpay_signature: string;
  }) => req<{ ok: boolean }>("/v1/billing/verify", { method: "POST", body: JSON.stringify(p) }),
  cancelSubscription: () => req<{ ok: boolean }>("/v1/billing/cancel", { method: "POST" }),
};
