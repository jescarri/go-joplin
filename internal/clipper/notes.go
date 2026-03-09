package clipper

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jescarri/go-joplin/internal/models"
)

func (s *Server) handleListNotes(w http.ResponseWriter, r *http.Request) {
	p := parsePagination(r)
	notes, hasMore, err := s.db.ListNotes(p.OrderBy, p.OrderDir, p.Limit, p.offset())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if notes == nil {
		notes = []*models.Note{}
	}
	writePaginated(w, notes, hasMore, p.Fields)
}

func (s *Server) handleGetNote(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	note, err := s.db.GetNote(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if note == nil {
		writeError(w, http.StatusNotFound, "not found")
		return
	}

	p := parsePagination(r)
	filterSingleItem(w, note, p.Fields)
}

// createNoteRequest is the body for POST /notes; note fields plus optional tag_ids.
type createNoteRequest struct {
	models.Note
	TagIDs []string `json:"tag_ids"`
}

func (s *Server) handleCreateNote(w http.ResponseWriter, r *http.Request) {
	var req createNoteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	note := &req.Note

	if s.policy != nil {
		var folderTitle string
		if note.ParentID != "" {
			if f, _ := s.db.GetFolder(note.ParentID); f != nil {
				folderTitle = f.Title
			}
		}
		if !s.policy.CanCreateNoteInFolderOrEmpty(note.ParentID, folderTitle) {
			writeError(w, http.StatusForbidden, "folder is read-only: create notes only in writable folders")
			return
		}
	} else {
		writeError(w, http.StatusForbidden, "folder is read-only: configure GOJOPLIN_MCP_ALLOW_FOLDERS to allow mutations")
		return
	}

	// Validate tag_ids against policy
	for _, tagID := range req.TagIDs {
		if tagID == "" {
			continue
		}
		tag, _ := s.db.GetTag(tagID)
		if tag == nil {
			writeError(w, http.StatusBadRequest, "tag not found: "+tagID)
			return
		}
		if !s.policy.CanAttachTag(tag.ID, tag.Title) {
			writeError(w, http.StatusForbidden, "tag "+tag.Title+" not in writable list")
			return
		}
	}

	// Handle body_html conversion
	if bodyHTML := getBodyHTML(r); bodyHTML != "" {
		md, err := htmlToMarkdown(bodyHTML)
		if err != nil {
			writeError(w, http.StatusBadRequest, "HTML conversion failed: "+err.Error())
			return
		}
		note.Body = md
	}

	if err := s.db.CreateNote(note); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Assign tags if tag_ids provided
	for _, tagID := range req.TagIDs {
		if tagID == "" {
			continue
		}
		if err := s.db.AddNoteTag(note.ID, tagID); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to add tag: "+err.Error())
			return
		}
	}

	s.triggerSync()
	writeJSON(w, http.StatusOK, note)
}

func (s *Server) handleUpdateNote(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	existing, err := s.db.GetNote(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if existing == nil {
		writeError(w, http.StatusNotFound, "not found")
		return
	}

	if s.policy != nil {
		var folderTitle string
		if existing.ParentID != "" {
			if f, _ := s.db.GetFolder(existing.ParentID); f != nil {
				folderTitle = f.Title
			}
		}
		if !s.policy.CanUpdateNoteInFolder(existing.ParentID, folderTitle) {
			writeError(w, http.StatusForbidden, "note's folder is read-only")
			return
		}
	} else {
		writeError(w, http.StatusForbidden, "folder is read-only: configure GOJOPLIN_MCP_ALLOW_FOLDERS")
		return
	}

	// Decode the update fields
	var updates map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&updates); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}

	// Apply updates to existing note
	if v, ok := updates["title"]; ok {
		existing.Title, _ = v.(string)
	}
	if v, ok := updates["body"]; ok {
		existing.Body, _ = v.(string)
	}
	if v, ok := updates["body_html"]; ok {
		if html, ok := v.(string); ok && html != "" {
			md, err := htmlToMarkdown(html)
			if err != nil {
				writeError(w, http.StatusBadRequest, "HTML conversion failed: "+err.Error())
				return
			}
			existing.Body = md
		}
	}
	if v, ok := updates["parent_id"]; ok {
		existing.ParentID, _ = v.(string)
	}
	if v, ok := updates["is_todo"]; ok {
		if n, ok := v.(float64); ok {
			existing.IsTodo = int(n)
		}
	}
	if v, ok := updates["todo_due"]; ok {
		if n, ok := v.(float64); ok {
			existing.TodoDue = int64(n)
		}
	}
	if v, ok := updates["todo_completed"]; ok {
		if n, ok := v.(float64); ok {
			existing.TodoCompleted = int64(n)
		}
	}
	if v, ok := updates["source_url"]; ok {
		existing.SourceURL, _ = v.(string)
	}
	if v, ok := updates["author"]; ok {
		existing.Author, _ = v.(string)
	}
	if v, ok := updates["markup_language"]; ok {
		if n, ok := v.(float64); ok {
			existing.MarkupLanguage = int(n)
		}
	}
	if v, ok := updates["order"]; ok {
		if n, ok := v.(float64); ok {
			existing.Order = int64(n)
		}
	}

	if err := s.db.UpdateNote(existing); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	s.triggerSync()
	writeJSON(w, http.StatusOK, existing)
}

func (s *Server) handleDeleteNote(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	note, _ := s.db.GetNote(id)
	if note != nil && s.policy != nil {
		var folderTitle string
		if note.ParentID != "" {
			if f, _ := s.db.GetFolder(note.ParentID); f != nil {
				folderTitle = f.Title
			}
		}
		if !s.policy.CanUpdateNoteInFolder(note.ParentID, folderTitle) {
			writeError(w, http.StatusForbidden, "note's folder is read-only")
			return
		}
	} else if note != nil && s.policy == nil {
		writeError(w, http.StatusForbidden, "folder is read-only: configure GOJOPLIN_MCP_ALLOW_FOLDERS")
		return
	}
	if err := s.db.DeleteNote(id); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.triggerSync()
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleGetNoteTags(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	tags, err := s.db.GetNoteTagsByNote(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if tags == nil {
		tags = []*models.Tag{}
	}

	p := parsePagination(r)
	writePaginated(w, tags, false, p.Fields)
}

func (s *Server) handleGetNoteResources(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	resources, err := s.db.GetResourcesByNote(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if resources == nil {
		resources = []*models.Resource{}
	}

	p := parsePagination(r)
	writePaginated(w, resources, false, p.Fields)
}

// getBodyHTML extracts body_html from the request body if present.
func getBodyHTML(r *http.Request) string {
	// This is handled in the JSON decode, but we provide a helper
	// for cases where it comes as a separate field
	return ""
}
