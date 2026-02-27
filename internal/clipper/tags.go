package clipper

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jescarri/go-joplin/internal/models"
)

func (s *Server) handleListTags(w http.ResponseWriter, r *http.Request) {
	p := parsePagination(r)
	tags, hasMore, err := s.db.ListTags(p.OrderBy, p.OrderDir, p.Limit, p.offset())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if tags == nil {
		tags = []*models.Tag{}
	}
	writePaginated(w, tags, hasMore, p.Fields)
}

func (s *Server) handleGetTag(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	tag, err := s.db.GetTag(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if tag == nil {
		writeError(w, http.StatusNotFound, "not found")
		return
	}

	p := parsePagination(r)
	filterSingleItem(w, tag, p.Fields)
}

func (s *Server) handleCreateTag(w http.ResponseWriter, r *http.Request) {
	var tag models.Tag
	if err := json.NewDecoder(r.Body).Decode(&tag); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}

	if err := s.db.CreateTag(&tag); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	s.triggerSync()
	writeJSON(w, http.StatusOK, tag)
}

func (s *Server) handleUpdateTag(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	existing, err := s.db.GetTag(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if existing == nil {
		writeError(w, http.StatusNotFound, "not found")
		return
	}

	var updates map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&updates); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}

	if v, ok := updates["title"]; ok {
		existing.Title, _ = v.(string)
	}
	if v, ok := updates["parent_id"]; ok {
		existing.ParentID, _ = v.(string)
	}

	if err := s.db.UpdateTag(existing); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	s.triggerSync()
	writeJSON(w, http.StatusOK, existing)
}

func (s *Server) handleDeleteTag(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := s.db.DeleteTag(id); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.triggerSync()
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleGetTagNotes(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	p := parsePagination(r)

	notes, hasMore, err := s.db.GetNotesByTag(id, p.Limit, p.offset())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if notes == nil {
		notes = []*models.Note{}
	}
	writePaginated(w, notes, hasMore, p.Fields)
}

func (s *Server) handleAddTagNote(w http.ResponseWriter, r *http.Request) {
	tagID := chi.URLParam(r, "id")

	var body struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}

	if body.ID == "" {
		writeError(w, http.StatusBadRequest, "note id is required")
		return
	}

	if err := s.db.AddNoteTag(body.ID, tagID); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	s.triggerSync()
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleRemoveTagNote(w http.ResponseWriter, r *http.Request) {
	tagID := chi.URLParam(r, "id")
	noteID := chi.URLParam(r, "noteId")

	if err := s.db.RemoveNoteTag(noteID, tagID); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	s.triggerSync()
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
