package llm

const extractPrompt = `You are a memory extraction system. Analyze the following conversation and extract distinct memories.

For each memory, determine:
- type: one of "preference", "fact", "decision", "constraint"
- content: a clear, concise statement of the memory
- evidence_type: how this belief was derived:
  - "explicit_statement": user directly stated this
  - "implicit_inference": inferred from indirect statements or patterns
  - "behavioral_signal": observed from user actions or behavior

Respond ONLY with a JSON array. No markdown, no explanation. Example:
[{"type":"preference","content":"User prefers dark mode","evidence_type":"explicit_statement"}]

If no memories can be extracted, respond with an empty array: []

Conversation:
%s`

const summarizePrompt = `You are a memory summarizer. Given the following memories, produce a single concise summary that captures the key information.

Each memory is tagged with its provenance:
- [USER] = directly stated by the user
- [INFERRED] = inferred from behavior or context
- [TOOL] = provided by an external tool
- [AGENT] = determined by the agent
- [DERIVED] = derived from other memories

Weight [USER] and [TOOL] memories more heavily than [INFERRED] or [DERIVED].

Memories:
%s

Respond with ONLY the summary text. No explanation, no formatting.`

const contradictionPrompt = `Do these two statements contradict each other?
Statement A: %s
Statement B: %s

Answer only "true" or "false". No explanation.`

const tensionPrompt = `Analyze the relationship between these two statements:
Statement A: %s
Statement B: %s

Classify the tension type:
- none: No conflict, compatible statements
- hard: Direct logical contradiction, both cannot be true
- soft: Some tension but both could be true in different ways
- contextual: Depends on unstated context or conditions
- temporal: True at different times (belief evolution)

Respond ONLY with JSON, no markdown:
{"type":"none|hard|soft|contextual|temporal","tension_score":0.0,"explanation":"brief reason"}`

const episodeExtractionPrompt = `Analyze this experience and extract structured information.

Experience: %s

Extract:
1. entities: List of key entities mentioned (people, things, concepts)
2. topics: Main topics/themes
3. causal_links: Any cause-effect relationships (as [{cause, effect, confidence}])
4. emotional_valence: Overall sentiment (-1 negative to +1 positive)
5. emotional_intensity: How strong is the emotion (0 to 1)
6. importance_score: How important/memorable is this (0 to 1)

Respond ONLY with JSON, no markdown fences:
{
  "entities": ["entity1", "entity2"],
  "topics": ["topic1", "topic2"],
  "causal_links": [{"cause": "X", "effect": "Y", "confidence": 0.8}],
  "emotional_valence": 0.0,
  "emotional_intensity": 0.5,
  "importance_score": 0.5
}`

const procedureExtractionPrompt = `Analyze this successful interaction and extract the trigger-action pattern (skill/procedure).

Interaction: %s

Extract a reusable procedure:
1. trigger_pattern: A description of the situation/trigger that should invoke this procedure
2. trigger_keywords: Key words/phrases that indicate this trigger
3. action_template: The response pattern or approach that worked
4. action_type: One of "response_style", "problem_solving", "communication", "workflow"

Respond ONLY with JSON, no markdown fences:
{
  "trigger_pattern": "When user is frustrated about X",
  "trigger_keywords": ["frustrated", "annoyed", "upset"],
  "action_template": "Acknowledge feelings first, then provide solution",
  "action_type": "communication"
}`

const schemaPatternPrompt = `Analyze these related memories and detect if they represent a coherent mental model (schema) of the user.

Memories:
%s

Look for patterns that suggest:
- User archetypes (communication style, expertise level, preferences pattern)
- Situation templates (recurring context patterns)
- Causal models (if X then Y patterns)

If a schema is detected, extract:
1. schema_type: one of "user_archetype", "situation_template", "causal_model"
2. name: short descriptive name (e.g., "Night-owl power user", "Technical expert")
3. description: 1-2 sentence description
4. attributes: key attributes as a JSON object
5. applicable_contexts: when this schema applies
6. confidence: 0.0-1.0 based on evidence strength

If no clear schema pattern is detected, return null.

Respond ONLY with JSON, no markdown fences:
{
  "schema_type": "user_archetype",
  "name": "Technical Expert",
  "description": "User has deep technical knowledge and prefers detailed explanations",
  "attributes": {"expertise_level": "expert", "communication_style": "technical"},
  "applicable_contexts": ["debugging", "architecture"],
  "confidence": 0.8
}`
