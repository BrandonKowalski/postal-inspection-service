package web

import (
	"embed"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"postal-inspection-service/internal/db"
)

//go:embed templates/*.html
var templateFS embed.FS

type Server struct {
	db   *db.DB
	port int
	tmpl *template.Template
}

func NewServer(database *db.DB, port int) (*Server, error) {
	funcMap := template.FuncMap{
		"formatTime": func(t time.Time) string {
			return t.Format("2006-01-02 15:04:05")
		},
		"actionLabel": func(action string) string {
			switch action {
			case db.ActionBlockedSender:
				return "Blocked Sender"
			case db.ActionDeletedEmail:
				return "Deleted Email"
			case db.ActionUnblockedSender:
				return "Unblocked Sender"
			case db.ActionTransactionalOnlySender:
				return "Transactional Only"
			case db.ActionRemovedTransactionalOnly:
				return "Removed Trans. Only"
			case db.ActionDeletedMarketing:
				return "Deleted Marketing"
			default:
				return action
			}
		},
		"actionClass": func(action string) string {
			switch action {
			case db.ActionBlockedSender:
				return "action-blocked"
			case db.ActionDeletedEmail:
				return "action-deleted"
			case db.ActionUnblockedSender:
				return "action-unblocked"
			case db.ActionTransactionalOnlySender:
				return "action-transactional"
			case db.ActionRemovedTransactionalOnly:
				return "action-unblocked"
			case db.ActionDeletedMarketing:
				return "action-marketing"
			default:
				return ""
			}
		},
	}

	tmpl, err := template.New("").Funcs(funcMap).ParseFS(templateFS, "templates/*.html")
	if err != nil {
		return nil, fmt.Errorf("failed to parse templates: %w", err)
	}

	return &Server{
		db:   database,
		port: port,
		tmpl: tmpl,
	}, nil
}

func (s *Server) Start() error {
	mux := http.NewServeMux()

	mux.HandleFunc("/", s.handleIndex)
	mux.HandleFunc("/blocked", s.handleBlocked)
	mux.HandleFunc("/blocked/add", s.handleAddBlocked)
	mux.HandleFunc("/blocked/delete", s.handleDeleteBlocked)
	mux.HandleFunc("/transactional", s.handleTransactional)
	mux.HandleFunc("/transactional/add", s.handleAddTransactional)
	mux.HandleFunc("/transactional/delete", s.handleDeleteTransactional)
	mux.HandleFunc("/log", s.handleLog)
	mux.HandleFunc("/log/detail", s.handleLogDetail)

	addr := fmt.Sprintf(":%d", s.port)
	log.Printf("Starting web server on %s", addr)
	return http.ListenAndServe(addr, mux)
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	stats, err := s.db.GetStats()
	if err != nil {
		http.Error(w, "Failed to load stats", http.StatusInternalServerError)
		log.Printf("Error loading stats: %v", err)
		return
	}

	data := map[string]any{
		"Title": "Dashboard",
		"Stats": stats,
	}

	if err := s.tmpl.ExecuteTemplate(w, "index.html", data); err != nil {
		log.Printf("Error rendering template: %v", err)
	}
}

func (s *Server) handleBlocked(w http.ResponseWriter, r *http.Request) {
	senders, err := s.db.GetBlockedSenders()
	if err != nil {
		http.Error(w, "Failed to load blocked senders", http.StatusInternalServerError)
		log.Printf("Error loading blocked senders: %v", err)
		return
	}

	data := map[string]any{
		"Title":   "Blocked Senders",
		"Senders": senders,
	}

	if err := s.tmpl.ExecuteTemplate(w, "blocked.html", data); err != nil {
		log.Printf("Error rendering template: %v", err)
	}
}

func (s *Server) handleAddBlocked(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	email := strings.ToLower(strings.TrimSpace(r.FormValue("email")))
	reason := strings.TrimSpace(r.FormValue("reason"))

	if email == "" {
		http.Error(w, "Email is required", http.StatusBadRequest)
		return
	}

	if reason == "" {
		reason = "Manually added via web UI"
	}

	if err := s.db.AddBlockedSender(email, reason); err != nil {
		http.Error(w, "Failed to add sender", http.StatusInternalServerError)
		log.Printf("Error adding blocked sender: %v", err)
		return
	}

	s.db.LogAction(
		db.ActionBlockedSender,
		email,
		"",
		"",
		"Manually added via web UI",
	)

	log.Printf("Added sender to blocked list via web UI: %s", email)
	http.Redirect(w, r, "/blocked", http.StatusSeeOther)
}

func (s *Server) handleDeleteBlocked(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	idStr := r.URL.Query().Get("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return
	}

	sender, err := s.db.GetBlockedSenderByID(id)
	if err != nil {
		http.Error(w, "Failed to find sender", http.StatusInternalServerError)
		return
	}
	if sender == nil {
		http.Error(w, "Sender not found", http.StatusNotFound)
		return
	}

	if err := s.db.RemoveBlockedSender(id); err != nil {
		http.Error(w, "Failed to remove sender", http.StatusInternalServerError)
		log.Printf("Error removing blocked sender: %v", err)
		return
	}

	s.db.LogAction(
		db.ActionUnblockedSender,
		sender.Email,
		"",
		"",
		"Removed from blocked list via web UI",
	)

	log.Printf("Removed sender from blocked list: %s", sender.Email)
	http.Redirect(w, r, "/blocked", http.StatusSeeOther)
}

