// Example: Learning Agent
//
// Demonstrates core Engram capabilities:
//   - Belief storage with confidence levels
//   - Reinforcement through repetition
//   - Contradiction detection
//   - Semantic recall
//
// Usage:
//   export ENGRAM_API_KEY=<your-api-key>
//   go run ./examples/01_learning_agent.go

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
)

var (
	baseURL = "http://localhost:8080"
	apiKey  = os.Getenv("ENGRAM_API_KEY")
	agentID string
)

func main() {
	if apiKey == "" {
		fmt.Println("Error: ENGRAM_API_KEY environment variable required")
		os.Exit(1)
	}

	fmt.Println("=== Engram Learning Agent ===")
	fmt.Println()

	// Create agent
	fmt.Println("1. Creating agent...")
	agentID = createAgent("learning-demo", "Learning Demo Agent")
	fmt.Printf("   Agent ID: %s\n\n", agentID)

	// Store initial belief
	fmt.Println("2. Storing preference: dark mode...")
	mem1 := storeMemory("User prefers dark mode for all interfaces", "preference", 0.85)
	fmt.Printf("   Confidence: %.2f\n\n", mem1.Confidence)

	// Reinforce belief
	fmt.Println("3. Reinforcing: user mentions dark mode again...")
	mem2 := storeMemory("User likes dark themes and dark mode", "preference", 0.80)
	fmt.Printf("   Reinforced: %v, Confidence: %.2f, Count: %d\n\n",
		mem2.Reinforced, mem2.Confidence, mem2.ReinforcementCount)

	// Store additional beliefs
	fmt.Println("4. Storing preference: concise responses...")
	mem3 := storeMemory("User prefers short, concise responses", "preference", 0.90)
	fmt.Printf("   Confidence: %.2f\n\n", mem3.Confidence)

	fmt.Println("5. Storing fact: user background...")
	mem4 := storeMemory("User is a backend engineer working with Go", "fact", 0.95)
	fmt.Printf("   Confidence: %.2f\n\n", mem4.Confidence)

	// Contradiction
	fmt.Println("6. Contradicting belief: user now prefers light mode...")
	mem5 := storeMemory("User prefers light mode for better readability", "preference", 0.75)
	fmt.Printf("   New belief stored: %.2f\n\n", mem5.Confidence)

	// Recall
	fmt.Println("7. Recalling: display preferences...")
	memories := recallMemories("user display theme preferences", 5)
	fmt.Printf("   Found %d memories:\n", len(memories))
	for _, m := range memories {
		fmt.Printf("   - [%s] %.2f: %s\n", m.Type, m.Confidence, truncate(m.Content, 45))
	}
	fmt.Println()

	fmt.Println("8. Recalling: programming background...")
	memories2 := recallMemories("programming languages user knows", 5)
	fmt.Printf("   Found %d memories:\n", len(memories2))
	for _, m := range memories2 {
		fmt.Printf("   - [%s] %.2f: %s\n", m.Type, m.Confidence, truncate(m.Content, 45))
	}

	fmt.Println("\n=== Complete ===")
}

// Types

type Agent struct {
	ID string `json:"id"`
}

type Memory struct {
	ID                 string  `json:"id"`
	Type               string  `json:"type"`
	Content            string  `json:"content"`
	Confidence         float64 `json:"confidence"`
	ReinforcementCount int     `json:"reinforcement_count"`
	Reinforced         bool    `json:"reinforced"`
}

type RecallResponse struct {
	Memories []Memory `json:"memories"`
}

// API

func createAgent(externalID, name string) string {
	resp := post("/v1/agents", map[string]string{"external_id": externalID, "name": name})
	var agent Agent
	json.Unmarshal(resp, &agent)
	return agent.ID
}

func storeMemory(content, memType string, confidence float64) Memory {
	resp := post("/v1/memories", map[string]any{
		"agent_id": agentID, "content": content, "type": memType, "confidence": confidence,
	})
	var mem Memory
	json.Unmarshal(resp, &mem)
	return mem
}

func recallMemories(query string, topK int) []Memory {
	params := url.Values{"agent_id": {agentID}, "query": {query}, "top_k": {fmt.Sprintf("%d", topK)}}
	resp := get("/v1/memories/recall?" + params.Encode())
	var result RecallResponse
	json.Unmarshal(resp, &result)
	return result.Memories
}

// HTTP

func post(path string, body any) []byte {
	data, _ := json.Marshal(body)
	req, _ := http.NewRequest("POST", baseURL+path, bytes.NewReader(data))
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}
	defer resp.Body.Close()
	result, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		fmt.Printf("API Error (%d): %s\n", resp.StatusCode, string(result))
		os.Exit(1)
	}
	return result
}

func get(path string) []byte {
	req, _ := http.NewRequest("GET", baseURL+path, nil)
	req.Header.Set("Authorization", "Bearer "+apiKey)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}
	defer resp.Body.Close()
	result, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		fmt.Printf("API Error (%d): %s\n", resp.StatusCode, string(result))
		os.Exit(1)
	}
	return result
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-3] + "..."
}
