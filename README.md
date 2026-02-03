# Engram

**Cognitive infrastructure for AI agents that learn and improve from experience.**

Engram provides persistent, adaptive memory for AI agents. Store beliefs with confidence tracking, reinforce knowledge through repetition, detect contradictions, and recall relevant context via a simple HTTP API. This enables agents to become more consistent, personalized, and effective the longer they run.

## TL;DR

- Drop-in cognitive memory service for agents
- Stores beliefs with confidence
- Reinforces repeated knowledge
- Detects contradictions
- Recalls relevant context
- Learns from usage over time

## Why Engram Exists

Most agent frameworks treat memory as a passive vector store. Engram treats memory as a dynamic belief system: memories gain confidence when reinforced, lose confidence when contradicted, and fade when unused. This allows agents to accumulate knowledge over time instead of repeatedly re-discovering it.

## Philosophy

- **Memory is belief, not storage.** Confidence matters as much as content.
- **Repetition builds conviction.** Reinforced knowledge becomes stronger.
- **Contradictions surface truth.** Conflicting beliefs compete; the system adapts.
- **Unused knowledge fades.** Decay keeps memory relevant and focused.
- **Simplicity over complexity.** One HTTP API, no custom models, no ML training.

## Core Capabilities

- **Belief Storage** with semantic embeddings and confidence tracking
- **Automatic Reinforcement** when similar statements are repeated
- **Contradiction Detection** with confidence adjustment
- **Memory Classification** into types (preferences, facts, decisions, constraints)
- **Conversation Extraction** to automatically capture memories
- **Semantic Recall** with confidence-filtered retrieval
- **Episodic Memory** for rich, contextual experiences
- **Policy-Based Tuning** using feedback signals

## Quick Start

### 1. Run with Docker

```bash
git clone https://github.com/yourusername/engram.git
cd engram

# Copy example env and configure
cp .env.example .env

# Start the stack
docker compose up -d

# Run migrations
docker compose exec server /engram migrate
```

### 2. Create a Tenant and Get an API Key

```bash
curl -X POST http://localhost:8080/v1/tenants \
  -H "Content-Type: application/json" \
  -d '{"name": "My App"}'
```

Response:
```json
{
  "id": "550e8400-e29b-41d4-a716-446655440000",
  "name": "My App",
  "api_key": "eg_abc123..."
}
```

### 3. Register an Agent

```bash
export API_KEY="eg_abc123..."

curl -X POST http://localhost:8080/v1/agents \
  -H "Authorization: Bearer $API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "external_id": "support-bot-1",
    "name": "Customer Support Agent"
  }'
```

### 4. Store a Memory

```bash
curl -X POST http://localhost:8080/v1/memories \
  -H "Authorization: Bearer $API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "agent_id": "AGENT_UUID",
    "content": "User prefers dark mode in all interfaces",
    "type": "preference",
    "confidence": 0.95
  }'
```

### 5. Recall Relevant Memories

```bash
curl "http://localhost:8080/v1/memories/recall?agent_id=AGENT_UUID&query=what+display+settings+does+the+user+prefer" \
  -H "Authorization: Bearer $API_KEY"
```

Response:
```json
{
  "memories": [
    {
      "id": "...",
      "type": "preference",
      "content": "User prefers dark mode in all interfaces",
      "confidence": 0.95,
      "score": 0.87
    }
  ],
  "query": "what display settings does the user prefer",
  "count": 1
}
```

## Integration Guide

### Injecting Memories into Agent Prompts

