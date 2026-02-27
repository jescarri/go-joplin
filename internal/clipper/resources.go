package clipper

import (
	"encoding/json"
	"io"
	"net/http"
	"os"

	"github.com/go-chi/chi/v5"
	"github.com/jescarri/go-joplin/internal/models"
)

func (s *Server) handleListResources(w http.ResponseWriter, r *http.Request) {
	p := parsePagination(r)
	resources, hasMore, err := s.db.ListResources(p.OrderBy, p.OrderDir, p.Limit, p.offset())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if resources == nil {
		resources = []*models.Resource{}
	}
	writePaginated(w, resources, hasMore, p.Fields)
}

func (s *Server) handleGetResource(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	resource, err := s.db.GetResource(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if resource == nil {
		writeError(w, http.StatusNotFound, "not found")
		return
	}

	p := parsePagination(r)
	filterSingleItem(w, resource, p.Fields)
}

func (s *Server) handleCreateResource(w http.ResponseWriter, r *http.Request) {
	// Support multipart upload
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		// Try JSON body
		var resource models.Resource
		if err := json.NewDecoder(r.Body).Decode(&resource); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request")
			return
		}
		if err := s.db.CreateResource(&resource); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		s.triggerSync()
		writeJSON(w, http.StatusOK, resource)
		return
	}

	// Parse resource properties from form
	var resource models.Resource
	if props := r.FormValue("props"); props != "" {
		json.Unmarshal([]byte(props), &resource)
	}
	resource.Title = r.FormValue("title")
	if resource.Title == "" {
		resource.Title = "Untitled"
	}

	// Handle file upload
	file, header, err := r.FormFile("data")
	if err != nil {
		writeError(w, http.StatusBadRequest, "file upload required")
		return
	}
	defer file.Close()

	data, err := io.ReadAll(file)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "cannot read file")
		return
	}

	resource.Size = int64(len(data))
	if resource.Filename == "" {
		resource.Filename = header.Filename
	}
	if resource.Mime == "" {
		resource.Mime = http.DetectContentType(data)
	}

	if err := s.db.CreateResource(&resource); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if err := s.db.SaveResourceFile(resource.ID, data); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	s.triggerSync()
	writeJSON(w, http.StatusOK, resource)
}

func (s *Server) handleUpdateResource(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	existing, err := s.db.GetResource(id)
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
	if v, ok := updates["filename"]; ok {
		existing.Filename, _ = v.(string)
	}
	if v, ok := updates["mime"]; ok {
		existing.Mime, _ = v.(string)
	}

	// Re-use UpsertResource for update
	if err := s.db.UpsertResource(existing); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	s.triggerSync()
	writeJSON(w, http.StatusOK, existing)
}

func (s *Server) handleDeleteResource(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := s.db.DeleteResource(id); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.triggerSync()
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleGetResourceFile(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	resource, err := s.db.GetResource(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if resource == nil {
		writeError(w, http.StatusNotFound, "not found")
		return
	}

	filePath := s.db.GetResourceFile(id)
	f, err := os.Open(filePath)
	if err != nil {
		writeError(w, http.StatusNotFound, "resource file not found")
		return
	}
	defer f.Close()

	if resource.Mime != "" {
		w.Header().Set("Content-Type", resource.Mime)
	}
	if resource.Filename != "" {
		w.Header().Set("Content-Disposition", "attachment; filename=\""+resource.Filename+"\"")
	}

	io.Copy(w, f)
}
