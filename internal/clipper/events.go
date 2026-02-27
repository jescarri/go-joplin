package clipper

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/jescarri/go-joplin/internal/models"
)

func (s *Server) handleListEvents(w http.ResponseWriter, r *http.Request) {
	cursor := 0
	if c := r.URL.Query().Get("cursor"); c != "" {
		cursor, _ = strconv.Atoi(c)
	}

	p := parsePagination(r)
	events, hasMore, err := s.db.GetEvents(cursor, p.Limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if events == nil {
		events = []*models.ItemChange{}
	}
	writePaginated(w, events, hasMore, p.Fields)
}

func (s *Server) handleGetEvent(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid event ID")
		return
	}

	event, err := s.db.GetEvent(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if event == nil {
		writeError(w, http.StatusNotFound, "not found")
		return
	}

	writeJSON(w, http.StatusOK, event)
}