```python
import requests

def get_relevant_memories(agent_id: str, context: str) -> list:
    """Recall memories relevant to the current context."""
    response = requests.get(
        f"http://localhost:8080/v1/memories/recall",
        params={"agent_id": agent_id, "query": context, "top_k": 5},
        headers={"Authorization": f"Bearer {API_KEY}"}
    )
    return response.json()["memories"]

def build_prompt(user_message: str, agent_id: str) -> str:
    """Build a prompt with relevant memories injected."""
    memories = get_relevant_memories(agent_id, user_message)

    memory_context = "\n".join([
        f"- [{m['type']}] {m['content']}"
        for m in memories
    ])

    return f"""You are a helpful assistant. Use these memories about the user:

{memory_context}

User: {user_message}
Assistant:"""
```

### Extracting Memories from Conversations

```bash
curl -X POST http://localhost:8080/v1/memories/extract \
  -H "Authorization: Bearer $API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "agent_id": "AGENT_UUID",
    "conversation": [
      {"role": "user", "content": "I always want responses in bullet points"},
      {"role": "assistant", "content": "Got it, I will use bullet points."},
      {"role": "user", "content": "Also, never suggest paid tools. I only use open source."}
    ],
    "auto_store": true
  }'
```

Response:
```json
{
  "extracted": [
    {
      "id": "...",
      "type": "preference",
      "content": "User prefers responses formatted as bullet points",
      "confidence": 0.9,
      "stored": true
    },
    {
      "id": "...",
      "type": "constraint",
      "content": "User only uses open source tools; never suggest paid alternatives",
      "confidence": 0.95,
      "stored": true
    }
  ],
  "count": 2
}
```

## Memory Types

| Type | Description | Example |
|------|-------------|---------|
| `preference` | User likes/dislikes, style preferences | "Prefers concise responses" |
| `fact` | Information about the user or context | "Works as a software engineer" |
| `decision` | Choices the user has made | "Decided to use PostgreSQL" |
| `constraint` | Hard rules or limitations | "Never suggest proprietary software" |

## Belief System

Engram treats each memory as a **belief** with epistemic properties:

- **Confidence** (0.0-1.0): How certain the system is about this belief
- **Reinforcement**: Repeated similar statements increase confidence
- **Contradiction Detection**: Conflicting beliefs decrease confidence
- **Source Tracking**: Where the belief originated

### Reinforcement

When you store a belief similar to an existing one (similarity > 0.85):
- Existing belief's confidence increases by +0.05 (max 0.99)
- `reinforcement_count` increments
- `last_verified_at` updates to current time
- No duplicate row is created

```bash
# First store
curl -X POST http://localhost:8080/v1/memories \
  -H "Authorization: Bearer $API_KEY" \
  -d '{"agent_id": "...", "content": "User prefers dark mode", "confidence": 0.9}'
# Response: {"id": "abc", "confidence": 0.9, "reinforcement_count": 1, "reinforced": false}

# Second store (similar content)
curl -X POST http://localhost:8080/v1/memories \
  -H "Authorization: Bearer $API_KEY" \
  -d '{"agent_id": "...", "content": "User likes dark mode"}'
# Response: {"id": "abc", "confidence": 0.95, "reinforcement_count": 2, "reinforced": true}
```

### Contradiction Detection

When a new belief contradicts an existing one:
- Old belief's confidence decreases by -0.2 (min 0.1)
- New belief is stored with confidence 0.7
- Both beliefs are linked in the contradiction graph

```bash
# Existing: "User prefers dark mode" (confidence: 0.9)
curl -X POST http://localhost:8080/v1/memories \
  -H "Authorization: Bearer $API_KEY" \
  -d '{"agent_id": "...", "content": "User prefers light mode"}'
# Result: Old belief confidence -> 0.7, new belief created with confidence 0.7
```

### Confidence-Filtered Retrieval

By default, recall only returns beliefs with confidence >= 0.6.

```bash
# Returns only high-confidence beliefs by default
curl "http://localhost:8080/v1/memories/recall?agent_id=...&query=theme+preference"

# Override to include lower-confidence beliefs
curl "http://localhost:8080/v1/memories/recall?agent_id=...&query=theme+preference&min_confidence=0.3"
```

### Memory Response Fields

