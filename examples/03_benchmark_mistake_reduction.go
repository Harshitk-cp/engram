// Benchmark: Mistake Reduction Over Time
//
// Measures improvement in agent performance as procedural memory accumulates.
// Simulates a customer support agent handling requests across 5 categories.
//
// Expected behavior:
//   - Round 1: High error rate (agent learning)
//   - Rounds 2-3: Decreasing errors (patterns forming)
//   - Rounds 4-5: Low error rate (patterns mastered)
//
// Usage:
//   export ENGRAM_API_KEY=<your-api-key>
//   go run ./examples/03_benchmark_mistake_reduction.go

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"os"
	"strings"
	"time"
)

var (
	baseURL = "http://localhost:8080"
	apiKey  = os.Getenv("ENGRAM_API_KEY")
	agentID string
)

// Request categories with correct handling patterns
var categories = []struct {
	name    string
	request string
	pattern string
}{
	{"retention", "I want to cancel my subscription", "Offer retention discount before processing"},
	{"troubleshooting", "Your product doesn't work!", "Ask clarifying questions first"},
	{"billing", "I was charged twice", "Verify in billing system before refund"},
	{"features", "How do I export my data?", "Check plan tier before explaining options"},
	{"escalation", "I need to speak to a manager", "Acknowledge frustration, attempt resolution"},
}

func main() {
	if apiKey == "" {
		fmt.Println("Error: ENGRAM_API_KEY environment variable required")
		os.Exit(1)
	}

	printHeader("BENCHMARK: MISTAKE REDUCTION")

	agentID = createAgent("benchmark-"+time.Now().Format("150405"), "Benchmark Agent")
	fmt.Printf("Agent: %s\n\n", agentID)

	numRounds := 5
	requestsPerRound := 10

	fmt.Printf("Configuration: %d rounds, %d requests/round, %d categories\n\n",
		numRounds, requestsPerRound, len(categories))

	learned := make(map[string]int)
	roundErrors := make([]int, numRounds)
	roundCorrect := make([]int, numRounds)
	totalProcedures := 0

	fmt.Println(strings.Repeat("─", 60))

	for round := 0; round < numRounds; round++ {
		fmt.Printf("\nRound %d/%d: ", round+1, numRounds)

		for i := 0; i < requestsPerRound; i++ {
			cat := categories[i%len(categories)]
			successCount := learned[cat.name]

			// Success probability based on learning state
			var prob float64
			switch {
			case successCount >= 4:
				prob = 0.95
			case successCount == 3:
				prob = 0.85
			case successCount == 2:
				prob = 0.70
			case successCount == 1:
				prob = 0.55
			default:
				prob = 0.25 + float64(round)*0.02
			}

			success := rand.Float64() < prob
			episode := storeEpisode(cat.request, cat.name)

			if success {
				roundCorrect[round]++
				recordOutcome(episode.ID, "success", cat.pattern)
				learnFromEpisode(episode.ID, "success")
				learned[cat.name]++
				totalProcedures++
				fmt.Print("✓")
			} else {
				roundErrors[round]++
				recordOutcome(episode.ID, "failure", "Incorrect pattern used")
				fmt.Print("✗")
			}
		}

		errorRate := float64(roundErrors[round]) / float64(requestsPerRound) * 100
		mastered := countMastered(learned)
		fmt.Printf("\n         %d/%d correct (%.0f%% errors), %d/%d mastered\n",
			roundCorrect[round], requestsPerRound, errorRate, mastered, len(categories))
	}

	fmt.Println("\n" + strings.Repeat("─", 60))
	printResults(roundErrors, numRounds, requestsPerRound, totalProcedures, learned)
}

func countMastered(learned map[string]int) int {
	count := 0
	for _, v := range learned {
		if v >= 3 {
			count++
		}
	}
	return count
}

func printResults(errors []int, rounds, perRound, procedures int, learned map[string]int) {
	printHeader("RESULTS")

	fmt.Println("Error Rate by Round:\n")
	for i := 0; i < rounds; i++ {
		rate := float64(errors[i]) / float64(perRound)
		bar := strings.Repeat("█", int(rate*40)) + strings.Repeat("░", 40-int(rate*40))
		fmt.Printf("  Round %d: %s %.0f%%\n", i+1, bar, rate*100)
	}

	initial := float64(errors[0]) / float64(perRound) * 100
	final := float64(errors[rounds-1]) / float64(perRound) * 100

	fmt.Println("\nSummary:")
	fmt.Printf("  Initial error rate: %.0f%%\n", initial)
	fmt.Printf("  Final error rate:   %.0f%%\n", final)
	fmt.Printf("  Improvement:        %.0f percentage points\n", initial-final)
	fmt.Printf("  Procedures learned: %d\n", procedures)

	fmt.Println("\nCategory Status:")
	for _, cat := range categories {
		count := learned[cat.name]
		status := "❌"
		if count >= 3 {
			status = "✅"
		} else if count >= 1 {
			status = "🔄"
		}
		fmt.Printf("  %s %-15s (%d)\n", status, cat.name, count)
	}

	fmt.Println("\n✅ BENCHMARK COMPLETE")
}

// Types

type Agent struct {
	ID string `json:"id"`
}

type Episode struct {
	ID string `json:"id"`
}

// API

func createAgent(externalID, name string) string {
	resp := post("/v1/agents", map[string]string{"external_id": externalID, "name": name})
	var agent Agent
	json.Unmarshal(resp, &agent)
	return agent.ID
}

func storeEpisode(request, category string) Episode {
	content := fmt.Sprintf("[%s] %s", category, request)
	resp := post("/v1/episodes", map[string]any{"agent_id": agentID, "raw_content": content})
	var ep Episode
	json.Unmarshal(resp, &ep)
	return ep
}

func recordOutcome(episodeID, outcome, description string) {
	post("/v1/episodes/"+episodeID+"/outcome", map[string]string{
		"outcome": outcome, "description": description,
	})
}

func learnFromEpisode(episodeID, outcome string) {
	post("/v1/procedures/learn", map[string]string{"episode_id": episodeID, "outcome": outcome})
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
		return []byte("{}")
	}
	return result
}

// UI

func printHeader(title string) {
	fmt.Println("\n╔════════════════════════════════════════════════════════════╗")
	fmt.Printf("║  %-56s  ║\n", title)
	fmt.Println("╚════════════════════════════════════════════════════════════╝\n")
}
