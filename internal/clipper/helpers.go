package clipper

import (
	"encoding/json"
	"net/http"
	"reflect"
	"strconv"
	"strings"

	"github.com/jescarri/go-joplin/internal/models"
)

// writeJSON writes a JSON response.
func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

// writeError writes a JSON error response.
func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

// paginationParams extracts pagination parameters from a request.
type paginationParams struct {
	Page     int
	Limit    int
	OrderBy  string
	OrderDir string
	Fields   []string
}

func parsePagination(r *http.Request) paginationParams {
	p := paginationParams{
		Page:     1,
		Limit:    10,
		OrderBy:  r.URL.Query().Get("order_by"),
		OrderDir: strings.ToUpper(r.URL.Query().Get("order_dir")),
	}

	if page, err := strconv.Atoi(r.URL.Query().Get("page")); err == nil && page > 0 {
		p.Page = page
	}
	if limit, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && limit > 0 {
		if limit > 100 {
			limit = 100
		}
		p.Limit = limit
	}

	if fields := r.URL.Query().Get("fields"); fields != "" {
		p.Fields = strings.Split(fields, ",")
	}

	return p
}

func (p paginationParams) offset() int {
	return (p.Page - 1) * p.Limit
}

// writePaginated writes a paginated response.
func writePaginated(w http.ResponseWriter, items interface{}, hasMore bool, fields []string) {
	// Filter fields if requested
	if len(fields) > 0 {
		items = filterFields(items, fields)
	}

	writeJSON(w, http.StatusOK, models.PaginatedResponse{
		Items:   items,
		HasMore: hasMore,
	})
}

// filterFields filters the response to only include specified fields.
func filterFields(items interface{}, fields []string) interface{} {
	fieldSet := make(map[string]bool)
	for _, f := range fields {
		fieldSet[strings.TrimSpace(f)] = true
	}

	val := reflect.ValueOf(items)
	if val.Kind() == reflect.Ptr {
		val = val.Elem()
	}
	if val.Kind() != reflect.Slice {
		return filterSingleFields(items, fieldSet)
	}

	result := make([]map[string]interface{}, val.Len())
	for i := 0; i < val.Len(); i++ {
		result[i] = filterSingleFields(val.Index(i).Interface(), fieldSet)
	}
	return result
}

func filterSingleFields(item interface{}, fieldSet map[string]bool) map[string]interface{} {
	result := make(map[string]interface{})

	// Marshal to JSON then unmarshal to map to get field names
	data, err := json.Marshal(item)
	if err != nil {
		return result
	}

	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		return result
	}

	for key, val := range m {
		if fieldSet[key] {
			result[key] = val
		}
	}
	return result
}

// filterSingleItem filters a single item's fields for GET responses.
func filterSingleItem(w http.ResponseWriter, item interface{}, fields []string) {
	if len(fields) > 0 {
		fieldSet := make(map[string]bool)
		for _, f := range fields {
			fieldSet[strings.TrimSpace(f)] = true
		}
		writeJSON(w, http.StatusOK, filterSingleFields(item, fieldSet))
	} else {
		writeJSON(w, http.StatusOK, item)
	}
}