| Field | Description |
|-------|-------------|
| `confidence` | Current confidence level (0.0-1.0) |
| `reinforcement_count` | Number of times this belief was reinforced |
| `last_verified_at` | Last time this belief was reinforced |
| `reinforced` | (create response only) Whether an existing belief was reinforced |

## Episodic Memory

In addition to semantic beliefs, Engram supports **episodic memory** for rich experiences with full context. While semantic memories store extracted facts, episodes preserve raw interactions along with emotional context, entities, and causal relationships.

### Semantic vs Episodic

Semantic only:
```
Input: "I hate light mode, it hurts my eyes"
Output: Store "user prefers dark mode"
```

Episodic:
```
Input: "I hate light mode, it hurts my eyes"
Output: Episode {
  raw: "I hate light mode, it hurts my eyes",
  entities: ["light mode", "eyes"],
  causal_links: ["light mode -> eye strain"],
  emotional_valence: -0.7,
  importance: 0.8
}
```

### Store an Episode

```bash
curl -X POST http://localhost:8080/v1/episodes \
  -H "Authorization: Bearer $API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "agent_id": "AGENT_UUID",
    "raw_content": "User: I hate light mode, it hurts my eyes\nAssistant: I understand, dark mode can be easier on the eyes.",
    "conversation_id": "conv-123",
    "occurred_at": "2024-01-15T23:45:00Z"
  }'
```

Response:
```json
{
  "id": "ep-uuid",
  "raw_content": "...",
  "entities": ["light mode", "eyes", "dark mode"],
  "topics": ["display preferences", "eye strain"],
  "causal_links": [{"cause": "light mode", "effect": "eye strain", "confidence": 0.9}],
  "emotional_valence": -0.5,
  "emotional_intensity": 0.7,
  "importance_score": 0.8,
  "time_of_day": "night",
  "day_of_week": "Monday",
  "consolidation_status": "raw",
  "memory_strength": 1.0
}
```

### Recall Episodes

```bash
# By semantic query
curl "http://localhost:8080/v1/episodes/recall?agent_id=AGENT_UUID&query=display+preferences" \
  -H "Authorization: Bearer $API_KEY"

# By time range
curl "http://localhost:8080/v1/episodes/recall?agent_id=AGENT_UUID&start_time=2024-01-01T00:00:00Z&end_time=2024-01-31T23:59:59Z" \
  -H "Authorization: Bearer $API_KEY"

# By minimum importance
curl "http://localhost:8080/v1/episodes/recall?agent_id=AGENT_UUID&min_importance=0.7" \
  -H "Authorization: Bearer $API_KEY"
```

### Record Outcome

```bash
curl -X POST http://localhost:8080/v1/episodes/ep-uuid/outcome \
  -H "Authorization: Bearer $API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "outcome": "success",
    "description": "User was satisfied with the dark mode suggestion"
  }'
```

Outcome types: `success`, `failure`, `neutral`, `unknown`

### Episode Associations

Episodes are automatically linked to similar past episodes:

```bash
curl http://localhost:8080/v1/episodes/ep-uuid/associations \
  -H "Authorization: Bearer $API_KEY"
```

### Memory Strength & Decay

Episodes have a `memory_strength` (0-1) that decays over time:
- Recently accessed episodes maintain high strength
- Unaccessed episodes decay based on `decay_rate`
- High-importance episodes decay slower
- Weak episodes (strength < 0.1) are archived

## Learning & Forgetting

Engram implements cognitive learning behaviors:

### Time-Based Decay

Both semantic beliefs and episodic memories decay over time without reinforcement:

```
Day 1:  Store "user prefers dark mode" (confidence 0.9)
Day 5:  User mentions dark mode again -> confidence 0.95 (reinforcement)
Day 10: Recalled in a query -> confidence 0.97 (usage reinforcement)
Day 30: No interaction -> confidence ~0.7 (decay)
Day 60: confidence ~0.4 -> "decaying" status
Day 90: confidence < 0.2 -> archived
```

