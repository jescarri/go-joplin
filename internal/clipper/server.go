package clipper

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	chiMiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/jescarri/go-joplin/internal/store"
	"github.com/jescarri/go-joplin/internal/telemetry"
)

// SyncTrigger is the interface used to trigger a sync after mutations.
type SyncTrigger interface {
	TriggerSync()
}

// Server is the Joplin Clipper REST API server.
type Server struct {
	db        *store.DB
	apiToken  string
	apiKey    string
	router    chi.Router
	syncer    SyncTrigger
	mcpHandler http.Handler // optional MCP SSE handler, mounted at /mcp
}

// NewServer creates a new clipper server.
// syncer may be nil if no sync trigger is desired.
// mcpHandler may be nil; if set, it is mounted at /mcp (Bearer auth applied).
func NewServer(db *store.DB, apiToken string, apiKey string, syncer SyncTrigger, mcpHandler http.Handler) *Server {
	s := &Server{
		db:         db,
		apiToken:   apiToken,
		apiKey:     apiKey,
		syncer:     syncer,
		mcpHandler: mcpHandler,
	}
	s.buildRouter()
	return s
}

// Router returns the HTTP handler.
func (s *Server) Router() http.Handler {
	return s.router
}

// triggerSync signals the sync engine to run if configured.
func (s *Server) triggerSync() {
	if s.syncer != nil {
		s.syncer.TriggerSync()
	}
}

func (s *Server) buildRouter() {
	r := chi.NewRouter()

	// Middleware: recovery, real IP, CORS first
	r.Use(chiMiddleware.Recoverer)
	r.Use(chiMiddleware.RealIP)
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"*"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type"},
		ExposedHeaders:   []string{"Link"},
		AllowCredentials: false,
		MaxAge:           300,
	}))
	// Metrics (request duration histogram); skips /health
	r.Use(telemetry.Middleware)
	// Request logging; skips /health
	r.Use(telemetry.LoggingSkipHealth)
	// Bearer auth (skips /health so probes need no token)
	r.Use(s.bearerAuth)

	// Routes (all middlewares above)
	r.Get("/health", s.handleHealth)
	r.Get("/ping", s.handlePing)
	r.Post("/auth", s.handleAuth)
	r.Get("/auth/check", s.handleAuthCheck)

	// Notes
	r.Get("/notes", s.handleListNotes)
	r.Post("/notes", s.handleCreateNote)
	r.Get("/notes/{id}", s.handleGetNote)
	r.Put("/notes/{id}", s.handleUpdateNote)
	r.Delete("/notes/{id}", s.handleDeleteNote)
	r.Get("/notes/{id}/tags", s.handleGetNoteTags)
	r.Get("/notes/{id}/resources", s.handleGetNoteResources)

	// Folders
	r.Get("/folders", s.handleListFolders)
	r.Post("/folders", s.handleCreateFolder)
	r.Get("/folders/{id}", s.handleGetFolder)
	r.Put("/folders/{id}", s.handleUpdateFolder)
	r.Delete("/folders/{id}", s.handleDeleteFolder)
	r.Get("/folders/{id}/notes", s.handleGetFolderNotes)

	// Tags
	r.Get("/tags", s.handleListTags)
	r.Post("/tags", s.handleCreateTag)
	r.Get("/tags/{id}", s.handleGetTag)
	r.Put("/tags/{id}", s.handleUpdateTag)
	r.Delete("/tags/{id}", s.handleDeleteTag)
	r.Get("/tags/{id}/notes", s.handleGetTagNotes)
	r.Post("/tags/{id}/notes", s.handleAddTagNote)
	r.Delete("/tags/{id}/notes/{noteId}", s.handleRemoveTagNote)

	// Resources
	r.Get("/resources", s.handleListResources)
	r.Post("/resources", s.handleCreateResource)
	r.Get("/resources/{id}", s.handleGetResource)
	r.Put("/resources/{id}", s.handleUpdateResource)
	r.Delete("/resources/{id}", s.handleDeleteResource)
	r.Get("/resources/{id}/file", s.handleGetResourceFile)

	// Search
	r.Get("/search", s.handleSearch)

	// Events
	r.Get("/events", s.handleListEvents)
	r.Get("/events/{id}", s.handleGetEvent)

	// MCP over SSE (optional; Bearer auth applied). GET /mcp = new session, POST /mcp?sessionid=... = message.
	if s.mcpHandler != nil {
		r.Handle("/mcp", s.mcpHandler)
	}

	s.router = r
}
