package clipper

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jescarri/go-joplin/internal/models"
)

func (s *Server) handleListFolders(w http.ResponseWriter, r *http.Request) {
	// Check for tree view
	if r.URL.Query().Get("as_tree") == "1" {
		tree, err := s.db.FolderTree()
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if tree == nil {
			tree = []*models.Folder{}
		}
		writeJSON(w, http.StatusOK, tree)
		return
	}

	p := parsePagination(r)
	folders, hasMore, err := s.db.ListFolders(p.OrderBy, p.OrderDir, p.Limit, p.offset())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if folders == nil {
		folders = []*models.Folder{}
	}
	writePaginated(w, folders, hasMore, p.Fields)
}

func (s *Server) handleGetFolder(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	folder, err := s.db.GetFolder(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if folder == nil {
		writeError(w, http.StatusNotFound, "not found")
		return
	}

	p := parsePagination(r)
	filterSingleItem(w, folder, p.Fields)
}

func (s *Server) handleCreateFolder(w http.ResponseWriter, r *http.Request) {
	if s.policy == nil || !s.policy.CanCreateFolder() {
		writeError(w, http.StatusForbidden, "folder creation disabled: set GOJOPLIN_MCP_ALLOW_CREATE_FOLDER=true")
		return
	}
	var folder models.Folder
	if err := json.NewDecoder(r.Body).Decode(&folder); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}

	if err := s.db.CreateFolder(&folder); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	s.triggerSync()
	writeJSON(w, http.StatusOK, folder)
}

func (s *Server) handleUpdateFolder(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	existing, err := s.db.GetFolder(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if existing == nil {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	if s.policy == nil || !s.policy.CanUpdateNoteInFolder(existing.ID, existing.Title) {
		writeError(w, http.StatusForbidden, "folder is read-only")
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
	if v, ok := updates["icon"]; ok {
		existing.Icon, _ = v.(string)
	}

	if err := s.db.UpdateFolder(existing); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	s.triggerSync()
	writeJSON(w, http.StatusOK, existing)
}

func (s *Server) handleDeleteFolder(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	folder, _ := s.db.GetFolder(id)
	if folder != nil && (s.policy == nil || !s.policy.CanUpdateNoteInFolder(folder.ID, folder.Title)) {
		writeError(w, http.StatusForbidden, "folder is read-only")
		return
	}
	if err := s.db.DeleteFolder(id); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.triggerSync()
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleGetFolderNotes(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	p := parsePagination(r)

	notes, hasMore, err := s.db.ListNotesByFolder(id, p.OrderBy, p.OrderDir, p.Limit, p.offset())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if notes == nil {
		notes = []*models.Note{}
	}
	writePaginated(w, notes, hasMore, p.Fields)
}
