// Seed script for creating demo data in Engram.
// Run with: go run ./scripts/seed.go
package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"log"
	"os"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
)

func main() {
	// Load environment
	envFile := os.Getenv("ENGRAM_ENV")
	if envFile == "" {
		envFile = ".env"
	}
	_ = godotenv.Load(envFile)
	_ = godotenv.Load(envFile + ".secret")

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgres://engram:engram@localhost:5432/engram?sslmode=disable"
	}

	ctx := context.Background()

	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		log.Fatalf("Failed to ping database: %v", err)
	}

	fmt.Println("Connected to database")

	// Generate API key
	apiKey := generateAPIKey()
	apiKeyHash := hashAPIKey(apiKey)

	// Create demo tenant
	tenantID := uuid.New()
	_, err = pool.Exec(ctx, `
		INSERT INTO tenants (id, name, api_key_hash)
		VALUES ($1, $2, $3)
		ON CONFLICT (api_key_hash) DO NOTHING
	`, tenantID, "Demo Tenant", apiKeyHash)
	if err != nil {
		log.Fatalf("Failed to create tenant: %v", err)
	}
	fmt.Printf("Created tenant: %s\n", tenantID)
	fmt.Printf("API Key: %s\n", apiKey)
	fmt.Println("(Save this API key - it cannot be retrieved later)")

	// Create demo agent
	agentID := uuid.New()
	_, err = pool.Exec(ctx, `
		INSERT INTO agents (id, tenant_id, external_id, name, metadata)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (tenant_id, external_id) DO NOTHING
	`, agentID, tenantID, "demo-agent-1", "Demo Support Agent", `{"version": "1.0", "purpose": "demo"}`)
	if err != nil {
		log.Fatalf("Failed to create agent: %v", err)
	}
	fmt.Printf("Created agent: %s (external_id: demo-agent-1)\n", agentID)

	// Create sample memories
	memories := []struct {
		memType    string
		content    string
		source     string
		confidence float64
	}{
		{"preference", "User prefers dark mode in all interfaces", "onboarding", 0.95},
		{"preference", "User likes responses formatted as bullet points", "conversation-001", 0.9},
		{"fact", "User is a software engineer working on backend systems", "profile", 1.0},
		{"fact", "User's primary programming language is Go", "conversation-002", 0.85},
		{"constraint", "Never suggest proprietary or paid tools - user only uses open source", "conversation-003", 0.98},
		{"constraint", "Keep responses under 500 words unless explicitly asked for detail", "feedback", 0.88},
		{"decision", "User decided to use PostgreSQL for the new project", "conversation-004", 0.92},
		{"decision", "User chose to implement microservices architecture", "conversation-005", 0.87},
	}

	for _, m := range memories {
		memID := uuid.New()
		_, err = pool.Exec(ctx, `
			INSERT INTO memories (id, agent_id, tenant_id, type, content, source, confidence, metadata)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		`, memID, agentID, tenantID, m.memType, m.content, m.source, m.confidence, "{}")
		if err != nil {
			log.Printf("Warning: Failed to create memory: %v", err)
		} else {
			fmt.Printf("Created memory [%s]: %s\n", m.memType, truncate(m.content, 50))
		}
	}

	// Create default policies
	policyTypes := []string{"preference", "fact", "decision", "constraint"}
	for _, ptype := range policyTypes {
		_, err = pool.Exec(ctx, `
			INSERT INTO memory_policies (agent_id, memory_type, max_memories, retention_days, priority_weight, auto_summarize)
			VALUES ($1, $2, $3, $4, $5, $6)
			ON CONFLICT (agent_id, memory_type) DO NOTHING
		`, agentID, ptype, 100, 90, 1.0, false)
		if err != nil {
			log.Printf("Warning: Failed to create policy for %s: %v", ptype, err)
		}
	}
	fmt.Println("Created default policies for all memory types")

	fmt.Println("\n=== Seed Complete ===")
	fmt.Println("\nTo test the API, use:")
	fmt.Printf("curl -H 'Authorization: Bearer %s' http://localhost:8080/v1/agents/%s\n", apiKey, agentID)
	fmt.Printf("\nTo recall memories:")
	fmt.Printf("\ncurl -H 'Authorization: Bearer %s' 'http://localhost:8080/v1/memories/recall?agent_id=%s&query=user+preferences'\n", apiKey, agentID)
}

func generateAPIKey() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		log.Fatalf("Failed to generate API key: %v", err)
	}
	return "mz_" + base64.URLEncoding.EncodeToString(b)[:40]
}

func hashAPIKey(key string) string {
	h := sha256.Sum256([]byte(key))
	return hex.EncodeToString(h[:])
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
