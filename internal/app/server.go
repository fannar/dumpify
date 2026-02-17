package app

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"dumpify/internal/db"
	"dumpify/internal/domain"
	"dumpify/internal/exporter"
	"dumpify/internal/services"
	"dumpify/internal/ui"
)

const (
	stateCookieName   = "dumpify_oauth_state"
	accountCookieName = "dumpify_account"
)

type Server struct {
	cfg       Config
	store     *db.Store
	renderer  *ui.Renderer
	providers map[string]services.MusicService
	jobs      *exportJobManager
}

func NewServer(cfg Config, store *db.Store, renderer *ui.Renderer, providers map[string]services.MusicService) *Server {
	return &Server{
		cfg:       cfg,
		store:     store,
		renderer:  renderer,
		providers: providers,
		jobs:      newExportJobManager(),
	}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /", s.handleHome)
	mux.HandleFunc("GET /auth/{provider}/login", s.handleLogin)
	mux.HandleFunc("POST /export", s.handleExport)
	mux.HandleFunc("POST /export/playlist", s.handleExportSinglePlaylist)
	mux.HandleFunc("GET /api/playlists", s.handlePlaylists)
	mux.HandleFunc("POST /api/exports/start", s.handleStartExport)
	mux.HandleFunc("GET /api/exports/{id}", s.handleExportStatus)
	mux.HandleFunc("GET /downloads/{id}", s.handleDownload)

	for name, provider := range s.providers {
		callbackPath := provider.CallbackPath()
		mux.HandleFunc("GET "+callbackPath, s.makeCallbackHandler(name))
	}

	return s.withLogging(mux)
}

func (s *Server) handleHome(w http.ResponseWriter, r *http.Request) {
	account, connected := s.currentAccount(r)
	var exports []domain.ExportRecord
	if connected {
		exports = s.store.ListExportsForAccount(account.ID, 20)
	}

	data := struct {
		Providers []string
		Connected bool
		Account   domain.Account
		Exports   []domain.ExportRecord
	}{
		Providers: providerNames(s.providers),
		Connected: connected,
		Account:   account,
		Exports:   exports,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.renderer.Render(w, "index.html", data); err != nil {
		log.Printf("render home: %v", err)
		http.Error(w, "failed to render page", http.StatusInternalServerError)
		return
	}
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	providerName := strings.ToLower(strings.TrimSpace(r.PathValue("provider")))
	provider, ok := s.providers[providerName]
	if !ok {
		http.NotFound(w, r)
		return
	}

	state, err := randomState(24)
	if err != nil {
		http.Error(w, "failed to create oauth state", http.StatusInternalServerError)
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     stateCookieName,
		Value:    state,
		Path:     "/",
		HttpOnly: true,
		Secure:   isSecureRequest(r),
		SameSite: http.SameSiteLaxMode,
		Expires:  time.Now().Add(10 * time.Minute),
	})

	http.Redirect(w, r, provider.AuthURL(state), http.StatusFound)
}

func (s *Server) makeCallbackHandler(providerName string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		provider := s.providers[providerName]

		if spotifyErr := r.URL.Query().Get("error"); spotifyErr != "" {
			http.Error(w, "provider login failed: "+spotifyErr, http.StatusBadRequest)
			return
		}

		state := strings.TrimSpace(r.URL.Query().Get("state"))
		code := strings.TrimSpace(r.URL.Query().Get("code"))
		if state == "" || code == "" {
			http.Error(w, "missing oauth code or state", http.StatusBadRequest)
			return
		}

		cookie, err := r.Cookie(stateCookieName)
		if err != nil || cookie.Value == "" || cookie.Value != state {
			http.Error(w, "invalid oauth state", http.StatusBadRequest)
			return
		}

		token, err := provider.ExchangeCode(r.Context(), code, state)
		if err != nil {
			log.Printf("exchange %s code: %v", providerName, err)
			http.Error(w, "failed to exchange oauth code", http.StatusBadGateway)
			return
		}

		user, updatedToken, err := provider.CurrentUser(r.Context(), token)
		if err != nil {
			log.Printf("load %s user: %v", providerName, err)
			http.Error(w, "failed to load user", http.StatusBadGateway)
			return
		}

		account, err := s.store.UpsertAccount(providerName, user, updatedToken)
		if err != nil {
			log.Printf("upsert account: %v", err)
			http.Error(w, "failed to save account", http.StatusInternalServerError)
			return
		}

		http.SetCookie(w, &http.Cookie{
			Name:     accountCookieName,
			Value:    fmt.Sprintf("%s:%d", providerName, account.ID),
			Path:     "/",
			HttpOnly: true,
			Secure:   isSecureRequest(r),
			SameSite: http.SameSiteLaxMode,
			Expires:  time.Now().Add(30 * 24 * time.Hour),
		})

		http.SetCookie(w, &http.Cookie{
			Name:     stateCookieName,
			Value:    "",
			Path:     "/",
			HttpOnly: true,
			Secure:   isSecureRequest(r),
			SameSite: http.SameSiteLaxMode,
			MaxAge:   -1,
		})

		http.Redirect(w, r, "/", http.StatusFound)
	}
}

