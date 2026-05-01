package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"

	"github.com/kariz/kariz/internal"
	"github.com/kariz/kariz/internal/api"
	"github.com/kariz/kariz/internal/artifact"
	"github.com/kariz/kariz/internal/auth"
	"github.com/kariz/kariz/internal/catalog"
	"github.com/kariz/kariz/internal/config"
	"github.com/kariz/kariz/internal/docker"
	"github.com/kariz/kariz/internal/envvar"
	"github.com/kariz/kariz/internal/executor"
	"github.com/kariz/kariz/internal/notification"
	"github.com/kariz/kariz/internal/scheduler"
	"github.com/kariz/kariz/internal/stream"
	"github.com/kariz/kariz/internal/validator"
	"github.com/kariz/kariz/migrations"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})))

	// Simple subcommand routing: "createsuperuser" or default (serve).
	if len(os.Args) > 1 && os.Args[1] == "createsuperuser" {
		runCreateSuperuser()
		return
	}

	runServe()
}

// ---------------------------------------------------------------------------
// serve — the main application server
// ---------------------------------------------------------------------------

func runServe() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("failed to load configuration", "error", err)
		os.Exit(1)
	}

	slog.Info("starting KARIZ application",
		"port", cfg.AppPort,
		"database_url", maskDatabaseURL(cfg.DatabaseURL),
		"docker_socket", cfg.DockerSocketPath,
		"session_ttl", cfg.SessionTTL.String(),
		"smtp_configured", cfg.SMTPConfigured(),
	)

	// Migrations.
	slog.Info("running database migrations")
	if err := internal.RunMigrations(migrations.FS, ".", cfg.DatabaseURL); err != nil {
		slog.Error("failed to run database migrations", "error", err)
		os.Exit(1)
	}
	slog.Info("database migrations completed")

	// Database.
	db, err := internal.ConnectDB(cfg)
	if err != nil {
		slog.Error("failed to connect to database", "error", err)
		os.Exit(1)
	}
	defer func() { _ = db.Close() }()

	// Docker manager.
	dockerMgr, err := docker.NewDockerManager(cfg.DockerSocketPath)
	if err != nil {
		slog.Warn("Docker manager unavailable — execution features disabled", "error", err)
	}

	// Application context for background goroutines.
	appCtx, appCancel := context.WithCancel(context.Background())
	defer appCancel()

	// --- Repositories ---
	userRepo := auth.NewUserRepository(db)
	sessionRepo := auth.NewSessionRepository(db)
	commandRepo := catalog.NewCommandRepository(db)
	executionRepo := executor.NewExecutionRepository(db)
	notificationRepo := notification.NewNotificationRepository(db)
	artifactRepo := artifact.NewExecutionArtifactRepository(db)
	scheduleRepo := scheduler.NewScheduleRepository(db)

	// --- Services ---
	authProvider := auth.NewLocalAuthProvider(userRepo)
	sessionStore := auth.NewSessionStore(sessionRepo, cfg.SessionTTL)
	catalogSvc := catalog.NewCatalogService(commandRepo)
	paramVal := validator.NewParameterValidator()
	streamMgr := stream.NewStreamManager()
	notifySvc := notification.NewNotificationService(notificationRepo, cfg)
	localArtifactStore := artifact.NewLocalArtifactStore(cfg.ArtifactStorePath)
	artifactCopier := artifact.NewArtifactCopier(dockerMgr, localArtifactStore, artifactRepo)
	envResolver := envvar.NewEnvVarResolver(dockerMgr)

	executorSvc := executor.NewExecutorService(
		executionRepo, dockerMgr, paramVal, streamMgr, notifySvc, envResolver, artifactCopier,
	)

	schedulerSvc := scheduler.NewSchedulerService(
		scheduleRepo, executorSvc, notifySvc, catalogSvc,
	)

	// --- Handlers ---
	authHandler := auth.NewAuthHandler(authProvider, sessionStore, cfg.SessionTTL)
	catalogHandler := catalog.NewCatalogHandler(catalogSvc)
	executorHandler := executor.NewExecutorHandler(executorSvc, catalogSvc, executionRepo)
	sseHandler := stream.NewSSEHandler(streamMgr)
	notificationHandler := notification.NewNotificationHandler(notifySvc)
	artifactHandler := artifact.NewArtifactHandler(artifactRepo, localArtifactStore)
	adminHandler := api.NewAdminHandler(userRepo, sessionRepo)
	healthHandler := api.NewHealthHandler(db, dockerMgr)
	schedulerHandler := scheduler.NewSchedulerHandler(schedulerSvc, catalogSvc)

	// --- Router ---
	router := gin.Default()
	apiGroup := router.Group("/api")

	authMiddleware := auth.AuthMiddleware(sessionStore)

	// Register all routes.
	authHandler.RegisterRoutes(apiGroup)
	catalogHandler.RegisterRoutes(apiGroup, authMiddleware)
	executorHandler.RegisterRoutes(apiGroup, authMiddleware)
	sseHandler.RegisterRoutes(apiGroup, authMiddleware)
	notificationHandler.RegisterRoutes(apiGroup, authMiddleware)
	artifactHandler.RegisterRoutes(apiGroup, authMiddleware)
	adminHandler.RegisterRoutes(apiGroup, authMiddleware)
	healthHandler.RegisterRoutes(apiGroup)
	schedulerHandler.RegisterRoutes(apiGroup, authMiddleware)

	// --- SPA static files ---
	serveSPA(router)

	// --- Background tasks ---
	auth.StartSessionCleanup(appCtx, sessionStore, 15*time.Minute)

	if err := schedulerSvc.Start(appCtx); err != nil {
		slog.Error("failed to start scheduler", "error", err)
	}

	// --- HTTP server ---
	srv := &http.Server{
		Addr:    ":" + cfg.AppPort,
		Handler: router,
	}

	go func() {
		slog.Info("HTTP server listening", "addr", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("HTTP server error", "error", err)
			os.Exit(1)
		}
	}()

	// --- Graceful shutdown ---
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGTERM, syscall.SIGINT)
	sig := <-quit
	slog.Info("received shutdown signal", "signal", sig)

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	slog.Info("shutting down HTTP server")
	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.Error("HTTP server shutdown error", "error", err)
	}

	slog.Info("stopping background services")
	appCancel()
	_ = schedulerSvc.Stop()

	slog.Info("graceful shutdown complete")
}

