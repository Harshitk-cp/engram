package api

import (
	"encoding/json"
	"net/http"
	"runtime"
	"sync/atomic"
	"time"

	"github.com/Harshitk-cp/engram/internal/api/handlers"
	mw "github.com/Harshitk-cp/engram/internal/api/middleware"
	"github.com/Harshitk-cp/engram/internal/config"
	"github.com/Harshitk-cp/engram/internal/domain"
	"github.com/Harshitk-cp/engram/internal/embedding"
	"github.com/Harshitk-cp/engram/internal/llm"
	"github.com/Harshitk-cp/engram/internal/service"
	"github.com/Harshitk-cp/engram/internal/store"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"
)

// App holds the router and background services for lifecycle management.
type App struct {
	Router        *chi.Mux
	Tuner         *service.TunerService
	Expirer       *service.ExpirerService
	Decay         *service.DecayService
	Consolidation *service.ConsolidationService
	startTime     time.Time
	requestCount  atomic.Int64
	errorCount    atomic.Int64
}

func NewApp(db *pgxpool.Pool, logger *zap.Logger) *App {
	// Stores
	tenantStore := store.NewTenantStore(db)
	agentStore := store.NewAgentStore(db)
	memoryStore := store.NewMemoryStore(db)
	policyStore := store.NewPolicyStore(db)
	feedbackStore := store.NewFeedbackStore(db)
	contradictionStore := store.NewContradictionStore(db)
	episodeStore := store.NewEpisodeStore(db)
	procedureStore := store.NewProcedureStore(db)
	schemaStore := store.NewSchemaStore(db)
	wmStore := store.NewWorkingMemoryStore(db)
	assocStore := store.NewMemoryAssociationStore(db)

	// External clients via provider factory
	var embeddingClient domain.EmbeddingClient
	var llmClient domain.LLMClient

	llmProvider := config.LLMProvider()
	llmAPIKey := config.LLMAPIKey()
	embeddingProvider := config.EmbeddingProvider()
	embeddingAPIKey := config.EmbeddingAPIKey()

	var err error
	llmClient, err = llm.NewClient(llmProvider, llmAPIKey)
	if err != nil {
		logger.Warn("LLM client initialization failed", zap.String("provider", llmProvider), zap.Error(err))
	} else {
		logger.Info("LLM client initialized", zap.String("provider", llmProvider))
	}

	embeddingClient, err = embedding.NewClient(embeddingProvider, embeddingAPIKey)
	if err != nil {
		logger.Warn("Embedding client initialization failed", zap.String("provider", embeddingProvider), zap.Error(err))
	} else {
		logger.Info("Embedding client initialized", zap.String("provider", embeddingProvider))
	}

	// Services
	agentSvc := service.NewAgentService(agentStore)
	memorySvc := service.NewMemoryService(memoryStore, agentStore, embeddingClient, llmClient, logger)
	policySvc := service.NewPolicyService(policyStore, memoryStore, agentStore, llmClient, embeddingClient, logger)
	feedbackSvc := service.NewFeedbackService(feedbackStore, memoryStore, agentStore)
	tunerSvc := service.NewTunerService(feedbackStore, policyStore, logger)
	expirerSvc := service.NewExpirerService(memoryStore, policyStore, feedbackStore, logger)
	decaySvc := service.NewDecayService(memoryStore, episodeStore, logger)
	episodeSvc := service.NewEpisodeService(episodeStore, agentStore, embeddingClient, llmClient, logger)
	proceduralSvc := service.NewProceduralService(procedureStore, episodeStore, agentStore, embeddingClient, llmClient, logger)
	schemaSvc := service.NewSchemaService(schemaStore, memoryStore, agentStore, embeddingClient, llmClient, logger)
	wmSvc := service.NewWorkingMemoryService(wmStore, assocStore, memoryStore, episodeStore, procedureStore, schemaStore, embeddingClient, logger)
	consolidationSvc := service.NewConsolidationService(memoryStore, episodeStore, procedureStore, schemaStore, assocStore, contradictionStore, embeddingClient, llmClient, logger)
	metacognitiveSvc := service.NewMetacognitiveService(memoryStore, episodeStore, procedureStore, schemaStore, contradictionStore, embeddingClient, logger)

	// Wire policy enforcer and contradiction store into memory service
	memorySvc.SetPolicyEnforcer(policySvc)
	memorySvc.SetContradictionStore(contradictionStore)

	// Wire memory store into episode service for belief extraction
	episodeSvc.SetMemoryStore(memoryStore)

	// Handlers
	tenantHandler := handlers.NewTenantHandler(tenantStore)
	agentHandler := handlers.NewAgentHandler(agentSvc)
	memoryHandler := handlers.NewMemoryHandler(memorySvc)
	policyHandler := handlers.NewPolicyHandler(policySvc)
	feedbackHandler := handlers.NewFeedbackHandler(feedbackSvc)
	episodeHandler := handlers.NewEpisodeHandler(episodeSvc)
	procedureHandler := handlers.NewProcedureHandler(proceduralSvc)
	schemaHandler := handlers.NewSchemaHandler(schemaSvc)
	wmHandler := handlers.NewWorkingMemoryHandler(wmSvc)
	cognitiveHandler := handlers.NewCognitiveHandler(decaySvc, consolidationSvc)
	metacognitiveHandler := handlers.NewMetacognitiveHandler(metacognitiveSvc)
	mindHandler := handlers.NewMindHandler(memoryStore, episodeStore, procedureStore, schemaStore)

	r := chi.NewRouter()

	// Initialize app with metrics tracking
	app := &App{
		Router:        r,
		Tuner:         tunerSvc,
		Expirer:       expirerSvc,
		Decay:         decaySvc,
		Consolidation: consolidationSvc,
		startTime:     time.Now(),
	}

	// Metrics collector for middleware
	metricsCollector := mw.NewMetricsCollector(&app.requestCount, &app.errorCount)

	// Global middleware (order matters)
	r.Use(mw.RequestID)                                                 // Generate/extract request ID first
	r.Use(middleware.RealIP)                                            // Extract real IP
	r.Use(metricsCollector.Middleware)                                  // Collect metrics
	r.Use(mw.Logging(logger))                                           // Log all requests
	r.Use(middleware.Recoverer)                                         // Recover from panics
	r.Use(mw.RateLimit(config.RateLimitRPS(), config.RateLimitBurst())) // Rate limiting

	// Health (no auth)
	r.Get("/health", healthHandler(db))

	// Metrics (no auth)
	r.Get("/metrics", app.metricsHandler())

	// Tenant creation (no auth — bootstrap endpoint)
	r.Post("/v1/tenants", tenantHandler.Create)

	// Authenticated routes
	r.Route("/v1", func(r chi.Router) {
		r.Use(mw.APIKeyAuth(tenantStore))

		// Agents
		r.Route("/agents", func(r chi.Router) {
			r.Post("/", agentHandler.Create)
			r.Route("/{id}", func(r chi.Router) {
				r.Get("/", agentHandler.GetByID)
				r.Get("/mind", mindHandler.GetMind)
				r.Get("/policies", policyHandler.Get)
				r.Put("/policies", policyHandler.Upsert)
			})
		})

		// Memories
		r.Route("/memories", func(r chi.Router) {
			r.Get("/recall", memoryHandler.Recall)
			r.Post("/extract", memoryHandler.Extract)
			r.Post("/", memoryHandler.Create)
			r.Get("/{id}", memoryHandler.GetByID)
			r.Delete("/{id}", memoryHandler.Delete)
		})

		// Episodes (episodic memory)
		r.Route("/episodes", func(r chi.Router) {
			r.Get("/recall", episodeHandler.Recall)
			r.Post("/", episodeHandler.Create)
			r.Route("/{id}", func(r chi.Router) {
				r.Get("/", episodeHandler.GetByID)
				r.Post("/outcome", episodeHandler.RecordOutcome)
				r.Get("/associations", episodeHandler.GetAssociations)
			})
		})

		// Feedback
		r.Post("/feedback", feedbackHandler.Create)

		// Procedures (procedural memory)
		r.Route("/procedures", func(r chi.Router) {
			r.Post("/match", procedureHandler.Match)
			r.Post("/learn", procedureHandler.LearnFromEpisode)
			r.Route("/{id}", func(r chi.Router) {
				r.Get("/", procedureHandler.GetByID)
				r.Post("/outcome", procedureHandler.RecordOutcome)
			})
		})

		// Schemas (mental models)
		r.Route("/schemas", func(r chi.Router) {
			r.Get("/", schemaHandler.List)
			r.Post("/match", schemaHandler.Match)
			r.Post("/detect", schemaHandler.Detect)
			r.Route("/{id}", func(r chi.Router) {
				r.Get("/", schemaHandler.GetByID)
				r.Delete("/", schemaHandler.Delete)
				r.Post("/contradict", schemaHandler.Contradict)
				r.Post("/validate", schemaHandler.Validate)
			})
		})

		// Cognitive operations (working memory, decay, consolidation, metacognition, etc.)
		r.Route("/cognitive", func(r chi.Router) {
			r.Post("/decay", cognitiveHandler.TriggerDecay)
			r.Post("/consolidate", cognitiveHandler.TriggerConsolidation)
			r.Get("/health", cognitiveHandler.GetMemoryHealth)
			r.Post("/activate", wmHandler.Activate)
			r.Get("/session", wmHandler.GetSession)
			r.Put("/goal", wmHandler.UpdateGoal)
			r.Delete("/session", wmHandler.ClearSession)
			// Metacognitive operations
			r.Post("/reflect", metacognitiveHandler.Reflect)
			r.Get("/confidence", metacognitiveHandler.AssessConfidence)
			r.Get("/uncertainty", metacognitiveHandler.DetectUncertainty)
		})
	})

	return app
}

