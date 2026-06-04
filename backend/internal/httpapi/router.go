package httpapi

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"html"
	"log/slog"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/abhijitmohanty/second-brain/backend/internal/config"
	"github.com/abhijitmohanty/second-brain/backend/internal/knowledge"
	"github.com/abhijitmohanty/second-brain/backend/internal/platform/httputil"
	"github.com/golang-jwt/jwt/v5"
)

type requestScope struct {
	service       *knowledge.Service
	ownerID       string
	authenticated bool
	publicOwner   bool
}

type supabaseAuthUser struct {
	ID    string `json:"id"`
	Email string `json:"email"`
}

func NewRouter(cfg config.Config, service *knowledge.Service, logger *slog.Logger) http.Handler {
	service.SetLogger(logger)
	mux := http.NewServeMux()
	publicOwnerID := strings.TrimSpace(cfg.PublicOwnerID)
	if publicOwnerID == "" {
		publicOwnerID = config.DefaultOwnerID
	}
	ownerServices := map[string]*knowledge.Service{
		service.OwnerID(): service,
	}
	var ownerServicesMu sync.Mutex
	serviceForOwner := func(ownerID string) *knowledge.Service {
		ownerID = strings.TrimSpace(ownerID)
		if ownerID == "" {
			ownerID = publicOwnerID
		}
		ownerServicesMu.Lock()
		defer ownerServicesMu.Unlock()
		if scoped, ok := ownerServices[ownerID]; ok {
			return scoped
		}
		scoped := service.ForOwner(ownerID)
		scoped.SetLogger(logger)
		ownerServices[ownerID] = scoped
		return scoped
	}
	findXOAuthService := func(state string) *knowledge.Service {
		state = strings.TrimSpace(state)
		ownerServicesMu.Lock()
		for _, scoped := range ownerServices {
			if scoped.HasXOAuthState(state) {
				ownerServicesMu.Unlock()
				return scoped
			}
		}
		ownerServicesMu.Unlock()
		return serviceForOwner(publicOwnerID)
	}
	resolveScope := func(w http.ResponseWriter, r *http.Request, requireAuth bool) (*requestScope, bool) {
		authUser, hasBearer, err := readSupabaseAuthUser(r.Context(), cfg, r.Header.Get("Authorization"))
		if err != nil {
			httputil.Error(w, http.StatusUnauthorized, err.Error())
			return nil, false
		}
		if !hasBearer {
			if requireAuth {
				httputil.Error(w, http.StatusUnauthorized, "Sign in with Supabase to use this action.")
				return nil, false
			}
			return &requestScope{service: serviceForOwner(publicOwnerID), ownerID: publicOwnerID, publicOwner: true}, true
		}
		ownerID, err := service.ResolveOwnerForAuthUser(r.Context(), authUser.ID, authUser.Email, publicOwnerID, cfg.PublicOwnerEmail)
		if err != nil {
			logger.Error("resolve authenticated owner", "error", err)
			httputil.Error(w, http.StatusUnauthorized, "Supabase user could not be mapped to a workspace.")
			return nil, false
		}
		return &requestScope{
			service:       serviceForOwner(ownerID),
			ownerID:       ownerID,
			authenticated: true,
			publicOwner:   ownerID == publicOwnerID,
		}, true
	}

	healthz := func(w http.ResponseWriter, r *http.Request) {
		httputil.JSON(w, http.StatusOK, map[string]string{"status": "ok", "env": cfg.Env})
	}
	mux.HandleFunc("GET /healthz", healthz)
	mux.HandleFunc("GET /api/healthz", healthz)

	readLatest := func(w http.ResponseWriter, r *http.Request) {
		scope, ok := resolveScope(w, r, false)
		if !ok {
			return
		}
		latest, err := scope.service.ReadLatest(r.Context())
		if err != nil {
			logger.Error("read latest knowledge run", "error", err)
			httputil.Error(w, http.StatusInternalServerError, "read latest knowledge run")
			return
		}
		setReadModelCacheHeadersForScope(w, scope)
		httputil.JSON(w, http.StatusOK, map[string]any{"latest": latest})
	}

	readAppState := func(w http.ResponseWriter, r *http.Request) {
		scope, ok := resolveScope(w, r, false)
		if !ok {
			return
		}
		start := time.Now()
		view := strings.TrimSpace(r.URL.Query().Get("view"))
		limit := queryInt(r, "limit")
		var state *knowledge.AppState
		var cacheStatus string
		var err error
		if view != "" {
			state, cacheStatus, err = scope.service.ReadAppStateView(r.Context(), view, limit)
		} else {
			state, cacheStatus, err = scope.service.ReadAppState(r.Context())
		}
		if err != nil {
			logger.Error("read app state", "error", err)
			httputil.Error(w, http.StatusInternalServerError, "read app state")
			return
		}
		setReadModelCacheHeadersForScope(w, scope)
		w.Header().Set("Server-Timing", fmt.Sprintf(`appstate;dur=%d, cache;desc="%s"`, time.Since(start).Milliseconds(), cacheStatus))
		if cacheStatus != "" {
			w.Header().Set("X-Second-Brain-Cache", cacheStatus)
		}
		if state != nil && state.Manifest.ETag != "" {
			etag := responseETag(state.Manifest.ETag)
			if view != "" {
				etag = responseETag(state.Manifest.ETag, view, knowledge.NormalizeAppStateViewLimit(view, limit))
			}
			w.Header().Set("ETag", etag)
			if etagMatches(r.Header.Get("If-None-Match"), etag) {
				w.WriteHeader(http.StatusNotModified)
				return
			}
		}
		httputil.JSON(w, http.StatusOK, state)
	}

	runInbox := func(w http.ResponseWriter, r *http.Request) {
		scope, ok := resolveScope(w, r, true)
		if !ok {
			return
		}
		status := scope.service.StartRefresh()
		httputil.JSON(w, http.StatusAccepted, status)
	}

	readRefreshStatus := func(w http.ResponseWriter, r *http.Request) {
		scope, ok := resolveScope(w, r, false)
		if !ok {
			return
		}
		httputil.JSON(w, http.StatusOK, scope.service.ReadRefreshStatus(r.Context()))
	}

	saveFeedback := func(w http.ResponseWriter, r *http.Request) {
		scope, ok := resolveScope(w, r, true)
		if !ok {
			return
		}
		var event knowledge.FeedbackEvent
		if err := json.NewDecoder(r.Body).Decode(&event); err != nil {
			httputil.Error(w, http.StatusBadRequest, "invalid feedback payload")
			return
		}
		if err := scope.service.SaveFeedback(r.Context(), event); err != nil {
			httputil.Error(w, http.StatusBadRequest, err.Error())
			return
		}
		httputil.JSON(w, http.StatusCreated, map[string]string{"status": "saved"})
	}

	listDigests := func(w http.ResponseWriter, r *http.Request) {
		scope, ok := resolveScope(w, r, false)
		if !ok {
			return
		}
		digests, err := scope.service.ReadDigests(r.Context(), 50)
		if err != nil {
			logger.Error("list digests", "error", err)
			httputil.Error(w, http.StatusInternalServerError, "list digests")
			return
		}
		setReadModelCacheHeadersForScope(w, scope)
		httputil.JSON(w, http.StatusOK, map[string]any{"digests": digests})
	}

	readDigestIllustration := func(w http.ResponseWriter, r *http.Request) {
		scope, ok := resolveScope(w, r, false)
		if !ok {
			return
		}
		digestID := strings.TrimSpace(r.PathValue("id"))
		if digestID == "" {
			httputil.Error(w, http.StatusNotFound, "digest illustration not found")
			return
		}
		illustration, err := scope.service.ReadDigestIllustration(r.Context(), digestID)
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
		w.Header().Set("Cache-Control", "public, max-age=31536000, s-maxage=31536000, immutable")
		etag := responseETag(digestID, "illustration", 0)
		w.Header().Set("ETag", etag)
		if etagMatches(r.Header.Get("If-None-Match"), etag) {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		if illustration.Alt != "" {
			w.Header().Set("X-Image-Alt", illustration.Alt)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(raw)
	}

	shareTweet := func(w http.ResponseWriter, r *http.Request) {
		scope, ok := resolveScope(w, r, true)
		if !ok {
			return
		}
		var input knowledge.TweetShareRequest
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			httputil.Error(w, http.StatusBadRequest, "invalid tweet payload")
			return
		}
		result, err := scope.service.ShareTweet(r.Context(), input)
		if err != nil {
			logger.Error("share tweet", "error", err)
			httputil.Error(w, http.StatusBadRequest, err.Error())
			return
		}
		httputil.JSON(w, http.StatusCreated, result)
	}

	askSecondBrain := func(w http.ResponseWriter, r *http.Request) {
		scope, ok := resolveScope(w, r, true)
		if !ok {
			return
		}
		var input knowledge.AskSecondBrainRequest
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			httputil.Error(w, http.StatusBadRequest, "invalid ask payload")
			return
		}
		result, err := scope.service.AskSecondBrain(r.Context(), input)
		if err != nil {
			logger.Error("ask second brain", "error", err)
			httputil.Error(w, http.StatusBadRequest, err.Error())
			return
		}
		httputil.JSON(w, http.StatusOK, result)
	}

	readInsightGraph := func(w http.ResponseWriter, r *http.Request) {
		scope, ok := resolveScope(w, r, false)
		if !ok {
			return
		}
		limit, err := knowledge.NormalizeInsightGraphLimit(r.URL.Query().Get("limit"))
		if err != nil {
			httputil.Error(w, http.StatusBadRequest, err.Error())
			return
		}
		graph, err := scope.service.ReadInsightGraph(r.Context(), limit)
		if err != nil {
			logger.Error("read insight graph", "error", err)
			httputil.Error(w, http.StatusServiceUnavailable, err.Error())
			return
		}
		setReadModelCacheHeaders(w)
		httputil.JSON(w, http.StatusOK, graph)
	}

	startXAuth := func(w http.ResponseWriter, r *http.Request) {
		scope, ok := resolveScope(w, r, false)
		if !ok {
			return
		}
		url, err := scope.service.BeginXOAuth(r.Context())
		if err != nil {
			httputil.Error(w, http.StatusBadRequest, err.Error())
			return
		}
		http.Redirect(w, r, url, http.StatusFound)
	}

	startXAuthJSON := func(w http.ResponseWriter, r *http.Request) {
		scope, ok := resolveScope(w, r, true)
		if !ok {
			return
		}
		url, err := scope.service.BeginXOAuth(r.Context())
		if err != nil {
			httputil.Error(w, http.StatusBadRequest, err.Error())
			return
		}
		httputil.JSON(w, http.StatusOK, map[string]string{"url": url})
	}

	completeXAuth := func(w http.ResponseWriter, r *http.Request) {
		if oauthErr := strings.TrimSpace(r.URL.Query().Get("error")); oauthErr != "" {
			detail := strings.TrimSpace(r.URL.Query().Get("error_description"))
			httputil.Error(w, http.StatusBadRequest, strings.TrimSpace(oauthErr+" "+detail))
			return
		}
		state := r.URL.Query().Get("state")
		xService := findXOAuthService(state)
		result, err := xService.CompleteXOAuth(r.Context(), state, r.URL.Query().Get("code"))
		if err != nil {
			logger.Error("complete X OAuth", "error", err)
			httputil.Error(w, http.StatusBadRequest, err.Error())
			return
		}
		if err := setSessionCookie(w, cfg, xService.OwnerID(), result.Profile.ID); err != nil {
			httputil.Error(w, http.StatusInternalServerError, err.Error())
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprintf(w, "<h1>X authorization saved</h1><p>Authorized @%s. You can close this tab.</p>", html.EscapeString(result.Profile.Username))
	}

	readXAuthStatus := func(w http.ResponseWriter, r *http.Request) {
		scope, ok := resolveScope(w, r, false)
		if !ok {
			return
		}
		httputil.JSON(w, http.StatusOK, scope.service.XAuthStatus(r.Context()))
	}

	readWorkspace := func(w http.ResponseWriter, r *http.Request) {
		scope, ok := resolveScope(w, r, false)
		if !ok {
			return
		}
		status, err := scope.service.WorkspaceStatus(r.Context(), scope.authenticated)
		if err != nil {
			logger.Error("read workspace", "error", err)
			httputil.Error(w, http.StatusInternalServerError, "read workspace")
			return
		}
		httputil.JSON(w, http.StatusOK, status)
	}

	saveYouTubeConnection := func(w http.ResponseWriter, r *http.Request) {
		scope, ok := resolveScope(w, r, true)
		if !ok {
			return
		}
		var input knowledge.YouTubePlaylistInput
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			httputil.Error(w, http.StatusBadRequest, "invalid YouTube playlist payload")
			return
		}
		connection, err := scope.service.SaveYouTubePlaylist(r.Context(), input)
		if err != nil {
			logger.Error("save YouTube playlist", "error", err)
			httputil.Error(w, http.StatusBadRequest, err.Error())
			return
		}
		httputil.JSON(w, http.StatusOK, connection)
	}

	mux.HandleFunc("GET /api/auth/x", startXAuth)
	mux.HandleFunc("GET /api/auth/x/start", startXAuthJSON)
	mux.HandleFunc("GET /api/auth/x/callback", completeXAuth)
	mux.HandleFunc("GET /api/auth/x/status", readXAuthStatus)
	mux.HandleFunc("GET /api/workspace", readWorkspace)
	mux.HandleFunc("POST /api/source-connections/youtube", saveYouTubeConnection)
	mux.HandleFunc("GET /api/app-state", readAppState)
	mux.HandleFunc("GET /api/knowledge-runs/latest", readLatest)
	mux.HandleFunc("GET /api/knowledge-runs/refresh", readRefreshStatus)
	mux.HandleFunc("POST /api/knowledge-runs/refresh", runInbox)
	mux.HandleFunc("POST /api/feedback", saveFeedback)
	mux.HandleFunc("GET /api/digests", listDigests)
	mux.HandleFunc("GET /api/digests/{id}/illustration", readDigestIllustration)
	mux.HandleFunc("POST /api/share/tweet", shareTweet)
	mux.HandleFunc("POST /api/ask", askSecondBrain)
	mux.HandleFunc("GET /api/knowledge-graph/insights", readInsightGraph)
	registerProfilingRoutes(mux, cfg, logger)

	return requestLogger(logger, cors(cfg.AllowedOrigins, mux))
}