- **Semantic memories** decay slowly (~50% after 30 days without access)
- **Episodic memories** decay faster (~50% after 7 days without access)
- **High reinforcement** slows decay
- **Archived memories** are soft-deleted when confidence falls below threshold

### Usage Reinforcement

When memories are recalled:
- Each recall adds +0.02 to confidence (max 0.99)
- Access count is tracked
- Last accessed timestamp is updated

### Episode-to-Belief Extraction

When an episode is created:
1. LLM analyzes the episode content
2. Extracts typed memories (preferences, facts, decisions, constraints)
3. Stores with slightly discounted confidence (0.8x)
4. Links extracted memories to source episode

### Decay Status

| Status | Confidence | Meaning |
|--------|------------|---------|
| `healthy` | >= 0.7 | Strong, recently reinforced |
| `decaying` | 0.4-0.7 | Needs reinforcement |
| `at_risk` | < 0.4 | May be archived soon |

### Manual Decay Trigger

```bash
curl -X POST http://localhost:8080/v1/cognitive/decay \
  -H "Authorization: Bearer $API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"agent_id": "AGENT_UUID"}'
```

Response:
```json
{
  "memories_decayed": 5,
  "memories_archived": 1,
  "episodes_decayed": 3,
  "episodes_archived": 0
}
```

## API Reference

### Endpoints

| Method | Endpoint | Description |
|--------|----------|-------------|
| `POST` | `/v1/tenants` | Create tenant (returns API key) |
| `POST` | `/v1/agents` | Register an agent |
| `GET` | `/v1/agents/:id` | Get agent details |
| `POST` | `/v1/memories` | Store a memory |
| `GET` | `/v1/memories/:id` | Get a memory |
| `DELETE` | `/v1/memories/:id` | Delete a memory |
| `GET` | `/v1/memories/recall` | Recall relevant memories |
| `POST` | `/v1/memories/extract` | Extract memories from conversation |
| `POST` | `/v1/episodes` | Store an episode |
| `GET` | `/v1/episodes/:id` | Get an episode |
| `GET` | `/v1/episodes/recall` | Recall episodes |
| `POST` | `/v1/episodes/:id/outcome` | Record episode outcome |
| `GET` | `/v1/episodes/:id/associations` | Get episode associations |
| `GET` | `/v1/agents/:id/policies` | Get agent policies |
| `PUT` | `/v1/agents/:id/policies` | Update agent policies |
| `POST` | `/v1/feedback` | Submit feedback on a memory |
| `POST` | `/v1/cognitive/decay` | Trigger decay for an agent |
| `GET` | `/health` | Health check |
| `GET` | `/metrics` | Server metrics |

### Authentication

All `/v1/*` endpoints require an API key:

```
Authorization: Bearer <api_key>
```

### Recall Parameters

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `query` | string | required | Text to match against |
| `agent_id` | uuid | required | Agent to query |
| `top_k` | int | 10 | Max results |
| `type` | string | - | Filter by memory type |
| `min_confidence` | float | 0.6 | Minimum confidence threshold |

## Policies

Control memory behavior per agent:

```bash
curl -X PUT http://localhost:8080/v1/agents/AGENT_UUID/policies \
  -H "Authorization: Bearer $API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "policies": [
      {
        "memory_type": "preference",
        "max_memories": 50,
        "retention_days": 90,
        "priority_weight": 1.2,
        "auto_summarize": true
      }
    ]
  }'
```

### Policy Options

| Field | Type | Description |
|-------|------|-------------|
| `memory_type` | string | preference, fact, decision, constraint |
| `max_memories` | int | Maximum memories of this type per agent |
| `retention_days` | int | Auto-delete after N days (null = forever) |
| `priority_weight` | float | Boost/reduce recall priority (1.0 = neutral) |
| `auto_summarize` | bool | Summarize when exceeding max_memories |