// NewRouter returns just the chi.Mux for backward compatibility.
func NewRouter(db *pgxpool.Pool, logger *zap.Logger) *chi.Mux {
	return NewApp(db, logger).Router
}

func healthHandler(db *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := db.Ping(r.Context()); err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusServiceUnavailable)
			_ = json.NewEncoder(w).Encode(map[string]string{"status": "error", "error": err.Error()})
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}
}

func (app *App) metricsHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var memStats runtime.MemStats
		runtime.ReadMemStats(&memStats)

		uptime := time.Since(app.startTime)

		response := map[string]any{
			"uptime_seconds": uptime.Seconds(),
			"uptime_human":   uptime.Round(time.Second).String(),
			"request_count":  app.requestCount.Load(),
			"error_count":    app.errorCount.Load(),
			"goroutines":     runtime.NumGoroutine(),
			"memory": map[string]any{
				"alloc_mb":       float64(memStats.Alloc) / 1024 / 1024,
				"total_alloc_mb": float64(memStats.TotalAlloc) / 1024 / 1024,
				"sys_mb":         float64(memStats.Sys) / 1024 / 1024,
				"num_gc":         memStats.NumGC,
			},
			"go_version": runtime.Version(),
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(response)
	}
}

// Ensure stores and clients satisfy interfaces at compile time.
var (
	_ domain.TenantStore        = (*store.TenantStore)(nil)
	_ domain.AgentStore         = (*store.AgentStore)(nil)
	_ domain.MemoryStore        = (*store.MemoryStore)(nil)
	_ domain.PolicyStore        = (*store.PolicyStore)(nil)
	_ domain.FeedbackStore      = (*store.FeedbackStore)(nil)
	_ domain.ContradictionStore = (*store.ContradictionStore)(nil)
	_ domain.EpisodeStore       = (*store.EpisodeStore)(nil)
	_ domain.ProcedureStore     = (*store.ProcedureStore)(nil)
	_ domain.SchemaStore             = (*store.SchemaStore)(nil)
	_ domain.WorkingMemoryStore      = (*store.WorkingMemoryStore)(nil)
	_ domain.MemoryAssociationStore  = (*store.MemoryAssociationStore)(nil)
	_ domain.EmbeddingClient         = (*embedding.OpenAIClient)(nil)
	_ domain.EmbeddingClient    = (*embedding.MockClient)(nil)
	_ domain.LLMClient          = (*llm.OpenAIClient)(nil)
	_ domain.LLMClient          = (*llm.AnthropicClient)(nil)
	_ domain.LLMClient          = (*llm.GeminiClient)(nil)
	_ domain.LLMClient          = (*llm.CerebrasClient)(nil)
	_ domain.LLMClient          = (*llm.MockClient)(nil)
)