func setSessionCookie(w http.ResponseWriter, cfg config.Config, ownerID string, xUserID string) error {
	secret := strings.TrimSpace(cfg.XSessionSecret)
	if secret == "" {
		return fmt.Errorf("X_SESSION_SECRET is required to issue the backend session cookie")
	}
	now := time.Now().UTC()
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"iss":       "second-brain",
		"sub":       ownerID,
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

func queryInt(r *http.Request, key string) int {
	raw := strings.TrimSpace(r.URL.Query().Get(key))
	if raw == "" {
		return 0
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return 0
	}
	return value
}

func setReadModelCacheHeaders(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "public, max-age=30, s-maxage=300, stale-while-revalidate=1800")
}

func setReadModelCacheHeadersForScope(w http.ResponseWriter, scope *requestScope) {
	if scope != nil && scope.authenticated {
		w.Header().Set("Cache-Control", "private, no-store")
		return
	}
	setReadModelCacheHeaders(w)
}

func readSupabaseAuthUser(ctx context.Context, cfg config.Config, authorization string) (supabaseAuthUser, bool, error) {
	const bearerPrefix = "Bearer "
	authorization = strings.TrimSpace(authorization)
	if authorization == "" {
		return supabaseAuthUser{}, false, nil
	}
	if !strings.HasPrefix(authorization, bearerPrefix) {
		return supabaseAuthUser{}, true, fmt.Errorf("Authorization header must use a bearer token.")
	}
	token := strings.TrimSpace(strings.TrimPrefix(authorization, bearerPrefix))
	if token == "" {
		return supabaseAuthUser{}, true, fmt.Errorf("Authorization bearer token is empty.")
	}
	baseURL := strings.TrimRight(strings.TrimSpace(cfg.SupabaseURL), "/")
	publishableKey := strings.TrimSpace(cfg.SupabasePublishableKey)
	if baseURL == "" || publishableKey == "" {
		return supabaseAuthUser{}, true, fmt.Errorf("Supabase auth is not configured.")
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/auth/v1/user", nil)
	if err != nil {
		return supabaseAuthUser{}, true, err
	}
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("apikey", publishableKey)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return supabaseAuthUser{}, true, fmt.Errorf("Supabase auth validation failed.")
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return supabaseAuthUser{}, true, fmt.Errorf("Supabase session is invalid or expired.")
	}
	var user supabaseAuthUser
	if err := json.NewDecoder(response.Body).Decode(&user); err != nil {
		return supabaseAuthUser{}, true, fmt.Errorf("Supabase auth response could not be decoded.")
	}
	user.ID = strings.TrimSpace(user.ID)
	user.Email = strings.TrimSpace(user.Email)
	if user.ID == "" {
		return supabaseAuthUser{}, true, fmt.Errorf("Supabase auth response did not include a user id.")
	}
	return user, true, nil
}

func responseETag(parts ...any) string {
	values := []string{}
	for _, part := range parts {
		value := strings.TrimSpace(fmt.Sprint(part))
		if value != "" && value != "0" {
			values = append(values, value)
		}
	}
	return `"` + strings.Join(values, ":") + `"`
}

func etagMatches(header string, etag string) bool {
	for _, value := range strings.Split(header, ",") {
		if strings.TrimSpace(value) == etag {
			return true
		}
	}
	return false
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
