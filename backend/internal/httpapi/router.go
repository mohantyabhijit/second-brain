package httpapi

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"slices"
	"time"

	"github.com/abhijitmohanty/second-brain/backend/internal/config"
	"github.com/abhijitmohanty/second-brain/backend/internal/knowledge"
	"github.com/abhijitmohanty/second-brain/backend/internal/platform/httputil"
)

func NewRouter(cfg config.Config, service *knowledge.Service, logger *slog.Logger) http.Handler {
	service.SetLogger(logger)
	mux := http.NewServeMux()

	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		httputil.JSON(w, http.StatusOK, map[string]string{"status": "ok", "env": cfg.Env})
	})

	readLatest := func(w http.ResponseWriter, r *http.Request) {
		latest, err := service.ReadLatest(r.Context())
		if err != nil {
			logger.Error("read latest knowledge run", "error", err)
			httputil.Error(w, http.StatusInternalServerError, "read latest knowledge run")
			return
		}
		httputil.JSON(w, http.StatusOK, map[string]any{"latest": latest})
	}

	runInbox := func(w http.ResponseWriter, r *http.Request) {
		status := service.StartRefresh()
		httputil.JSON(w, http.StatusAccepted, status)
	}

	readRefreshStatus := func(w http.ResponseWriter, r *http.Request) {
		httputil.JSON(w, http.StatusOK, service.RefreshStatus())
	}

	saveFeedback := func(w http.ResponseWriter, r *http.Request) {
		var event knowledge.FeedbackEvent
		if err := json.NewDecoder(r.Body).Decode(&event); err != nil {
			httputil.Error(w, http.StatusBadRequest, "invalid feedback payload")
			return
		}
		if err := service.SaveFeedback(r.Context(), event); err != nil {
			httputil.Error(w, http.StatusBadRequest, err.Error())
			return
		}
		httputil.JSON(w, http.StatusCreated, map[string]string{"status": "saved"})
	}

	generateDigest := func(w http.ResponseWriter, r *http.Request) {
		digest, err := service.GenerateDigest(r.Context())
		if err != nil {
			logger.Error("generate digest", "error", err)
			httputil.Error(w, http.StatusInternalServerError, "generate digest")
			return
		}
		httputil.JSON(w, http.StatusOK, digest)
	}

	mux.HandleFunc("GET /api/knowledge-runs/latest", readLatest)
	mux.HandleFunc("GET /api/knowledge-runs/refresh", readRefreshStatus)
	mux.HandleFunc("POST /api/knowledge-runs/refresh", runInbox)
	mux.HandleFunc("POST /api/feedback", saveFeedback)
	mux.HandleFunc("POST /api/digests/generate", generateDigest)

	return requestLogger(logger, cors(cfg.AllowedOrigins, mux))
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

func requestLogger(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		recorder := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(recorder, r)
		logger.Info(
			"http request completed",
			"method", r.Method,
			"path", r.URL.Path,
			"status", recorder.status,
			"duration_ms", time.Since(start).Milliseconds(),
		)
	})
}

func cors(allowedOrigins []string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" && slices.Contains(allowedOrigins, origin) {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