// serveSPA configures the Gin router to serve the React SPA.
func serveSPA(router *gin.Engine) {
	webDistPath := "/app/web/dist"
	if _, err := os.Stat(webDistPath); os.IsNotExist(err) {
		webDistPath = "./web/dist"
	}

	if info, err := os.Stat(webDistPath); err == nil && info.IsDir() {
		router.Static("/assets", webDistPath+"/assets")
		router.NoRoute(func(c *gin.Context) {
			if strings.HasPrefix(c.Request.URL.Path, "/api/") {
				c.JSON(http.StatusNotFound, gin.H{"code": "not_found", "message": "endpoint not found"})
				return
			}
			c.File(webDistPath + "/index.html")
		})
		slog.Info("serving React SPA", "path", webDistPath)
	} else {
		slog.Warn("React SPA dist not found", "path", webDistPath)
		router.NoRoute(func(c *gin.Context) {
			if strings.HasPrefix(c.Request.URL.Path, "/api/") {
				c.JSON(http.StatusNotFound, gin.H{"code": "not_found", "message": "endpoint not found"})
				return
			}
			c.String(http.StatusOK, "KARIZ backend running. Frontend not built — run 'cd web && pnpm build'.")
		})
	}
}

// ---------------------------------------------------------------------------
// createsuperuser — CLI command to create an admin user
// ---------------------------------------------------------------------------

func runCreateSuperuser() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}

	// Run migrations first.
	if err := internal.RunMigrations(migrations.FS, ".", cfg.DatabaseURL); err != nil {
		fmt.Fprintf(os.Stderr, "Error running migrations: %v\n", err)
		os.Exit(1)
	}

	db, err := internal.ConnectDB(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error connecting to database: %v\n", err)
		os.Exit(1)
	}
	defer func() { _ = db.Close() }()

	// Parse flags: --username, --password, --email (or positional args).
	username := "admin"
	password := "admin123"
	email := "admin@kariz.local"

	// Simple arg parsing: createsuperuser [username] [password] [email]
	args := os.Args[2:]
	if len(args) >= 1 {
		username = args[0]
	}
	if len(args) >= 2 {
		password = args[1]
	}
	if len(args) >= 3 {
		email = args[2]
	}

	if err := createSuperuser(db, username, password, email); err != nil {
		fmt.Fprintf(os.Stderr, "Error creating superuser: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Superuser created successfully:\n  username: %s\n  email:    %s\n  roles:    [admin]\n", username, email)
}

func createSuperuser(db *sqlx.DB, username, password, email string) error {
	// Hash the password.
	hash, err := auth.HashPassword(password)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}

	// Check if user already exists.
	var count int
	err = db.Get(&count, "SELECT COUNT(*) FROM users WHERE username = $1", username)
	if err != nil {
		return fmt.Errorf("check existing user: %w", err)
	}
	if count > 0 {
		return fmt.Errorf("user %q already exists", username)
	}

	// Insert user.
	tx, err := db.Beginx()
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var userID string
	err = tx.QueryRow(`
		INSERT INTO users (id, username, password_hash, email, is_active, created_at, updated_at)
		VALUES (gen_random_uuid(), $1, $2, $3, true, NOW(), NOW())
		RETURNING id`, username, hash, email).Scan(&userID)
	if err != nil {
		return fmt.Errorf("insert user: %w", err)
	}

	_, err = tx.Exec(`
		INSERT INTO user_roles (user_id, role, assigned_at)
		VALUES ($1, 'admin', NOW())`, userID)
	if err != nil {
		return fmt.Errorf("insert admin role: %w", err)
	}

	return tx.Commit()
}

// maskDatabaseURL masks the password in a PostgreSQL connection string for safe logging.
func maskDatabaseURL(url string) string {
	schemeEnd := strings.Index(url, "://")
	if schemeEnd == -1 {
		return "***"
	}
	atIdx := strings.Index(url[schemeEnd+3:], "@")
	if atIdx == -1 {
		return url
	}
	credsPart := url[schemeEnd+3 : schemeEnd+3+atIdx]
	colonIdx := strings.Index(credsPart, ":")
	if colonIdx == -1 {
		return url
	}
	user := credsPart[:colonIdx]
	return url[:schemeEnd+3] + user + ":***" + url[schemeEnd+3+atIdx:]
}
