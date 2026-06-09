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
};

// ---------- Data plane ----------
export const Api = {
  listAgents: (limit = 50, offset = 0) =>
    req<{ agents: Agent[]; total: number; count: number; limit: number; offset: number }>(
      `/v1/agents/?limit=${limit}&offset=${offset}`
    ),
  createAgent: (external_id: string, name: string) =>
    req<Agent>("/v1/agents/", { method: "POST", body: JSON.stringify({ external_id, name }) }),
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
  listKeys: () => req<{ keys: ApiKey[] }>("/v1/keys/").then((r) => r.keys ?? []),
  createKey: (name: string, scopes: string[]) =>
    req<ApiKey & { api_key?: string }>("/v1/keys/", { method: "POST", body: JSON.stringify({ name, scopes }) }),
  revokeKey: (id: string) => req<void>(`/v1/keys/${id}`, { method: "DELETE" }),
};
