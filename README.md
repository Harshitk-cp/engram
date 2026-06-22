# Engram

**Provable memory infrastructure for AI agents.**

Engram stores, retrieves, and injects typed memory into agent prompts via a simple HTTP API or an MCP server. Memories gain confidence when reinforced, lose it when contradicted, and fade when unused. Unlike a black-box memory layer, Engram records **where every belief came from**, lets stale beliefs **decay**, can **erase any subject on request**, and writes **every change to a tamper-evident audit trail** — so you can prove what your agent knows, and why.

## Architecture

<p align="center">
  <img src="https://assets.hakuya.ai/assets/cognitive_core.png" alt="Engram architecture — six layers: client entry points, the Provenance Firewall, the cognitive core (semantic / episodic / procedural / working memory, contradiction detection, consolidation, metacognition), per-subject isolation, the SHA-256 tamper-evident audit chain, and PostgreSQL + pgvector storage." width="480">
</p>

Every write enters through the **Provenance Firewall**, flows into the **cognitive core** (the four memory systems plus contradiction detection, consolidation, and metacognition), stays isolated per **subject**, and is recorded — change by change — in a **tamper-evident audit chain** on top of **PostgreSQL + pgvector**.

## Benchmarks

Engram scores **91.4% on [LongMemEval](https://github.com/xiaowu0162/LongMemEval)** — the ICLR 2025 benchmark for long-term conversational memory (500 questions over chat histories scalable past 1M tokens) — and **92.3% averaged across its six task types**.

| Task type | Engram |
|---|---:|
| Knowledge update | 100.0% |
| Abstention | 100.0% |
| Single-session (user fact) | 98.4% |
| Single-session (preference) | 93.3% |
| Single-session (assistant) | 89.3% |
| Multi-session | 90.2% |
| Temporal reasoning | 82.3% |
| **Overall (500 questions)** | **91.4%** |

Measured with Engram as the memory store + retrieval layer, graded by LongMemEval's standard LLM judge. Full per-task breakdown, methodology, and how to read memory benchmarks: **[hakuya.ai/#benchmarks](https://hakuya.ai/#benchmarks)** · **[docs.hakuya.ai/benchmarks](https://docs.hakuya.ai/benchmarks)**.

## Quick Start

```bash
# Clone and configure
git clone https://github.com/Harshitk-cp/engram.git
cd engram
cp .env.example .env

# Start with Docker
docker compose up -d

# Run migrations
docker compose exec server /engram migrate
```

Then open the **admin console at [http://localhost:8080/console](http://localhost:8080/console)** to create an account, register an agent, and mint an API key — or bootstrap headlessly:

```bash
# Bootstrap a deployment (one-time): creates a tenant + its first master key
curl -X POST http://localhost:8080/v1/setup \
  -H "X-Setup-Token: $ENGRAM_SETUP_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"org_name": "My App"}'   # → returns api_key (shown once)
```

### Basic Usage

```bash
# 1. Register an agent
curl -X POST http://localhost:8080/v1/agents \
  -H "Authorization: Bearer $API_KEY" \
  -d '{"external_id": "agent-1", "name": "My Agent"}'

# 2. Store memory
curl -X POST http://localhost:8080/v1/memories \
  -H "Authorization: Bearer $API_KEY" \
  -d '{"agent_id": "AGENT_UUID", "content": "User prefers dark mode"}'

# 3. Recall memories
curl "http://localhost:8080/v1/memories/recall?agent_id=AGENT_UUID&query=display+preferences" \
  -H "Authorization: Bearer $API_KEY"
```

## Use it as an MCP server

Engram ships a standalone **[MCP](https://modelcontextprotocol.io) server** (`engram-mcp`) that exposes **37 memory tools** to Claude Desktop, Cursor, Windsurf, or any MCP-compatible host — `remember`, `recall`, `recall_graph`, `get_hot_context`, `ingest_conversation`, plus episodic, schema, anchor, metacognition, calibration, and audit tools.

```json
{
  "mcpServers": {
    "engram": {
      "command": "engram-mcp",
      "args": ["--transport", "stdio"],
      "env": {
        "ENGRAM_API_URL": "http://localhost:8080",
        "ENGRAM_API_KEY": "mk_your_key_here",
        "ENGRAM_AGENT_ID": "your-agent-id"
      }
    }
  }
}
```

Full tool list and host setup: **[docs.hakuya.ai/guides/mcp](https://docs.hakuya.ai/guides/mcp)**.

## Core Concepts

### Memory Types

| Type | Description |
|------|-------------|
| `preference` | User likes/dislikes, style choices |
| `fact` | Information about user or context |
| `decision` | Choices the user has made |
| `constraint` | Hard rules or limitations |

### Memory Tiers

Memories are tiered by confidence for efficient retrieval:

| Tier | Confidence | Behavior |
|------|------------|----------|
| Hot | > 0.85 | Auto-injected into context |
| Warm | 0.70-0.85 | Retrieved on demand |
| Cold | 0.40-0.70 | Requires explicit query |
| Archive | < 0.40 | Soft-deleted, recoverable |

### Belief Dynamics

- **Reinforcement**: Similar statements increase confidence (+0.05)
- **Contradiction**: Conflicting beliefs decrease confidence (-0.2)
- **Decay**: Unused memories gradually lose confidence
- **Usage Boost**: Recalled memories gain small confidence (+0.02)

Every one of these changes is written to an append-only [mutation log](#provenance--trust) — the why-trail behind each belief.

### Multi-Subject Memory (Anchors, Sessions, Canon)

One agent can act on behalf of thousands of **subjects** (customers, guests, patients) with full isolation. Every memory carries a server-derived **binding**:

| Binding | Bound to | Set by |
|---|---|---|
| `private` | the forming agent (default — unchanged behavior) | no extra ids |
| `anchored` | a specific **anchor** (a subject) | `anchor_external_id` |
| `session` | one conversation (short-term, decays fast) | `session_id` |
| `canon` | tenant-shared org knowledge (policies, catalog) | `POST /v1/canon` (admin) |

```bash
# Store a fact ABOUT a subject (anchor auto-created on first use)
curl -X POST http://localhost:8080/v1/memories -H "Authorization: Bearer $API_KEY" \
  -d '{"agent_id":"AGENT","content":"Guest is vegetarian","anchor_external_id":"guest-42"}'

# Recall is isolated per subject — guest-99's data never leaks into guest-42's results
curl "http://localhost:8080/v1/memories/recall?agent_id=AGENT&query=diet&anchor_external_id=guest-42" \
  -H "Authorization: Bearer $API_KEY"

# GDPR per-subject erasure (crypto-shred — content unrecoverable, audit record retained)
curl -X DELETE "http://localhost:8080/v1/anchors/ANCHOR_ID?purge=true" -H "Authorization: Bearer $API_KEY"
```

Passing only `agent_id` preserves today's exact behavior. Endpoints: `/v1/anchors`, `/v1/sessions`, `/v1/canon`. See the [Subjects, Sessions & Canon guide](https://docs.hakuya.ai/concepts/scopes).

## Provenance & Trust

Engram is built so you can **prove** what an agent knows and how it changed:

- **Provenance on every belief** — source (`user`/`agent`/`tool`/`derived`), evidence type, and confidence are attached at write time.
- **Tamper-evident audit trail** — every memory mutation (create / reinforce / decay / contradict / redact / release) is appended to a per-tenant **SHA-256 hash chain**. `GET /v1/audit/verify` recomputes the chain and detects any edit, insertion, deletion, or reordering. `GET /v1/audit/export` produces a signed NDJSON trail for compliance.
- **Provenance Firewall** — untrusted memories (e.g. extracted from third-party content) are held in a **quarantine queue**, kept out of recall and belief logic until an admin reviews them. Release/reject decisions are themselves recorded in the audit chain.
- **Verified per-subject erasure** — `forget_subject` / crypto-shred destroys a subject's content irrecoverably while preserving the immutable audit record that the erasure happened (GDPR Article 17, EU AI Act).

```bash
# Verify the tamper-evident chain is intact
curl http://localhost:8080/v1/audit/verify -H "Authorization: Bearer $ADMIN_KEY"

# Review the Provenance Firewall queue, then release or reject
curl http://localhost:8080/v1/agents/AGENT_ID/quarantine -H "Authorization: Bearer $ADMIN_KEY"
curl -X POST http://localhost:8080/v1/quarantine/MEM_ID/release -H "Authorization: Bearer $ADMIN_KEY"
```

## Admin Console

A React admin console is embedded in the server binary at **`/console`**. It provides:

- **Agent dashboard** — knowledge health, tier distribution, learning velocity, stability
- **Memories browser** — filter by tier, type, source, and binding; inspect provenance
- **Contradictions** & **Review Queue** — resolve conflicting beliefs with audited reasons
- **Quarantine** — the Provenance Firewall review surface (release / reject)
- **Audit** — verify the hash chain and export the signed trail
- **Subjects** — manage anchors and per-subject erasure
- **Canon** — tenant-wide authoritative knowledge
- **Time Machine** — replay how a memory evolved over time
- **Keys**, **Billing**, **Settings** — API keys, plan/usage, and per-tenant engine tuning

## Memory Systems

Engram implements four memory types inspired by cognitive science:

| System | Purpose | API |
|--------|---------|-----|
| **Semantic** | Facts, preferences, beliefs | `/v1/memories` |
| **Episodic** | Rich experiences with context | `/v1/episodes` |
| **Procedural** | Learned skills and patterns | `/v1/procedures` |
| **Working** | Active context via spreading activation | `/v1/cognitive/activate` |

Plus **Schemas** for higher-order mental models (user archetypes, situation templates): `/v1/schemas`.

## Key Features

### Hybrid Retrieval (Vector + Graph)

All recall uses hybrid retrieval by default, combining semantic similarity with graph relationship traversal:

```bash
curl "http://localhost:8080/v1/memories/recall?agent_id=...&query=...&graph_weight=0.4&max_hops=2" \
  -H "Authorization: Bearer $API_KEY"
```

Response includes both vector and graph scores for transparency.

### Conversation Extraction

Automatically extract memories from conversations:

```bash
curl -X POST http://localhost:8080/v1/memories/extract \
  -H "Authorization: Bearer $API_KEY" \
  -d '{
    "agent_id": "...",
    "conversation": [
      {"role": "user", "content": "I only use open source tools"},
      {"role": "assistant", "content": "Noted, I will suggest open source alternatives."}
    ],
    "auto_store": true
  }'
```

### Metacognition & Calibration

Self-assessment of memory quality, plus a measured calibration score (ECE / MCE / Brier):

```bash
# Reflect on memory state
curl -X POST http://localhost:8080/v1/cognitive/reflect \
  -H "Authorization: Bearer $API_KEY" -d '{"agent_id": "..."}'

# Measure how well-calibrated the agent's confidence is
curl "http://localhost:8080/v1/cognitive/calibration?agent_id=..." \
  -H "Authorization: Bearer $API_KEY"
```

### Confidence Lifecycle

Explicit confidence management:

```bash
# Reinforce a memory
curl -X POST http://localhost:8080/v1/cognitive/confidence/reinforce \
  -d '{"memory_id": "...", "boost": 0.1}'

# Penalize a memory
curl -X POST http://localhost:8080/v1/cognitive/confidence/penalize \
  -d '{"memory_id": "...", "penalty": 0.15}'
```

## API Reference

The full, browsable reference lives at **[docs.hakuya.ai/api-reference](https://docs.hakuya.ai)**. Key families:

### Auth & Keys

| Method | Endpoint | Description |
|--------|----------|-------------|
| `POST` | `/v1/setup` | Bootstrap a deployment (X-Setup-Token) |
| `POST` | `/v1/keys` | Create API key (admin) |
| `GET` | `/v1/keys` | List keys (admin) |
| `DELETE` | `/v1/keys/:id` | Revoke key (admin) |

### Agents & Memories

| Method | Endpoint | Description |
|--------|----------|-------------|
| `POST` | `/v1/agents` | Register agent |
| `GET` | `/v1/agents/:id/mind` | Get agent's complete mental state |
| `POST` | `/v1/memories` | Store memory |
| `GET` | `/v1/memories/recall` | Hybrid recall (vector + graph) |
| `POST` | `/v1/memories/extract` | Extract from conversation |
| `GET` | `/v1/memories/:id/mutations` | Provenance / why-trail |
| `POST` | `/v1/memories/:id/restore` | Un-archive a memory |

### Multi-Subject (Anchors, Sessions, Canon)

| Method | Endpoint | Description |
|--------|----------|-------------|
| `POST` | `/v1/anchors` | Create a subject (anchor) |
| `GET` | `/v1/anchors/:id/memories` | A subject's durable profile |
| `DELETE` | `/v1/anchors/:id?purge=true` | GDPR per-subject erasure |
| `POST` | `/v1/sessions` | Start a conversation session |
| `POST` | `/v1/sessions/:id/end` | End session (promote recurring memory) |
| `GET` `POST` `DELETE` | `/v1/canon` | Tenant-shared knowledge |

### Provenance, Audit & Admin

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/v1/audit/verify` | Verify tamper-evident hash chain |
| `GET` | `/v1/audit/chain` | Recent chain entries (seq + hash) |
| `GET` | `/v1/audit/export` | Signed NDJSON audit export |
| `GET` | `/v1/agents/:id/quarantine` | Provenance Firewall queue |
| `POST` | `/v1/quarantine/:id/release` | Release a quarantined memory |
| `POST` | `/v1/quarantine/:id/reject` | Reject a quarantined memory |
| `POST` | `/v1/admin/anchors/:id/shred` | Crypto-shred a subject |
| `POST` | `/v1/admin/memories/:id/redact` | Redact content (audited) |

### Cognitive, Graph & Learning

| Method | Endpoint | Description |
|--------|----------|-------------|
| `POST` | `/v1/cognitive/activate` | Activate working memory (spreading activation) |
| `POST` | `/v1/cognitive/reflect` | Metacognitive reflection |
| `GET` | `/v1/cognitive/calibration` | Calibration metrics (ECE / MCE / Brier) |
| `GET` | `/v1/cognitive/health` | Knowledge health |
| `GET` | `/v1/graph/entities` | Extracted entities |
| `POST` | `/v1/graph/traverse` | Traverse relationship graph |
| `POST` | `/v1/episodes` | Store an episode |
| `POST` | `/v1/procedures/match` | Find matching learned skills |
| `GET` | `/v1/schemas` | List schemas (mental models) |
| `POST` | `/v1/feedback` | Record feedback signal |

### Billing & Settings

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/v1/billing` | Subscription plan & usage |
| `POST` | `/v1/billing/checkout` | Create Razorpay subscription (returns `subscription_id` + `key_id`) |
| `POST` | `/v1/billing/verify` | Verify the Checkout modal's payment signature |
| `POST` | `/v1/billing/cancel` | Cancel the org's subscription |
| `GET` `PUT` | `/v1/settings` | Per-tenant engine tuning |

## Managed Cloud & Plans

The server enforces per-org plans and usage quotas (memories written, recalls, agent count) when billing is enabled. Billing is **gated on `RAZORPAY_KEY_ID` + `RAZORPAY_KEY_SECRET`** — leave them unset for unlimited self-hosted use. Self-serve upgrades use the embedded Razorpay Checkout modal; plan state is reconciled from the `subscription.*` webhook.

| Plan | Agents | Memories / mo |
|---|---|---|
| Free | 1 | 1,000 |
| Developer | 5 | 50,000 |
| Team | 25 | 500,000 |
| Growth | 100 | 5,000,000 |
| Enterprise | Unlimited | Custom |

## Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| `DATABASE_URL` | - | PostgreSQL connection string |
| `SERVER_PORT` | 8080 | HTTP server port |
| `LLM_PROVIDER` | openai | LLM provider (`openai`, `anthropic`, `gemini`, `cerebras`, `none`) |
| `EMBEDDING_PROVIDER` | openai | Embedding provider |
| `OPENAI_API_KEY` | - | OpenAI API key |
| `ANTHROPIC_API_KEY` | - | Anthropic API key |
| `ENGRAM_SETUP_TOKEN` | - | Token gating `POST /v1/setup` |
| `RAZORPAY_KEY_ID` / `RAZORPAY_KEY_SECRET` | - | Enables billing/quota enforcement when both set |
| `RATE_LIMIT_RPS` | 100 | Requests per second |
| `LOG_LEVEL` | info | Log level |

Set `LLM_PROVIDER=none` for embedding-only mode (no external LLM calls, P99 < 150 ms) — recall and decay still work; LLM-based extraction and contradiction analysis degrade gracefully.

## Development

```bash
# Install mise (https://mise.jdx.dev)
mise install

# Run tests
mise run test

# Run linter
mise run lint

# Start server
mise run serve

# Build binary
mise run build
```

## License

Apache 2.0
</content>
</invoke>
