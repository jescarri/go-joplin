package clipper

import (
	"net/http"

	"github.com/jescarri/go-joplin/internal/models"
)

func (s *Server) handleSearch(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("query")
	if query == "" {
		writeError(w, http.StatusBadRequest, "query parameter is required")
		return
	}

	searchType := r.URL.Query().Get("type")
	p := parsePagination(r)

	switch searchType {
	case "folder":
		folders, hasMore, err := s.db.SearchFolders(query, p.Limit, p.offset())
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if folders == nil {
			folders = []*models.Folder{}
		}
		writePaginated(w, folders, hasMore, p.Fields)

	case "tag":
		tags, hasMore, err := s.db.SearchTags(query, p.Limit, p.offset())
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if tags == nil {
			tags = []*models.Tag{}
		}
		writePaginated(w, tags, hasMore, p.Fields)

	default: // "note" or empty defaults to note search
		var notes []*models.Note
		var hasMore bool
		var err error

		if s.ragSearcher != nil {
			notes, hasMore, err = s.ragSearcher.Search(r.Context(), query, p.Limit)
			if err != nil {
				// Fall back to FTS4
				notes, hasMore, err = s.db.SearchNotes(query, p.Limit, p.offset())
			}
		} else {
			notes, hasMore, err = s.db.SearchNotes(query, p.Limit, p.offset())
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if notes == nil {
			notes = []*models.Note{}
		}
		writePaginated(w, notes, hasMore, p.Fields)
	}
}