## Feedback Loop

Improve memory quality with feedback:

```bash
curl -X POST http://localhost:8080/v1/feedback \
  -H "Authorization: Bearer $API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "memory_id": "MEMORY_UUID",
    "agent_id": "AGENT_UUID",
    "signal_type": "helpful"
  }'
```

Signal types: `used`, `ignored`, `helpful`, `unhelpful`

The policy tuner automatically adjusts `priority_weight` based on feedback patterns.

## Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| `DATABASE_URL` | - | PostgreSQL connection string |
| `SERVER_PORT` | 8080 | HTTP server port |
| `LLM_PROVIDER` | openai | LLM for classification (openai, anthropic, mock) |
| `EMBEDDING_PROVIDER` | openai | Embedding provider (openai, mock) |
| `OPENAI_API_KEY` | - | OpenAI API key |
| `RATE_LIMIT_RPS` | 100 | Requests per second limit |
| `RATE_LIMIT_BURST` | 20 | Burst size for rate limiting |
| `LOG_LEVEL` | info | Log level (debug, info, warn, error) |

## Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                         Your Application                        │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌──────────────────┐ │
│  │  Agent 1 │  │  Agent 2 │  │  Agent 3 │  │       ...        │ │
│  └────┬─────┘  └────┬─────┘  └────┬─────┘  └────────┬─────────┘ │
└───────┼─────────────┼─────────────┼─────────────────┼───────────┘
        │             │             │                 │
        └─────────────┴──────┬──────┴─────────────────┘
                             │
                             ▼
┌─────────────────────────────────────────────────────────────────┐
│                         Engram API                              │
│  ┌─────────────────────────────────────────────────────────┐    │
│  │                    HTTP API (chi)                       │    │
│  │  /v1/memories  /v1/episodes  /v1/agents  /v1/feedback   │    │
│  └─────────────────────────────────────────────────────────┘    │
│                             │                                    │
│  ┌──────────────────────────┴──────────────────────────────┐    │
│  │                   Service Layer                          │    │
│  │  ┌─────────┐ ┌──────────┐ ┌─────────┐ ┌──────────────┐  │    │
│  │  │ Memory  │ │ Episode  │ │ Policy  │ │  Cognitive   │  │    │
│  │  │ Service │ │ Service  │ │ Tuner   │ │   Service    │  │    │
│  │  └────┬────┘ └────┬─────┘ └────┬────┘ └──────┬───────┘  │    │
│  └───────┼───────────┼────────────┼─────────────┼──────────┘    │
│          │           │            │             │                │
│  ┌───────┴───────────┴────────────┴─────────────┴──────────┐    │
│  │                    Store Layer (pgx)                     │    │
│  └──────────────────────────┬──────────────────────────────┘    │
└─────────────────────────────┼───────────────────────────────────┘
                              │
        ┌─────────────────────┼─────────────────────┐
        │                     │                     │
        ▼                     ▼                     ▼
┌───────────────┐    ┌───────────────┐    ┌───────────────┐
│  PostgreSQL   │    │    OpenAI     │    │    OpenAI     │
│  + pgvector   │    │     LLM       │    │   Embeddings  │
│               │    │ (gpt-4o-mini) │    │(text-embed-3) │
└───────────────┘    └───────────────┘    └───────────────┘
```

## Deployment

### Docker Compose

```bash
cp .env.example .env
# Configure your environment variables
docker compose up -d
```

### From Source

```bash
# Install mise (https://mise.jdx.dev)
mise install

# Start dependencies
docker compose up -d postgres

# Run migrations
mise run db:migrate

# Start server
mise run serve
```

## Development

```bash
# Run tests
mise run test

# Run linter
mise run lint

# Build binary
mise run build

# Build Docker image
mise run docker:build
```

## License

MIT
