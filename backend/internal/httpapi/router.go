package httpapi

import (
	"log/slog"
	"net/http"
	"slices"

	"github.com/abhijitmohanty/second-brain/backend/internal/config"
	"github.com/abhijitmohanty/second-brain/backend/internal/knowledge"
	"github.com/abhijitmohanty/second-brain/backend/internal/platform/httputil"
)

func NewRouter(cfg config.Config, service *knowledge.Service, logger *slog.Logger) http.Handler {
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
		result, err := service.Run(r.Context())
		if err != nil {
			logger.Error("run knowledge inbox", "error", err)
			httputil.Error(w, http.StatusInternalServerError, "run knowledge inbox")
			return
		}
		httputil.JSON(w, http.StatusOK, result)
	}

	mux.HandleFunc("GET /api/knowledge-runs/latest", readLatest)
	mux.HandleFunc("POST /api/knowledge-runs/refresh", runInbox)

	return cors(cfg.AllowedOrigins, mux)
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
