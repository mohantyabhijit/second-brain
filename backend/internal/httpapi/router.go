package httpapi

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"html"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/abhijitmohanty/second-brain/backend/internal/config"
	"github.com/abhijitmohanty/second-brain/backend/internal/knowledge"
	"github.com/abhijitmohanty/second-brain/backend/internal/platform/httputil"
	"github.com/golang-jwt/jwt/v5"
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

	readAppState := func(w http.ResponseWriter, r *http.Request) {
		state, cacheStatus, err := service.ReadAppState(r.Context())
		if err != nil {
			logger.Error("read app state", "error", err)
			httputil.Error(w, http.StatusInternalServerError, "read app state")
			return
		}
		if cacheStatus != "" {
			w.Header().Set("X-Second-Brain-Cache", cacheStatus)
		}
		if state != nil && state.Manifest.ETag != "" {
			w.Header().Set("ETag", `"`+state.Manifest.ETag+`"`)
		}
		httputil.JSON(w, http.StatusOK, state)
	}

	runInbox := func(w http.ResponseWriter, r *http.Request) {
		status := service.StartRefresh()
		httputil.JSON(w, http.StatusAccepted, status)
	}

	readRefreshStatus := func(w http.ResponseWriter, r *http.Request) {
		httputil.JSON(w, http.StatusOK, service.ReadRefreshStatus(r.Context()))
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

	listDigests := func(w http.ResponseWriter, r *http.Request) {
		digests, err := service.ReadDigests(r.Context(), 50)
		if err != nil {
			logger.Error("list digests", "error", err)
			httputil.Error(w, http.StatusInternalServerError, "list digests")
			return
		}
		httputil.JSON(w, http.StatusOK, map[string]any{"digests": digests})
	}

	readDigestIllustration := func(w http.ResponseWriter, r *http.Request) {
		digestID := strings.TrimSpace(r.PathValue("id"))
		if digestID == "" {
			httputil.Error(w, http.StatusNotFound, "digest illustration not found")
			return
		}
		illustration, err := service.ReadDigestIllustration(r.Context(), digestID)
		if err != nil {
			logger.Error("read digest illustration", "error", err)
			httputil.Error(w, http.StatusInternalServerError, "read digest illustration")
			return
		}
		if illustration == nil {
			httputil.Error(w, http.StatusNotFound, "digest illustration not found")
			return
		}
		raw, err := base64.StdEncoding.DecodeString(illustration.Base64)
		if err != nil {
			logger.Error("decode digest illustration", "digest_id", digestID, "error", err)
			httputil.Error(w, http.StatusInternalServerError, "decode digest illustration")
			return
		}
		mimeType := strings.TrimSpace(illustration.MimeType)
		if mimeType == "" {
			mimeType = http.DetectContentType(raw)
		}
		w.Header().Set("Content-Type", mimeType)
		w.Header().Set("Cache-Control", "public, max-age=3600")
		if illustration.Alt != "" {
			w.Header().Set("X-Image-Alt", illustration.Alt)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(raw)
	}

	sendDigest := func(w http.ResponseWriter, r *http.Request) {
		var input knowledge.DigestSendRequest
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			httputil.Error(w, http.StatusBadRequest, "invalid digest delivery payload")
			return
		}
		var digest *knowledge.DigestIssue
		var err error
		if input.Digest != nil {
			digest, err = service.SendProvidedDigest(r.Context(), input.RecipientEmail, *input.Digest)
		} else {
			digest, err = service.SendLatestDigest(r.Context(), input.RecipientEmail)
		}
		if err != nil {
			logger.Error("send latest digest", "error", err)
			httputil.Error(w, http.StatusBadRequest, err.Error())
			return
		}
		httputil.JSON(w, http.StatusOK, digest)
	}

	shareTweet := func(w http.ResponseWriter, r *http.Request) {
		var input knowledge.TweetShareRequest
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			httputil.Error(w, http.StatusBadRequest, "invalid tweet payload")
			return
		}
		result, err := service.ShareTweet(r.Context(), input)
		if err != nil {
			logger.Error("share tweet", "error", err)
			httputil.Error(w, http.StatusBadRequest, err.Error())
			return
		}
		httputil.JSON(w, http.StatusCreated, result)
	}

	askSecondBrain := func(w http.ResponseWriter, r *http.Request) {
		var input knowledge.AskSecondBrainRequest
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			httputil.Error(w, http.StatusBadRequest, "invalid ask payload")
			return
		}
		result, err := service.AskSecondBrain(r.Context(), input)
		if err != nil {
			logger.Error("ask second brain", "error", err)
			httputil.Error(w, http.StatusBadRequest, err.Error())
			return
		}
		httputil.JSON(w, http.StatusOK, result)
	}

	readInsightGraph := func(w http.ResponseWriter, r *http.Request) {
		limit, err := knowledge.NormalizeInsightGraphLimit(r.URL.Query().Get("limit"))
		if err != nil {
			httputil.Error(w, http.StatusBadRequest, err.Error())
			return
		}
		graph, err := service.ReadInsightGraph(r.Context(), limit)
		if err != nil {
			logger.Error("read insight graph", "error", err)
			httputil.Error(w, http.StatusServiceUnavailable, err.Error())
			return
		}
		httputil.JSON(w, http.StatusOK, graph)
	}

	startXAuth := func(w http.ResponseWriter, r *http.Request) {
		url, err := service.BeginXOAuth(r.Context())
		if err != nil {
			httputil.Error(w, http.StatusBadRequest, err.Error())
			return
		}
		http.Redirect(w, r, url, http.StatusFound)
	}

	completeXAuth := func(w http.ResponseWriter, r *http.Request) {
		if oauthErr := strings.TrimSpace(r.URL.Query().Get("error")); oauthErr != "" {
			detail := strings.TrimSpace(r.URL.Query().Get("error_description"))
			httputil.Error(w, http.StatusBadRequest, strings.TrimSpace(oauthErr+" "+detail))
			return
		}
		result, err := service.CompleteXOAuth(r.Context(), r.URL.Query().Get("state"), r.URL.Query().Get("code"))
		if err != nil {
			logger.Error("complete X OAuth", "error", err)
			httputil.Error(w, http.StatusBadRequest, err.Error())
			return
		}
		if err := setSessionCookie(w, cfg, result.Profile.ID); err != nil {
			httputil.Error(w, http.StatusInternalServerError, err.Error())
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprintf(w, "<h1>X authorization saved</h1><p>Authorized @%s. You can close this tab.</p>", html.EscapeString(result.Profile.Username))
	}

	readXAuthStatus := func(w http.ResponseWriter, r *http.Request) {
		httputil.JSON(w, http.StatusOK, service.XAuthStatus(r.Context()))
	}

	mux.HandleFunc("GET /api/auth/x", startXAuth)
	mux.HandleFunc("GET /api/auth/x/callback", completeXAuth)
	mux.HandleFunc("GET /api/auth/x/status", readXAuthStatus)
	mux.HandleFunc("GET /api/app-state", readAppState)
	mux.HandleFunc("GET /api/knowledge-runs/latest", readLatest)
	mux.HandleFunc("GET /api/knowledge-runs/refresh", readRefreshStatus)
	mux.HandleFunc("POST /api/knowledge-runs/refresh", runInbox)
	mux.HandleFunc("POST /api/feedback", saveFeedback)
	mux.HandleFunc("GET /api/digests", listDigests)
	mux.HandleFunc("GET /api/digests/{id}/illustration", readDigestIllustration)
	mux.HandleFunc("POST /api/digests/generate", generateDigest)
	mux.HandleFunc("POST /api/digests/send", sendDigest)
	mux.HandleFunc("POST /api/share/tweet", shareTweet)
	mux.HandleFunc("POST /api/ask", askSecondBrain)
	mux.HandleFunc("GET /api/knowledge-graph/insights", readInsightGraph)

	return requestLogger(logger, cors(cfg.AllowedOrigins, mux))
}

func setSessionCookie(w http.ResponseWriter, cfg config.Config, xUserID string) error {
	secret := strings.TrimSpace(cfg.XSessionSecret)
	if secret == "" {
		return fmt.Errorf("X_SESSION_SECRET is required to issue the backend session cookie")
	}
	now := time.Now().UTC()
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"iss":       "second-brain",
		"sub":       cfg.OwnerID,
		"x_user_id": xUserID,
		"iat":       now.Unix(),
		"exp":       now.Add(time.Hour).Unix(),
	})
	signed, err := token.SignedString([]byte(secret))
	if err != nil {
		return err
	}
	name := strings.TrimSpace(cfg.XSessionCookieName)
	if name == "" {
		name = "second_brain_session"
	}
	http.SetCookie(w, &http.Cookie{
		Name:     name,
		Value:    signed,
		Path:     "/",
		MaxAge:   int(time.Hour.Seconds()),
		HttpOnly: true,
		Secure:   cfg.Env == "production",
		SameSite: http.SameSiteLaxMode,
	})
	return nil
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