func (s *Server) handleExport(w http.ResponseWriter, r *http.Request) {
	account, ok := s.currentAccount(r)
	if !ok {
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}

	format, err := parseExportFormat(r.FormValue("format"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	record, err := s.executeExport(r.Context(), account, format, nil)
	if err != nil {
		log.Printf("export failed: %v", err)
		http.Error(w, "failed to export playlists", http.StatusBadGateway)
		return
	}

	http.Redirect(w, r, fmt.Sprintf("/downloads/%d", record.ID), http.StatusFound)
}

func (s *Server) handleExportSinglePlaylist(w http.ResponseWriter, r *http.Request) {
	account, ok := s.currentAccount(r)
	if !ok {
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}

	format, err := parseExportFormat(r.FormValue("format"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	playlistID := strings.TrimSpace(r.FormValue("playlist_id"))
	if playlistID == "" {
		http.Error(w, "playlist_id is required", http.StatusBadRequest)
		return
	}

	record, err := s.executeSinglePlaylistExport(r.Context(), account, format, playlistID)
	if err != nil {
		log.Printf("single playlist export failed: %v", err)
		http.Error(w, "failed to export playlist", http.StatusBadGateway)
		return
	}

	http.Redirect(w, r, fmt.Sprintf("/downloads/%d", record.ID), http.StatusFound)
}

func (s *Server) handlePlaylists(w http.ResponseWriter, r *http.Request) {
	account, ok := s.currentAccount(r)
	if !ok {
		http.Error(w, "not authenticated", http.StatusUnauthorized)
		return
	}

	provider, ok := s.providers[account.Provider]
	if !ok {
		http.Error(w, "provider is not configured", http.StatusBadRequest)
		return
	}

	playlists, newToken, err := provider.ListPlaylists(r.Context(), account.Token)
	if err != nil {
		log.Printf("list playlists: %v", err)
		http.Error(w, "failed to list playlists", http.StatusBadGateway)
		return
	}

	if err := s.store.UpdateAccountToken(account.ID, newToken); err != nil {
		log.Printf("update token after list playlists: %v", err)
		http.Error(w, "failed to persist token", http.StatusInternalServerError)
		return
	}

	type playlistListItem struct {
		ID            string `json:"id"`
		Name          string `json:"name"`
		Description   string `json:"description"`
		OwnerID       string `json:"owner_id"`
		Public        bool   `json:"public"`
		Collaborative bool   `json:"collaborative"`
	}
	items := make([]playlistListItem, 0, len(playlists))
	for _, pl := range playlists {
		items = append(items, playlistListItem{
			ID:            pl.ID,
			Name:          pl.Name,
			Description:   pl.Description,
			OwnerID:       pl.OwnerID,
			Public:        pl.Public,
			Collaborative: pl.Collaborative,
		})
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(struct {
		Provider  string             `json:"provider"`
		Playlists []playlistListItem `json:"playlists"`
	}{
		Provider:  account.Provider,
		Playlists: items,
	})
}

func (s *Server) handleStartExport(w http.ResponseWriter, r *http.Request) {
	account, ok := s.currentAccount(r)
	if !ok {
		http.Error(w, "not authenticated", http.StatusUnauthorized)
		return
	}

	format, err := parseExportFormat(r.FormValue("format"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	job, err := s.jobs.create(account.ID, account.Provider, format)
	if err != nil {
		log.Printf("create export job: %v", err)
		http.Error(w, "failed to create export job", http.StatusInternalServerError)
		return
	}

	go s.runExportJob(job.ID, account, format)

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(job)
}

func (s *Server) handleExportStatus(w http.ResponseWriter, r *http.Request) {
	account, ok := s.currentAccount(r)
	if !ok {
		http.Error(w, "not authenticated", http.StatusUnauthorized)
		return
	}

	jobID := strings.TrimSpace(r.PathValue("id"))
	if jobID == "" {
		http.NotFound(w, r)
		return
	}

	job, ok := s.jobs.get(jobID)
	if !ok || job.AccountID != account.ID {
		http.NotFound(w, r)
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(job)
}

func (s *Server) runExportJob(jobID string, account domain.Account, format string) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	record, err := s.executeExport(ctx, account, format, func(progress services.DumpProgress) {
		s.jobs.update(jobID, func(job *exportJob) {
			if progress.TotalPlaylists > 0 {
				job.TotalPlaylists = progress.TotalPlaylists
			}
			job.CompletedPlaylists = progress.CompletedPlaylists
			job.SkippedPlaylists = progress.SkippedPlaylists
			job.CurrentPlaylist = progress.CurrentPlaylist

			if progress.LastCompletedPlaylist != "" {
				job.CompletedPlaylistNames = appendIfChanged(job.CompletedPlaylistNames, progress.LastCompletedPlaylist)
			}
			if progress.LastSkippedPlaylist != "" {
				job.SkippedPlaylistNames = appendIfChanged(job.SkippedPlaylistNames, progress.LastSkippedPlaylist)
			}
		})
	})
	if err != nil {
		log.Printf("export job %s failed: %v", jobID, err)
		s.jobs.update(jobID, func(job *exportJob) {
			job.State = "failed"
			job.Error = err.Error()
			job.CurrentPlaylist = ""
		})
		return
	}

	s.jobs.update(jobID, func(job *exportJob) {
		job.State = "done"
		job.DownloadURL = fmt.Sprintf("/downloads/%d", record.ID)
		job.CurrentPlaylist = ""
	})
}

func (s *Server) executeExport(ctx context.Context, account domain.Account, format string, onProgress func(services.DumpProgress)) (domain.ExportRecord, error) {
	provider, ok := s.providers[account.Provider]
	if !ok {
		return domain.ExportRecord{}, fmt.Errorf("provider %q is not configured", account.Provider)
	}

	dump, newToken, err := provider.DumpPlaylists(ctx, account.Token, onProgress)
	if err != nil {
		return domain.ExportRecord{}, fmt.Errorf("dump playlists: %w", err)
	}

	if err := s.store.UpdateAccountToken(account.ID, newToken); err != nil {
		return domain.ExportRecord{}, fmt.Errorf("update token: %w", err)
	}

	return s.writeDumpAndCreateRecord(account, format, dump)
}

func (s *Server) executeSinglePlaylistExport(ctx context.Context, account domain.Account, format, playlistID string) (domain.ExportRecord, error) {
	provider, ok := s.providers[account.Provider]
	if !ok {
		return domain.ExportRecord{}, fmt.Errorf("provider %q is not configured", account.Provider)
	}

	user, tokenAfterUser, err := provider.CurrentUser(ctx, account.Token)
	if err != nil {
		return domain.ExportRecord{}, fmt.Errorf("load user: %w", err)
	}

	playlist, tokenAfterPlaylist, err := provider.DumpPlaylist(ctx, tokenAfterUser, playlistID)
	if err != nil {
		return domain.ExportRecord{}, fmt.Errorf("dump playlist %q: %w", playlistID, err)
	}

	if err := s.store.UpdateAccountToken(account.ID, tokenAfterPlaylist); err != nil {
		return domain.ExportRecord{}, fmt.Errorf("update token: %w", err)
	}

	dump := domain.PlaylistDump{
		Provider:   provider.Name(),
		ExportedAt: time.Now().UTC(),
		User:       user,
		Playlists:  []domain.Playlist{playlist},
	}
	return s.writeDumpAndCreateRecord(account, format, dump)
}

func (s *Server) writeDumpAndCreateRecord(account domain.Account, format string, dump domain.PlaylistDump) (domain.ExportRecord, error) {
	fileName := exporter.BuildFilename(account.Provider, dump.User.ID, format, time.Now().UTC())
	filePath := filepath.Join(s.cfg.ExportDir, fileName)

	var err error
	switch format {
	case "json":
		err = exporter.WriteJSON(filePath, dump)
	case "csv":
		err = exporter.WriteCSV(filePath, dump)
	}
	if err != nil {
		return domain.ExportRecord{}, fmt.Errorf("write export file: %w", err)
	}

	record, err := s.store.CreateExport(account.ID, account.Provider, format, filePath)
	if err != nil {
		return domain.ExportRecord{}, fmt.Errorf("save export record: %w", err)
	}
	return record, nil
}

func (s *Server) handleDownload(w http.ResponseWriter, r *http.Request) {
	account, ok := s.currentAccount(r)
	if !ok {
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}

	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	record, ok := s.store.GetExport(id)
	if !ok || record.AccountID != account.ID {
		http.NotFound(w, r)
		return
	}

	target, err := filepath.Abs(record.FilePath)
	if err != nil {
		http.Error(w, "invalid file path", http.StatusInternalServerError)
		return
	}
	exportRoot, err := filepath.Abs(s.cfg.ExportDir)
	if err != nil {
		http.Error(w, "invalid export root", http.StatusInternalServerError)
		return
	}
	if !strings.HasPrefix(target, exportRoot+string(os.PathSeparator)) && target != exportRoot {
		http.Error(w, "invalid file path", http.StatusForbidden)
		return
	}

	fileName := filepath.Base(record.FilePath)
	contentType := "application/octet-stream"
	if record.Format == "json" {
		contentType = "application/json"
	}
	if record.Format == "csv" {
		contentType = "text/csv"
	}

	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", fileName))
	http.ServeFile(w, r, target)
}

func (s *Server) currentAccount(r *http.Request) (domain.Account, bool) {
	cookie, err := r.Cookie(accountCookieName)
	if err != nil || cookie.Value == "" {
		return domain.Account{}, false
	}

	parts := strings.SplitN(cookie.Value, ":", 2)
	if len(parts) != 2 {
		return domain.Account{}, false
	}
	provider := strings.ToLower(strings.TrimSpace(parts[0]))
	id, err := strconv.ParseInt(strings.TrimSpace(parts[1]), 10, 64)
	if err != nil {
		return domain.Account{}, false
	}

	account, ok := s.store.GetAccount(id)
	if !ok {
		return domain.Account{}, false
	}
	if account.Provider != provider {
		return domain.Account{}, false
	}
	return account, true
}

func parseExportFormat(raw string) (string, error) {
	format := strings.ToLower(strings.TrimSpace(raw))
	if format != "json" && format != "csv" {
		return "", fmt.Errorf("format must be json or csv")
	}
	return format, nil
}

func appendIfChanged(items []string, value string) []string {
	if value == "" {
		return items
	}
	n := len(items)
	if n > 0 && items[n-1] == value {
		return items
	}
	return append(items, value)
}

func (s *Server) withLogging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		log.Printf("%s %s %s", r.Method, r.URL.Path, time.Since(start).Round(time.Millisecond))
	})
}

func providerNames(providers map[string]services.MusicService) []string {
	names := make([]string, 0, len(providers))
	for name := range providers {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func randomState(n int) (string, error) {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("read random bytes: %w", err)
	}
	return hex.EncodeToString(buf), nil
}

func isSecureRequest(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	if strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https") {
		return true
	}
	return false
}

func Run(ctx context.Context, cfg Config, srv *Server) error {
	httpServer := &http.Server{
		Addr:    cfg.Addr,
		Handler: srv.Handler(),
	}

	errCh := make(chan error, 1)
	go func() {
		log.Printf("Dumpify listening on %s", cfg.Addr)
		errCh <- httpServer.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return httpServer.Shutdown(shutdownCtx)
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}
