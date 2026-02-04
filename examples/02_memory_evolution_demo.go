// Example: Memory Evolution Demo
//
// Interactive demonstration of memory dynamics over time:
//   - Phase 1: Initial memory formation
//   - Phase 2: Reinforcement through repetition
//   - Phase 3: Contradiction detection
//   - Phase 4: Episodic memory storage
//   - Phase 5: Memory decay
//   - Phase 6: Final state overview
//
// Usage:
//   export ENGRAM_API_KEY=<your-api-key>
//   go run ./examples/02_memory_evolution_demo.go

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
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

	printHeader("ENGRAM MEMORY EVOLUTION DEMO")

	agentID = createAgent("evolution-demo-"+time.Now().Format("150405"), "Evolution Demo Agent")
	fmt.Printf("Agent: %s\n\n", agentID)

	phase1()
	phase2()
	phase3()
	phase4()
	phase5()
	phase6()

	printHeader("DEMO COMPLETE")
}

func phase1() {
	printPhase("1", "INITIAL MEMORY FORMATION")

	memories := []struct {
		content    string
		memType    string
		confidence float64
	}{
		{"User prefers Python for scripting tasks", "preference", 0.85},
		{"User works at a fintech startup", "fact", 0.95},
		{"User decided to use PostgreSQL for the new project", "decision", 0.90},
		{"Never suggest Windows-only tools to this user", "constraint", 0.98},
		{"User likes detailed code explanations", "preference", 0.70},
	}

	for _, m := range memories {
		mem := storeMemory(m.content, m.memType, m.confidence)
		fmt.Printf("  [%s] %.2f  %s\n", m.memType, mem.Confidence, truncate(m.content, 45))
	}
	pause()
}

func phase2() {
	printPhase("2", "REINFORCEMENT")

	fmt.Println("  Repeating Python preference...")
	mem1 := storeMemory("User really likes Python for quick scripts", "preference", 0.80)
	fmt.Printf("  Result: confidence=%.2f, reinforced=%v, count=%d\n",
		mem1.Confidence, mem1.Reinforced, mem1.ReinforcementCount)

	fmt.Println("\n  Repeating again...")
	mem2 := storeMemory("Python is the user's go-to language for automation", "preference", 0.75)
	fmt.Printf("  Result: confidence=%.2f, reinforced=%v, count=%d\n",
		mem2.Confidence, mem2.Reinforced, mem2.ReinforcementCount)
	pause()
}

func phase3() {
	printPhase("3", "CONTRADICTION DETECTION")

	fmt.Println("  Current: User likes detailed explanations (confidence ~0.70)")
	fmt.Println("\n  Storing contradictory belief: user wants brief responses...")

	mem := storeMemory("User prefers brief, to-the-point responses", "preference", 0.85)
	fmt.Printf("  New belief: confidence=%.2f\n", mem.Confidence)

	fmt.Println("\n  Recalling response style preferences...")
	memories := recallMemories("How does the user like responses formatted?", 5, 0.0)
	for _, m := range memories {
		fmt.Printf("  [%.2f] %s\n", m.Confidence, truncate(m.Content, 50))
	}
	pause()
}

func phase4() {
	printPhase("4", "EPISODIC MEMORY")

	episode := storeEpisode(`User: I'm frustrated with the deployment process. It failed again.
Assistant: I understand. Let me help you set up better monitoring.
User: That would be great. I've been dealing with this for weeks.`)

	fmt.Println("  Episode stored:")
	fmt.Printf("    ID: %s\n", episode.ID)
	fmt.Printf("    Entities: %v\n", episode.Entities)
	fmt.Printf("    Emotional valence: %.2f\n", episode.EmotionalValence)
	fmt.Printf("    Importance: %.2f\n", episode.ImportanceScore)

	fmt.Println("\n  Recording outcome: success")
	recordOutcome(episode.ID, "success", "User satisfied with monitoring setup")
	pause()
}

func phase5() {
	printPhase("5", "MEMORY DECAY")

	fmt.Println("  Triggering decay process...")
	result := triggerDecay()

	fmt.Printf("    Memories decayed: %d\n", result.MemoriesDecayed)
	fmt.Printf("    Memories archived: %d\n", result.MemoriesArchived)
	fmt.Printf("    Episodes decayed: %d\n", result.EpisodesDecayed)
	fmt.Printf("    Episodes archived: %d\n", result.EpisodesArchived)
	pause()
}

func phase6() {
	printPhase("6", "FINAL STATE")

	memories := recallMemories("*", 20, 0.0)

	byType := make(map[string][]Memory)
	for _, m := range memories {
		byType[m.Type] = append(byType[m.Type], m)
	}

	for _, memType := range []string{"constraint", "fact", "decision", "preference"} {
		mems := byType[memType]
		if len(mems) == 0 {
			continue
		}
		fmt.Printf("\n  %s:\n", strings.ToUpper(memType))
		for _, m := range mems {
			bar := strings.Repeat("█", int(m.Confidence*10)) + strings.Repeat("░", 10-int(m.Confidence*10))
			fmt.Printf("    %s %.2f  %s\n", bar, m.Confidence, truncate(m.Content, 40))
		}
	}
	fmt.Println()
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

type Episode struct {
	ID               string   `json:"id"`
	Entities         []string `json:"entities"`
	EmotionalValence float64  `json:"emotional_valence"`
	ImportanceScore  float64  `json:"importance_score"`
}

type DecayResult struct {
	MemoriesDecayed  int `json:"memories_decayed"`
	MemoriesArchived int `json:"memories_archived"`
	EpisodesDecayed  int `json:"episodes_decayed"`
	EpisodesArchived int `json:"episodes_archived"`
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

func recallMemories(query string, topK int, minConf float64) []Memory {
	params := url.Values{
		"agent_id": {agentID}, "query": {query},
		"top_k": {fmt.Sprintf("%d", topK)}, "min_confidence": {fmt.Sprintf("%.2f", minConf)},
	}
	resp := get("/v1/memories/recall?" + params.Encode())
	var result RecallResponse
	json.Unmarshal(resp, &result)
	return result.Memories
}

func storeEpisode(rawContent string) Episode {
	resp := post("/v1/episodes", map[string]any{"agent_id": agentID, "raw_content": rawContent})
	var ep Episode
	json.Unmarshal(resp, &ep)
	return ep
}

func recordOutcome(episodeID, outcome, description string) {
	post("/v1/episodes/"+episodeID+"/outcome", map[string]string{
		"outcome": outcome, "description": description,
	})
}

func triggerDecay() DecayResult {
	resp := post("/v1/cognitive/decay", map[string]string{"agent_id": agentID})
	var result DecayResult
	json.Unmarshal(resp, &result)
	return result
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

// UI

func printHeader(title string) {
	fmt.Println("╔════════════════════════════════════════════════════════════╗")
	fmt.Printf("║  %-56s  ║\n", title)
	fmt.Println("╚════════════════════════════════════════════════════════════╝")
	fmt.Println()
}

func printPhase(num, title string) {
	fmt.Printf("─── Phase %s: %s ───\n\n", num, title)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-3] + "..."
}

func pause() {
	fmt.Print("\n  [Enter to continue] ")
	fmt.Scanln()
	fmt.Println()
}