func (s *Server) handleTransactional(w http.ResponseWriter, r *http.Request) {
	senders, err := s.db.GetTransactionalOnlySenders()
	if err != nil {
		http.Error(w, "Failed to load transactional-only senders", http.StatusInternalServerError)
		log.Printf("Error loading transactional-only senders: %v", err)
		return
	}

	data := map[string]any{
		"Title":   "Transactional Only Senders",
		"Senders": senders,
	}

	if err := s.tmpl.ExecuteTemplate(w, "transactional.html", data); err != nil {
		log.Printf("Error rendering template: %v", err)
	}
}

func (s *Server) handleAddTransactional(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	email := strings.ToLower(strings.TrimSpace(r.FormValue("email")))
	reason := strings.TrimSpace(r.FormValue("reason"))

	if email == "" {
		http.Error(w, "Email is required", http.StatusBadRequest)
		return
	}

	if reason == "" {
		reason = "Manually added via web UI"
	}

	if err := s.db.AddTransactionalOnlySender(email, reason); err != nil {
		http.Error(w, "Failed to add sender", http.StatusInternalServerError)
		log.Printf("Error adding transactional-only sender: %v", err)
		return
	}

	s.db.LogAction(
		db.ActionTransactionalOnlySender,
		email,
		"",
		"",
		"Manually added via web UI - marketing emails will be deleted",
	)

	log.Printf("Added sender to transactional-only list via web UI: %s", email)
	http.Redirect(w, r, "/transactional", http.StatusSeeOther)
}

func (s *Server) handleDeleteTransactional(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	idStr := r.URL.Query().Get("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return
	}

	sender, err := s.db.GetTransactionalOnlySenderByID(id)
	if err != nil {
		http.Error(w, "Failed to find sender", http.StatusInternalServerError)
		return
	}
	if sender == nil {
		http.Error(w, "Sender not found", http.StatusNotFound)
		return
	}

	if err := s.db.RemoveTransactionalOnlySender(id); err != nil {
		http.Error(w, "Failed to remove sender", http.StatusInternalServerError)
		log.Printf("Error removing transactional-only sender: %v", err)
		return
	}

	s.db.LogAction(
		db.ActionRemovedTransactionalOnly,
		sender.Email,
		"",
		"",
		"Removed from transactional-only list via web UI",
	)

	log.Printf("Removed sender from transactional-only list: %s", sender.Email)
	http.Redirect(w, r, "/transactional", http.StatusSeeOther)
}

func (s *Server) handleLog(w http.ResponseWriter, r *http.Request) {
	page := 1
	if pageStr := r.URL.Query().Get("page"); pageStr != "" {
		if p, err := strconv.Atoi(pageStr); err == nil && p > 0 {
			page = p
		}
	}

	limit := 50
	offset := (page - 1) * limit

	logs, err := s.db.GetActionLogs(limit, offset)
	if err != nil {
		http.Error(w, "Failed to load action logs", http.StatusInternalServerError)
		log.Printf("Error loading action logs: %v", err)
		return
	}

	totalCount, err := s.db.GetActionLogCount()
	if err != nil {
		http.Error(w, "Failed to load action log count", http.StatusInternalServerError)
		return
	}

	totalPages := (totalCount + limit - 1) / limit
	if totalPages == 0 {
		totalPages = 1
	}

	data := map[string]any{
		"Title":       "Action Log",
		"Logs":        logs,
		"CurrentPage": page,
		"TotalPages":  totalPages,
		"HasPrev":     page > 1,
		"HasNext":     page < totalPages,
		"PrevPage":    page - 1,
		"NextPage":    page + 1,
	}

	if err := s.tmpl.ExecuteTemplate(w, "log.html", data); err != nil {
		log.Printf("Error rendering template: %v", err)
	}
}

func (s *Server) handleLogDetail(w http.ResponseWriter, r *http.Request) {
	idStr := r.URL.Query().Get("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return
	}

	actionLog, err := s.db.GetActionLogByID(id)
	if err != nil {
		http.Error(w, "Failed to load action log", http.StatusInternalServerError)
		log.Printf("Error loading action log: %v", err)
		return
	}
	if actionLog == nil {
		http.Error(w, "Action log not found", http.StatusNotFound)
		return
	}

	var emailDetail *db.EmailDetail
	if actionLog.EmailDetailID != nil {
		emailDetail, err = s.db.GetEmailDetail(*actionLog.EmailDetailID)
		if err != nil {
			log.Printf("Error loading email detail: %v", err)
		}
	}

	data := map[string]any{
		"Title":       "Action Detail",
		"Log":         actionLog,
		"EmailDetail": emailDetail,
	}

	if err := s.tmpl.ExecuteTemplate(w, "log_detail.html", data); err != nil {
		log.Printf("Error rendering template: %v", err)
	}
}
