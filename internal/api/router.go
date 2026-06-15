package api

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"runtime"
	"strings"
	"sync/atomic"
	"time"

	"github.com/Harshitk-cp/engram/console"
	"github.com/Harshitk-cp/engram/internal/api/handlers"
	mw "github.com/Harshitk-cp/engram/internal/api/middleware"
	"github.com/Harshitk-cp/engram/internal/billing"
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
	Learning      *service.LearningService
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
	graphStore := store.NewGraphStore(db)
	entityStore := store.NewEntityStore(db)
	sessionStore := store.NewSessionStore(db)
	mutationLogStore := store.NewMutationLogStore(db)
	episodeMemUsageStore := store.NewEpisodeMemoryUsageStore(db)
	learningStatsStore := store.NewLearningStatsStore(db)

	// Unit of work for atomic state-change + audit-log writes.
	uow := store.NewUnitOfWork(db, memoryStore, mutationLogStore, contradictionStore)

	// External clients via provider factory
	var embeddingClient domain.EmbeddingClient
	var llmClient domain.LLMClient

	llmProvider := config.LLMProvider()
	llmAPIKey := config.LLMAPIKey()
	embeddingProvider := config.EmbeddingProvider()

	var err error
	llmClient, err = llm.NewClient(llmProvider, llmAPIKey)
	if err != nil {
		logger.Warn("LLM client initialization failed", zap.String("provider", llmProvider), zap.Error(err))
	} else {
		logger.Info("LLM client initialized", zap.String("provider", llmProvider))
	}

	embeddingClient, err = embedding.NewClient(embedding.Config{
		Provider:   embeddingProvider,
		APIKey:     config.EmbeddingAPIKey(),
		BaseURL:    config.EmbeddingBaseURL(),
		Model:      config.EmbeddingModel(),
		Dimensions: config.EmbeddingDim(),
	})
	if err != nil {
		logger.Warn("Embedding client initialization failed", zap.String("provider", embeddingProvider), zap.Error(err))
	} else {
		logger.Info("Embedding client initialized",
			zap.String("provider", embeddingProvider),
			zap.String("model", config.EmbeddingModel()),
			zap.Int("dimension", config.EmbeddingDim()))
		if embeddingProvider != embedding.ProviderMock {
			probeCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			if vec, perr := embeddingClient.Embed(probeCtx, "dimension probe"); perr != nil {
				logger.Warn("embedding provider probe failed (continuing); recall will be empty until it works", zap.Error(perr))
			} else if len(vec) != config.EmbeddingDim() {
				logger.Error("embedding dimension mismatch — writes will fail until resolved",
					zap.Int("model_output_dim", len(vec)),
					zap.Int("configured_dim", config.EmbeddingDim()),
					zap.String("hint", "set EMBEDDING_DIM to the model's dimension (on a fresh DB) or pick a matching model"))
			}
			cancel()
		}
	}

	// Services
	agentSvc := service.NewAgentService(agentStore)
	memorySvc := service.NewMemoryService(memoryStore, agentStore, embeddingClient, llmClient, logger)
	policySvc := service.NewPolicyService(policyStore, memoryStore, agentStore, llmClient, embeddingClient, logger)
	feedbackSvc := service.NewFeedbackService(feedbackStore, memoryStore, agentStore)
	feedbackSvc.SetUnitOfWork(uow)
	tunerSvc := service.NewTunerService(feedbackStore, policyStore, logger)
	expirerSvc := service.NewExpirerService(memoryStore, policyStore, feedbackStore, logger)
	expirerSvc.SetSessionStore(sessionStore)
	confidenceSvc := service.NewConfidenceService(memoryStore, logger)
	episodeSvc := service.NewEpisodeService(episodeStore, agentStore, embeddingClient, llmClient, logger)
	proceduralSvc := service.NewProceduralService(procedureStore, episodeStore, agentStore, embeddingClient, llmClient, logger)
	schemaSvc := service.NewSchemaService(schemaStore, memoryStore, agentStore, embeddingClient, llmClient, logger)
	wmSvc := service.NewWorkingMemoryService(wmStore, assocStore, memoryStore, episodeStore, procedureStore, schemaStore, embeddingClient, logger)
	consolidationSvc := service.NewConsolidationService(memoryStore, episodeStore, procedureStore, schemaStore, assocStore, contradictionStore, embeddingClient, llmClient, logger)
	decaySvc := service.NewDecayService(memoryStore, episodeStore, logger)
	decaySvc.SetMutationLogStore(mutationLogStore)
	decaySvc.SetUnitOfWork(uow)

	// Per-tenant engine tuning (decay rate, floor, competition, confidence deltas).
	tenantSettingsStore := store.NewTenantSettingsStore(db)
	confidenceSvc.SetSettingsStore(tenantSettingsStore)
	decaySvc.SetSettingsStore(tenantSettingsStore)
	consolidationSvc.SetDecayService(decaySvc)
	consolidationSvc.SetGraphStore(graphStore)
	metacognitiveSvc := service.NewMetacognitiveService(memoryStore, episodeStore, procedureStore, schemaStore, contradictionStore, embeddingClient, logger)
	adminSvc := service.NewAdminService(memoryStore, embeddingClient, uow, logger)
	consoleSvc := service.NewConsoleService(memoryStore, contradictionStore, learningStatsStore, logger)

	// Graph services
	hybridRecallSvc := service.NewHybridRecallService(memoryStore, graphStore, entityStore, embeddingClient, llmClient)
	hybridRecallSvc.SetSessionStore(sessionStore)
	graphBuilderSvc := service.NewGraphBuilderService(memoryStore, graphStore, entityStore, embeddingClient, llmClient, logger)

	// Learning services
	learningSvc := service.NewLearningService(memoryStore, episodeStore, logger)
	learningSvc.SetMutationLogStore(mutationLogStore)
	learningSvc.SetEpisodeMemoryUsageStore(episodeMemUsageStore)
	learningSvc.SetLearningStatsStore(learningStatsStore)
	learningSvc.SetUnitOfWork(uow)
	implicitFeedbackSvc := service.NewImplicitFeedbackDetector(llmClient, feedbackStore, memoryStore, logger)
	implicitFeedbackSvc.SetMutationLogStore(mutationLogStore)

	// Wire policy enforcer and contradiction store into memory service
	memorySvc.SetPolicyEnforcer(policySvc)
	memorySvc.SetContradictionStore(contradictionStore)
	memorySvc.SetMutationLogStore(mutationLogStore)
	memorySvc.SetSettingsStore(tenantSettingsStore) // Provenance Firewall policy
	memorySvc.SetUnitOfWork(uow)
	if os.Getenv("DISABLE_GRAPH") != "true" {
		memorySvc.SetGraphBuilder(graphBuilderSvc)
	}

	// Wire memory store into episode service for belief extraction
	episodeSvc.SetMemoryStore(memoryStore)

	// Key store
	apiKeyStore := store.NewAPIKeyStore(db)

	// Billing (managed cloud): per-org plan + usage. Quota enforcement and the
	// Stripe endpoints activate only when STRIPE_SECRET_KEY is configured; an
	// unconfigured (self-hosted/OSS) server runs unmetered.
	billingStore := store.NewBillingStore(db)
	stripeClient := billing.New(config.StripeSecretKey(), config.StripeWebhookSecret(), config.StripePriceIDs())
	billingEnabled := config.BillingEnabled()
	if billingEnabled {
		logger.Info("billing enabled (Stripe configured) — quotas enforced")
	}

	// Control plane (console auth): users, social identities, orgs, sessions.
	userStore := store.NewUserStore(db)
	oauthStore := store.NewOAuthAccountStore(db)
	membershipStore := store.NewMembershipStore(db)
	consoleSessionStore := store.NewConsoleSessionStore(db)
	sessionTTL := time.Duration(config.SessionTTLHours()) * time.Hour
	authSvc := service.NewAuthService(userStore, oauthStore, membershipStore, consoleSessionStore, tenantStore, config.DefaultTenantID(), config.DefaultTenantRole(), sessionTTL, logger)

	// Handlers
	tenantHandler := handlers.NewTenantHandler(tenantStore, apiKeyStore, config.SetupToken())
	setupHandler := handlers.NewSetupHandler(tenantStore, apiKeyStore, config.SetupToken())
	authHandler := handlers.NewAuthHandler(authSvc, sessionTTL)
	agentHandler := handlers.NewAgentHandler(agentSvc)
	memoryHandler := handlers.NewMemoryHandler(memorySvc, hybridRecallSvc, entityStore, sessionStore)
	anchorHandler := handlers.NewAnchorHandler(entityStore, memoryStore)
	sessionHandler := handlers.NewSessionHandler(sessionStore, entityStore, agentStore, 0)
	canonHandler := handlers.NewCanonHandler(memorySvc, memoryStore)
	policyHandler := handlers.NewPolicyHandler(policySvc)
	feedbackHandler := handlers.NewFeedbackHandler(feedbackSvc)
	episodeHandler := handlers.NewEpisodeHandler(episodeSvc)
	procedureHandler := handlers.NewProcedureHandler(proceduralSvc)
	schemaHandler := handlers.NewSchemaHandler(schemaSvc)
	wmHandler := handlers.NewWorkingMemoryHandler(wmSvc)
	cognitiveHandler := handlers.NewCognitiveHandler(decaySvc, consolidationSvc, agentStore)
	cognitiveHandler.SetConfidenceService(confidenceSvc)
	cognitiveHandler.SetCalibrationService(service.NewCalibrationService(mutationLogStore, logger))
	metacognitiveHandler := handlers.NewMetacognitiveHandler(metacognitiveSvc)
	adminHandler := handlers.NewAdminHandler(adminSvc)
	embeddingHandler := handlers.NewEmbeddingHandler()
	consoleHandler := handlers.NewConsoleHandler(consoleSvc)
	auditHandler := handlers.NewAuditHandler(mutationLogStore, config.AuditSigningKey())
	settingsHandler := handlers.NewSettingsHandler(tenantSettingsStore)
	billingHandler := handlers.NewBillingHandler(billingStore, stripeClient, config.AppBaseURL(), logger)
	mindHandler := handlers.NewMindHandler(memoryStore, episodeStore, procedureStore, schemaStore, agentStore)
	tierHandler := handlers.NewTierHandler(memorySvc)
	graphHandler := handlers.NewGraphHandler(hybridRecallSvc, graphBuilderSvc, graphStore, entityStore, agentStore, memoryStore)
	learningHandler := handlers.NewLearningHandler(learningSvc, implicitFeedbackSvc, mutationLogStore, agentStore)
	conversationSvc := service.NewConversationService(memorySvc, llmClient, logger)
	conversationHandler := handlers.NewConversationHandler(conversationSvc, entityStore, sessionStore)

	r := chi.NewRouter()

	// Initialize app with metrics tracking
	app := &App{
		Router:        r,
		Tuner:         tunerSvc,
		Expirer:       expirerSvc,
		Decay:         decaySvc,
		Consolidation: consolidationSvc,
		Learning:      learningSvc,
		startTime:     time.Now(),
	}

	// Metrics collector for middleware
	metricsCollector := mw.NewMetricsCollector(&app.requestCount, &app.errorCount)

	// Global middleware (order matters)
	r.Use(mw.RequestID) // Generate/extract request ID first
	r.Use(mw.CORS(config.CORSAllowedOrigins()))
	if config.TrustProxyHeaders() {
		r.Use(middleware.RealIP)
	}
	r.Use(metricsCollector.Middleware)                                  // Collect metrics
	r.Use(mw.Logging(logger))                                           // Log all requests
	r.Use(middleware.Recoverer)                                         // Recover from panics
	r.Use(mw.RateLimit(config.RateLimitRPS(), config.RateLimitBurst())) // Rate limiting

	// Embedded console SPA served at the site root (e.g. console.engram.to): any
	// unmatched path — static assets and client-side deep links alike — falls
	// through to it, while requests under an API namespace still get a JSON 404
	// rather than the HTML shell.
	if spa, err := console.Handler(); err != nil {
		logger.Warn("console SPA unavailable", zap.Error(err))
	} else {
		r.NotFound(spaFallback(spa))
	}

	// Health (no auth)
	r.Get("/health", healthHandler(db))

	// Metrics (no auth)
	r.Get("/metrics", app.metricsHandler())

	// Sensitive unauthenticated endpoints (tenant/key minting, credential
	// submission) get a much tighter per-IP limiter than the global default so a
	// leaked setup token or online password guessing can't be brute-forced at
	// 100rps. Each call returns its own isolated limiter instance.
	strictLimit := mw.RateLimit(1, 5)

	// Bootstrap (no auth — protected by X-Setup-Token header)
	r.With(strictLimit).Post("/v1/setup", setupHandler.Bootstrap)

	// Legacy tenant creation (deprecated, kept for backward compatibility)
	r.With(strictLimit).Post("/v1/tenants", tenantHandler.Create)

	// Stripe webhook (no session/key auth — verified by signature). Mounted
	// outside the /v1 auth group because Stripe calls it directly.
	r.Post("/v1/billing/webhook", billingHandler.Webhook)

	// Authenticated routes
	// Control-plane auth routes (public; session cookie based).
	r.Route("/auth", func(r chi.Router) {
		r.With(strictLimit).Post("/register", authHandler.Register)
		r.With(strictLimit).Post("/login", authHandler.Login)
		r.Post("/logout", authHandler.Logout)
		r.Get("/me", authHandler.Me)
		r.Post("/switch-tenant", authHandler.SwitchTenant)
		r.Post("/orgs", authHandler.CreateOrg)
		r.Get("/config", authHandler.Config)
		r.Get("/oauth/{provider}/start", authHandler.OAuthStart)
		r.Get("/oauth/{provider}/callback", authHandler.OAuthCallback)
		r.Get("/sso/start", authHandler.SSOStart)
		r.Get("/sso/callback", authHandler.SSOCallback)
	})

	r.Route("/v1", func(r chi.Router) {
		r.Use(mw.SessionOrAPIKey(apiKeyStore, authSvc, config.CORSAllowedOrigins()))
		r.Use(mw.RequireWriteForMutations)

		// Key management (admin scope required)
		r.Route("/keys", func(r chi.Router) {
			r.Use(mw.RequireScope("admin"))
			r.Post("/", setupHandler.CreateKey)
			r.Get("/", setupHandler.ListKeys)
			r.Delete("/{id}", setupHandler.RevokeKey)
		})

		// Billing (managed cloud). Reads + checkout/portal require admin scope,
		// which console owners/admins carry. Webhook is mounted separately (no auth).
		r.Route("/billing", func(r chi.Router) {
			r.Use(mw.RequireScope("admin"))
			r.Get("/", billingHandler.Get)
			r.Post("/checkout", billingHandler.Checkout)
			r.Post("/portal", billingHandler.Portal)
		})

		// Agents
		r.Route("/agents", func(r chi.Router) {
			r.Get("/", agentHandler.List)
			r.With(mw.EnforceAgentQuota(billingStore, billingEnabled)).Post("/", agentHandler.Create)
			r.Route("/{id}", func(r chi.Router) {
				r.Get("/", agentHandler.GetByID)
				r.Delete("/", agentHandler.Delete)
				r.Get("/mind", mindHandler.GetMind)
				r.Get("/policies", policyHandler.Get)
				r.Put("/policies", policyHandler.Upsert)
				r.Get("/tier-stats", tierHandler.GetTierStats)
				r.Get("/hot-memories", tierHandler.GetHotMemories)
				r.Get("/learning/stats", learningHandler.GetStats)
				r.Get("/dashboard", consoleHandler.Dashboard)
				r.Get("/review-queue", consoleHandler.ReviewQueue)
				r.With(mw.RequireScope("admin")).Get("/quarantine", memoryHandler.ListQuarantine)
				r.Get("/memories", consoleHandler.Memories)
				r.Get("/snapshot", consoleHandler.Snapshot)
				r.Get("/contradictions", consoleHandler.Contradictions)
				r.Post("/conversations/ingest", conversationHandler.Ingest)
			})
		})

		// Memories
		r.Route("/memories", func(r chi.Router) {
			r.With(mw.MeterRecall(billingStore, billingEnabled)).Get("/recall", memoryHandler.Recall)
			r.Post("/extract", memoryHandler.Extract)
			r.With(mw.EnforceMemoryQuota(billingStore, billingEnabled)).Post("/", memoryHandler.Create)
			r.Get("/{id}", memoryHandler.GetByID)
			r.Delete("/{id}", memoryHandler.Delete)
			r.With(mw.RequireScope("admin")).Patch("/{id}", adminHandler.UpdateMemory)
			r.Post("/{id}/restore", memoryHandler.Restore)
			r.Get("/{id}/mutations", learningHandler.GetMutationHistory)
		})

		// Provenance Firewall: review-queue decisions (admin-scoped).
		r.Route("/quarantine", func(r chi.Router) {
			r.Use(mw.RequireScope("admin"))
			r.Post("/{id}/release", memoryHandler.ReleaseQuarantine)
			r.Post("/{id}/reject", memoryHandler.RejectQuarantine)
		})

		// Audited admin operations (operator corrections, redaction).
		r.Route("/admin", func(r chi.Router) {
			r.Use(mw.RequireScope("admin"))
			r.Post("/memories/{id}/redact", adminHandler.Redact)
			r.Post("/contradictions/resolve", adminHandler.ResolveContradiction)
			r.Post("/anchors/{id}/shred", adminHandler.CryptoShredAnchor)
			r.Post("/agents/{id}/reembed", adminHandler.Reembed)
		})

		// Active embedding configuration (read-only; deploy-time choice).
		r.Get("/embedding/info", embeddingHandler.Info)

		// Tamper-evident audit trail.
		r.Route("/audit", func(r chi.Router) {
			r.Use(mw.RequireScope("admin"))
			r.Get("/verify", auditHandler.Verify)
			r.Get("/chain", auditHandler.Chain)
			r.Get("/export", auditHandler.Export)
		})

		// Anchors (who/what memories are about)
		r.Route("/anchors", func(r chi.Router) {
			r.Get("/", anchorHandler.List)
			r.Post("/", anchorHandler.Create)
			r.Route("/{id}", func(r chi.Router) {
				r.Get("/", anchorHandler.GetByID)
				r.Delete("/", anchorHandler.Delete)
				r.Get("/memories", anchorHandler.ListMemories)
			})
		})

		// Sessions
		r.Route("/sessions", func(r chi.Router) {
			r.Post("/", sessionHandler.Create)
			r.Route("/{id}", func(r chi.Router) {
				r.Get("/", sessionHandler.GetByID)
				r.Post("/end", sessionHandler.End)
			})
		})

		// Canon (tenant-shared, authoritative knowledge). Reads open to any
		// authenticated caller; writes are admin-scoped.
		r.Route("/canon", func(r chi.Router) {
			r.Get("/", canonHandler.List)
			r.Group(func(r chi.Router) {
				r.Use(mw.RequireScope("admin"))
				r.Post("/", canonHandler.Create)
				r.Delete("/{id}", canonHandler.Delete)
			})
		})

		// Learning
		r.Route("/learning", func(r chi.Router) {
			r.Post("/outcome", learningHandler.RecordOutcome)
			r.Post("/detect-feedback", learningHandler.DetectImplicitFeedback)
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

		// Graph (entity and relationship queries)
		r.Route("/graph", func(r chi.Router) {
			r.Get("/entities", graphHandler.ListEntities)
			r.Get("/relationships", graphHandler.GetRelationships)
			r.Post("/traverse", graphHandler.Traverse)
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
			// Confidence lifecycle operations
			r.Get("/confidence/stats", cognitiveHandler.GetConfidenceStats)
			r.Post("/confidence/reinforce", cognitiveHandler.ReinforceMemory)
			r.Post("/confidence/penalize", cognitiveHandler.PenalizeMemory)
			r.Get("/calibration", cognitiveHandler.GetCalibration)
		})

		// Engine settings (per-tenant tuning). Read is open within the tenant;
		// writes require admin scope.
		r.Route("/settings", func(r chi.Router) {
			r.Get("/", settingsHandler.Get)
			r.With(mw.RequireScope("admin")).Put("/", settingsHandler.Update)
		})
	})

	return app
}

// NewRouter returns just the chi.Mux for backward compatibility.
func NewRouter(db *pgxpool.Pool, logger *zap.Logger) *chi.Mux {
	return NewApp(db, logger).Router
}

// spaFallback serves the console SPA for any unmatched path, except requests
// under an API namespace, which get a JSON 404 so clients don't receive the HTML
// shell in place of an error.
func spaFallback(spa http.Handler) http.HandlerFunc {
	apiPrefixes := []string{"/v1", "/auth", "/health", "/metrics"}
	return func(w http.ResponseWriter, r *http.Request) {
		for _, p := range apiPrefixes {
			if r.URL.Path == p || strings.HasPrefix(r.URL.Path, p+"/") {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusNotFound)
				_ = json.NewEncoder(w).Encode(map[string]string{"error": "not found"})
				return
			}
		}
		spa.ServeHTTP(w, r)
	}
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
	_ domain.TenantStore             = (*store.TenantStore)(nil)
	_ domain.BillingStore            = (*store.BillingStore)(nil)
	_ domain.AgentStore              = (*store.AgentStore)(nil)
	_ domain.MemoryStore             = (*store.MemoryStore)(nil)
	_ domain.PolicyStore             = (*store.PolicyStore)(nil)
	_ domain.FeedbackStore           = (*store.FeedbackStore)(nil)
	_ domain.ContradictionStore      = (*store.ContradictionStore)(nil)
	_ domain.EpisodeStore            = (*store.EpisodeStore)(nil)
	_ domain.ProcedureStore          = (*store.ProcedureStore)(nil)
	_ domain.SchemaStore             = (*store.SchemaStore)(nil)
	_ domain.WorkingMemoryStore      = (*store.WorkingMemoryStore)(nil)
	_ domain.MemoryAssociationStore  = (*store.MemoryAssociationStore)(nil)
	_ domain.MutationLogStore        = (*store.MutationLogStore)(nil)
	_ domain.EpisodeMemoryUsageStore = (*store.EpisodeMemoryUsageStore)(nil)
	_ domain.LearningStatsStore      = (*store.LearningStatsStore)(nil)
	_ domain.EmbeddingClient         = (*embedding.CompatibleClient)(nil)
	_ domain.EmbeddingClient         = (*embedding.MockClient)(nil)
	_ domain.LLMClient               = (*llm.OpenAIClient)(nil)
	_ domain.LLMClient               = (*llm.AnthropicClient)(nil)
	_ domain.LLMClient               = (*llm.GeminiClient)(nil)
	_ domain.LLMClient               = (*llm.CerebrasClient)(nil)
	_ domain.LLMClient               = (*llm.MockClient)(nil)
)
