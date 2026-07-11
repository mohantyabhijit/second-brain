package httpapi

import (
	"crypto/subtle"
	"net/http"
	"net/http/pprof"
	"runtime"
	"strings"
	"time"

	"github.com/abhijitmohanty/second-brain/backend/internal/config"
	"github.com/abhijitmohanty/second-brain/backend/internal/platform/httputil"
	"github.com/abhijitmohanty/second-brain/backend/internal/platform/logging"
)

type memoryStatsResponse struct {
	Timestamp            string `json:"timestamp"`
	GCTriggered          bool   `json:"gcTriggered"`
	Goroutines           int    `json:"goroutines"`
	AllocBytes           uint64 `json:"allocBytes"`
	TotalAllocBytes      uint64 `json:"totalAllocBytes"`
	SysBytes             uint64 `json:"sysBytes"`
	HeapAllocBytes       uint64 `json:"heapAllocBytes"`
	HeapInuseBytes       uint64 `json:"heapInuseBytes"`
	HeapIdleBytes        uint64 `json:"heapIdleBytes"`
	HeapReleasedBytes    uint64 `json:"heapReleasedBytes"`
	StackInuseBytes      uint64 `json:"stackInuseBytes"`
	NextGCBytes          uint64 `json:"nextGCBytes"`
	LastGCTimestamp      string `json:"lastGCTimestamp,omitempty"`
	NumGC                uint32 `json:"numGC"`
	PauseTotalNs         uint64 `json:"pauseTotalNs"`
	MemoryProfilePath    string `json:"memoryProfilePath"`
	HeapProfilePath      string `json:"heapProfilePath"`
	GoroutineProfilePath string `json:"goroutineProfilePath"`
}

func registerProfilingRoutes(mux *http.ServeMux, cfg config.Config, logger *logging.Logger) {
	if !cfg.MemoryProfilingEnabled {
		return
	}
	token := strings.TrimSpace(cfg.MemoryProfilingToken)
	if cfg.Env == "production" && token == "" {
		logger.Warn("memory profiling disabled: MEMORY_PROFILE_TOKEN is required in production")
		return
	}

	requireToken := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !profileTokenAuthorized(r, token) {
				w.Header().Set("WWW-Authenticate", `Bearer realm="second-brain-memory-profile"`)
				httputil.Error(w, http.StatusUnauthorized, "memory profiling token required")
				return
			}
			next.ServeHTTP(w, r)
		})
	}

	memoryStats := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gcTriggered := r.URL.Query().Get("gc") == "1" || strings.EqualFold(r.URL.Query().Get("gc"), "true")
		if gcTriggered {
			runtime.GC()
		}
		var stats runtime.MemStats
		runtime.ReadMemStats(&stats)
		response := memoryStatsResponse{
			Timestamp:            time.Now().UTC().Format(time.RFC3339Nano),
			GCTriggered:          gcTriggered,
			Goroutines:           runtime.NumGoroutine(),
			AllocBytes:           stats.Alloc,
			TotalAllocBytes:      stats.TotalAlloc,
			SysBytes:             stats.Sys,
			HeapAllocBytes:       stats.HeapAlloc,
			HeapInuseBytes:       stats.HeapInuse,
			HeapIdleBytes:        stats.HeapIdle,
			HeapReleasedBytes:    stats.HeapReleased,
			StackInuseBytes:      stats.StackInuse,
			NextGCBytes:          stats.NextGC,
			NumGC:                stats.NumGC,
			PauseTotalNs:         stats.PauseTotalNs,
			MemoryProfilePath:    "/api/debug/memory?gc=1",
			HeapProfilePath:      "/api/debug/pprof/heap?debug=1",
			GoroutineProfilePath: "/api/debug/pprof/goroutine?debug=1",
		}
		if stats.LastGC != 0 {
			response.LastGCTimestamp = unixNanoTimestamp(stats.LastGC)
		}
		httputil.JSON(w, http.StatusOK, response)
	})

	mux.Handle("GET /api/debug/memory", requireToken(memoryStats))
	mux.Handle("GET /api/debug/pprof/", requireToken(pprofHandler(pprof.Index)))
	mux.Handle("GET /api/debug/pprof/cmdline", requireToken(pprofHandler(pprof.Cmdline)))
	mux.Handle("GET /api/debug/pprof/profile", requireToken(pprofHandler(pprof.Profile)))
	mux.Handle("GET /api/debug/pprof/symbol", requireToken(pprofHandler(pprof.Symbol)))
	mux.Handle("GET /api/debug/pprof/trace", requireToken(pprofHandler(pprof.Trace)))
}

func unixNanoTimestamp(value uint64) string {
	const maxInt64 = uint64(^uint64(0) >> 1)
	if value > maxInt64 {
		value = maxInt64
	}
	return time.Unix(0, int64(value)).UTC().Format(time.RFC3339Nano)
}

func pprofHandler(handler func(http.ResponseWriter, *http.Request)) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		pprofRequest := r.Clone(r.Context())
		pprofURL := *r.URL
		pprofURL.Path = strings.TrimPrefix(pprofURL.Path, "/api")
		pprofRequest.URL = &pprofURL
		handler(w, pprofRequest)
	})
}

func profileTokenAuthorized(r *http.Request, token string) bool {
	if token == "" {
		return true
	}
	provided := strings.TrimSpace(r.Header.Get("X-Second-Brain-Profile-Token"))
	if provided == "" {
		provided = strings.TrimPrefix(strings.TrimSpace(r.Header.Get("Authorization")), "Bearer ")
	}
	if provided == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(provided), []byte(token)) == 1
}
