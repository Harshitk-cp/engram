# Engram

**Cognitive infrastructure for AI agents that learn and improve from experience.**

Engram stores, retrieves, and injects typed memory into agent prompts via a simple HTTP API. Memories gain confidence when reinforced, lose confidence when contradicted, and fade when unused.

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

### Basic Usage

```bash
# 1. Create tenant (returns API key)
curl -X POST http://localhost:8080/v1/tenants \
  -H "Content-Type: application/json" \
  -d '{"name": "My App"}'

# 2. Register agent
curl -X POST http://localhost:8080/v1/agents \
  -H "Authorization: Bearer $API_KEY" \
  -d '{"external_id": "agent-1", "name": "My Agent"}'

# 3. Store memory
curl -X POST http://localhost:8080/v1/memories \
  -H "Authorization: Bearer $API_KEY" \
  -d '{"agent_id": "AGENT_UUID", "content": "User prefers dark mode"}'

# 4. Recall memories
curl "http://localhost:8080/v1/memories/recall?agent_id=AGENT_UUID&query=display+preferences" \
  -H "Authorization: Bearer $API_KEY"
```

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

## Memory Systems

Engram implements four memory types inspired by cognitive science:

| System | Purpose | API |
|--------|---------|-----|
| **Semantic** | Facts, preferences, beliefs | `/v1/memories` |
| **Episodic** | Rich experiences with context | `/v1/episodes` |
| **Procedural** | Learned skills and patterns | `/v1/procedures` |
| **Working** | Active context and goals | `/v1/cognitive/session` |

Plus **Schemas** for mental models: `/v1/schemas`

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

### Metacognition

Self-assessment of memory quality:

```bash
# Reflect on memory state
curl -X POST http://localhost:8080/v1/cognitive/reflect \
  -H "Authorization: Bearer $API_KEY" \
  -d '{"agent_id": "..."}'

# Assess confidence for a query
curl "http://localhost:8080/v1/cognitive/confidence?agent_id=...&query=..." \
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

### Core Endpoints

| Method | Endpoint | Description |
|--------|----------|-------------|
| `POST` | `/v1/tenants` | Create tenant (returns API key) |
| `POST` | `/v1/agents` | Register agent |
| `GET` | `/v1/agents/:id` | Get agent |
| `GET` | `/v1/agents/:id/mind` | Get agent's complete mental state |

### Memory Operations

| Method | Endpoint | Description |
|--------|----------|-------------|
| `POST` | `/v1/memories` | Store memory |
| `GET` | `/v1/memories/:id` | Get memory |
| `DELETE` | `/v1/memories/:id` | Delete memory |
| `GET` | `/v1/memories/recall` | Hybrid recall (vector + graph) |
| `POST` | `/v1/memories/extract` | Extract from conversation |

### Episodic Memory

| Method | Endpoint | Description |
|--------|----------|-------------|
| `POST` | `/v1/episodes` | Store episode |
| `GET` | `/v1/episodes/recall` | Recall episodes |
| `POST` | `/v1/episodes/:id/outcome` | Record outcome |

### Procedural Memory

| Method | Endpoint | Description |
|--------|----------|-------------|
| `POST` | `/v1/procedures/match` | Find matching procedures |
| `POST` | `/v1/procedures/learn` | Learn from episode |
| `POST` | `/v1/procedures/:id/outcome` | Record outcome |

### Schemas

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/v1/schemas` | List schemas |
| `POST` | `/v1/schemas/detect` | Detect from memories |
| `POST` | `/v1/schemas/:id/validate` | Validate schema |

### Graph Operations

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/v1/graph/entities` | List extracted entities |
| `GET` | `/v1/graph/relationships` | Get memory relationships |
| `POST` | `/v1/graph/traverse` | Traverse relationship graph |

### Cognitive Operations

| Method | Endpoint | Description |
|--------|----------|-------------|
| `POST` | `/v1/cognitive/decay` | Trigger decay |
| `POST` | `/v1/cognitive/consolidate` | Consolidate memories |
| `GET` | `/v1/cognitive/health` | Memory health stats |
| `POST` | `/v1/cognitive/activate` | Activate working memory |
| `GET` | `/v1/cognitive/session` | Get working memory session |
| `POST` | `/v1/cognitive/reflect` | Metacognitive reflection |
| `GET` | `/v1/cognitive/confidence` | Assess query confidence |
| `GET` | `/v1/cognitive/confidence/stats` | Confidence statistics |
| `POST` | `/v1/cognitive/confidence/reinforce` | Boost memory confidence |
| `POST` | `/v1/cognitive/confidence/penalize` | Penalize memory |

### Feedback & Policies

| Method | Endpoint | Description |
|--------|----------|-------------|
| `POST` | `/v1/feedback` | Submit feedback |
| `GET` | `/v1/agents/:id/policies` | Get policies |
| `PUT` | `/v1/agents/:id/policies` | Update policies |
| `GET` | `/v1/agents/:id/tier-stats` | Get tier statistics |

## Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| `DATABASE_URL` | - | PostgreSQL connection string |
| `SERVER_PORT` | 8080 | HTTP server port |
| `LLM_PROVIDER` | openai | LLM provider (openai, anthropic, gemini, cerebras) |
| `EMBEDDING_PROVIDER` | openai | Embedding provider |
| `OPENAI_API_KEY` | - | OpenAI API key |
| `ANTHROPIC_API_KEY` | - | Anthropic API key |
| `RATE_LIMIT_RPS` | 100 | Requests per second |
| `LOG_LEVEL` | info | Log level |

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
