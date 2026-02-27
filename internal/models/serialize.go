package models

import (
	"strconv"
	"strings"
	"time"
)

// SerializeNote converts a Note to Joplin's text format for sync.
func SerializeNote(n *Note) string {
	var b strings.Builder
	b.WriteString(n.Title)
	b.WriteString("\n\n")
	b.WriteString(n.Body)
	b.WriteString("\n\n")
	writeMetadata(&b, map[string]string{
		"id":                     n.ID,
		"parent_id":              n.ParentID,
		"created_time":           fmtTime(n.CreatedTime),
		"updated_time":           fmtTime(n.UpdatedTime),
		"is_conflict":            strconv.Itoa(n.IsConflict),
		"latitude":               fmtFloat(n.Latitude),
		"longitude":              fmtFloat(n.Longitude),
		"altitude":               fmtFloat(n.Altitude),
		"author":                 n.Author,
		"source_url":             n.SourceURL,
		"is_todo":                strconv.Itoa(n.IsTodo),
		"todo_due":               fmtTime(n.TodoDue),
		"todo_completed":         fmtTime(n.TodoCompleted),
		"source":                 n.Source,
		"source_application":     n.SourceApplication,
		"application_data":       n.ApplicationData,
		"order":                  fmtTime(n.Order),
		"user_created_time":      fmtTime(n.UserCreatedTime),
		"user_updated_time":      fmtTime(n.UserUpdatedTime),
		"encryption_cipher_text": n.EncryptionCipherText,
		"encryption_applied":     strconv.Itoa(n.EncryptionApplied),
		"markup_language":        strconv.Itoa(n.MarkupLanguage),
		"is_shared":              strconv.Itoa(n.IsShared),
		"share_id":               n.ShareID,
		"conflict_original_id":   n.ConflictOriginalID,
		"master_key_id":          n.MasterKeyID,
		"user_data":              n.UserData,
		"deleted_time":           fmtTime(n.DeletedTime),
		"type_":                  strconv.Itoa(TypeNote),
	})
	return b.String()
}

// SerializeFolder converts a Folder to Joplin's text format.
func SerializeFolder(f *Folder) string {
	var b strings.Builder
	b.WriteString(f.Title)
	b.WriteString("\n\n")
	writeMetadata(&b, map[string]string{
		"id":                     f.ID,
		"created_time":           fmtTime(f.CreatedTime),
		"updated_time":           fmtTime(f.UpdatedTime),
		"user_created_time":      fmtTime(f.UserCreatedTime),
		"user_updated_time":      fmtTime(f.UserUpdatedTime),
		"encryption_cipher_text": f.EncryptionCipherText,
		"encryption_applied":     strconv.Itoa(f.EncryptionApplied),
		"parent_id":              f.ParentID,
		"is_shared":              strconv.Itoa(f.IsShared),
		"share_id":               f.ShareID,
		"master_key_id":          f.MasterKeyID,
		"icon":                   f.Icon,
		"user_data":              f.UserData,
		"deleted_time":           fmtTime(f.DeletedTime),
		"type_":                  strconv.Itoa(TypeFolder),
	})
	return b.String()
}

// SerializeTag converts a Tag to Joplin's text format.
func SerializeTag(t *Tag) string {
	var b strings.Builder
	b.WriteString(t.Title)
	b.WriteString("\n\n")
	writeMetadata(&b, map[string]string{
		"id":                     t.ID,
		"created_time":           fmtTime(t.CreatedTime),
		"updated_time":           fmtTime(t.UpdatedTime),
		"user_created_time":      fmtTime(t.UserCreatedTime),
		"user_updated_time":      fmtTime(t.UserUpdatedTime),
		"encryption_cipher_text": t.EncryptionCipherText,
		"encryption_applied":     strconv.Itoa(t.EncryptionApplied),
		"is_shared":              strconv.Itoa(t.IsShared),
		"parent_id":              t.ParentID,
		"user_data":              t.UserData,
		"type_":                  strconv.Itoa(TypeTag),
	})
	return b.String()
}

// SerializeNoteTag converts a NoteTag to Joplin's text format.
func SerializeNoteTag(nt *NoteTag) string {
	var b strings.Builder
	writeMetadata(&b, map[string]string{
		"id":                     nt.ID,
		"note_id":                nt.NoteID,
		"tag_id":                 nt.TagID,
		"created_time":           fmtTime(nt.CreatedTime),
		"updated_time":           fmtTime(nt.UpdatedTime),
		"user_created_time":      fmtTime(nt.UserCreatedTime),
		"user_updated_time":      fmtTime(nt.UserUpdatedTime),
		"encryption_cipher_text": nt.EncryptionCipherText,
		"encryption_applied":     strconv.Itoa(nt.EncryptionApplied),
		"is_shared":              strconv.Itoa(nt.IsShared),
		"type_":                  strconv.Itoa(TypeNoteTag),
	})
	return b.String()
}

// SerializeResource converts a Resource to Joplin's text format (metadata only).
func SerializeResource(r *Resource) string {
	var b strings.Builder
	b.WriteString(r.Title)
	b.WriteString("\n\n")
	writeMetadata(&b, map[string]string{
		"id":                          r.ID,
		"mime":                        r.Mime,
		"filename":                    r.Filename,
		"created_time":                fmtTime(r.CreatedTime),
		"updated_time":                fmtTime(r.UpdatedTime),
		"user_created_time":           fmtTime(r.UserCreatedTime),
		"user_updated_time":           fmtTime(r.UserUpdatedTime),
		"file_extension":              r.FileExtension,
		"encryption_cipher_text":      r.EncryptionCipherText,
		"encryption_applied":          strconv.Itoa(r.EncryptionApplied),
		"encryption_blob_encrypted":   strconv.Itoa(r.EncryptionBlobEncrypted),
		"size":                        strconv.FormatInt(r.Size, 10),
		"is_shared":                   strconv.Itoa(r.IsShared),
		"share_id":                    r.ShareID,
		"master_key_id":               r.MasterKeyID,
		"user_data":                   r.UserData,
		"blob_updated_time":           fmtTime(r.BlobUpdatedTime),
		"type_":                       strconv.Itoa(TypeResource),
	})
	return b.String()
}

// Deserialize parses Joplin's text format and returns the item type and a map of fields.
func Deserialize(content string) (int, map[string]string) {
	fields := make(map[string]string)

	// Trim trailing whitespace so a trailing \n doesn't produce
	// an empty element that breaks the backward metadata scan.
	content = strings.TrimRight(content, "\n\r ")

	// Find the metadata section: last block of "key: value" lines
	lines := strings.Split(content, "\n")

	// Find where metadata starts by scanning from the end
	metaStart := len(lines)
	for i := len(lines) - 1; i >= 0; i-- {
		line := lines[i]
		if line == "" {
			break
		}
		if idx := strings.Index(line, ": "); idx > 0 {
			metaStart = i
		} else {
			break
		}
	}

	// Parse metadata
	for i := metaStart; i < len(lines); i++ {
		line := lines[i]
		if idx := strings.Index(line, ": "); idx > 0 {
			key := line[:idx]
			value := unescapeMetaValue(line[idx+2:])
			fields[key] = value
		}
	}

	// Get type
	itemType := 0
	if t, ok := fields["type_"]; ok {
		itemType, _ = strconv.Atoi(t)
	}

	// Extract title and body
	if metaStart > 0 {
		// Title is the first line
		fields["title"] = lines[0]

		// Body is everything between title and metadata, trimmed
		if metaStart > 2 {
			bodyEnd := metaStart - 1 // skip blank line before metadata
			if bodyEnd > 1 {
				body := strings.Join(lines[2:bodyEnd], "\n")
				fields["body"] = body
			}
		}
	}

	return itemType, fields
}

// DeserializeNote parses text content into a Note.
func DeserializeNote(content string) *Note {
	_, fields := Deserialize(content)
	n := &Note{}
	n.ID = fields["id"]
	n.ParentID = fields["parent_id"]
	n.Title = fields["title"]
	n.Body = fields["body"]
	n.CreatedTime = parseTime(fields["created_time"])
	n.UpdatedTime = parseTime(fields["updated_time"])
	n.IsConflict = parseInt(fields["is_conflict"])
	n.Latitude, _ = strconv.ParseFloat(fields["latitude"], 64)
	n.Longitude, _ = strconv.ParseFloat(fields["longitude"], 64)
	n.Altitude, _ = strconv.ParseFloat(fields["altitude"], 64)
	n.Author = fields["author"]
	n.SourceURL = fields["source_url"]
	n.IsTodo = parseInt(fields["is_todo"])
	n.TodoDue = parseTime(fields["todo_due"])
	n.TodoCompleted = parseTime(fields["todo_completed"])
	n.Source = fields["source"]
	n.SourceApplication = fields["source_application"]
	n.ApplicationData = fields["application_data"]
	n.Order = parseTime(fields["order"])
	n.UserCreatedTime = parseTime(fields["user_created_time"])
	n.UserUpdatedTime = parseTime(fields["user_updated_time"])
	n.EncryptionCipherText = fields["encryption_cipher_text"]
	n.EncryptionApplied = parseInt(fields["encryption_applied"])
	n.MarkupLanguage = parseInt(fields["markup_language"])
	n.IsShared = parseInt(fields["is_shared"])
	n.ShareID = fields["share_id"]
	n.ConflictOriginalID = fields["conflict_original_id"]
	n.MasterKeyID = fields["master_key_id"]
	n.UserData = fields["user_data"]
	n.DeletedTime = parseTime(fields["deleted_time"])
	return n
}

// DeserializeFolder parses text content into a Folder.
func DeserializeFolder(content string) *Folder {
	_, fields := Deserialize(content)
	f := &Folder{}
	f.ID = fields["id"]
	f.Title = fields["title"]
	f.CreatedTime = parseTime(fields["created_time"])
	f.UpdatedTime = parseTime(fields["updated_time"])
	f.UserCreatedTime = parseTime(fields["user_created_time"])
	f.UserUpdatedTime = parseTime(fields["user_updated_time"])
	f.EncryptionCipherText = fields["encryption_cipher_text"]
	f.EncryptionApplied = parseInt(fields["encryption_applied"])
	f.ParentID = fields["parent_id"]
	f.IsShared = parseInt(fields["is_shared"])
	f.ShareID = fields["share_id"]
	f.MasterKeyID = fields["master_key_id"]
	f.Icon = fields["icon"]
	f.UserData = fields["user_data"]
	f.DeletedTime = parseTime(fields["deleted_time"])
	return f
}

// DeserializeTag parses text content into a Tag.
func DeserializeTag(content string) *Tag {
	_, fields := Deserialize(content)
	t := &Tag{}
	t.ID = fields["id"]
	t.Title = fields["title"]
	t.CreatedTime = parseTime(fields["created_time"])
	t.UpdatedTime = parseTime(fields["updated_time"])
	t.UserCreatedTime = parseTime(fields["user_created_time"])
	t.UserUpdatedTime = parseTime(fields["user_updated_time"])
	t.EncryptionCipherText = fields["encryption_cipher_text"]
	t.EncryptionApplied = parseInt(fields["encryption_applied"])
	t.IsShared = parseInt(fields["is_shared"])
	t.ParentID = fields["parent_id"]
	t.UserData = fields["user_data"]
	return t
}

// DeserializeNoteTag parses text content into a NoteTag.
func DeserializeNoteTag(content string) *NoteTag {
	_, fields := Deserialize(content)
	nt := &NoteTag{}
	nt.ID = fields["id"]
	nt.NoteID = fields["note_id"]
	nt.TagID = fields["tag_id"]
	nt.CreatedTime = parseTime(fields["created_time"])
	nt.UpdatedTime = parseTime(fields["updated_time"])
	nt.UserCreatedTime = parseTime(fields["user_created_time"])
	nt.UserUpdatedTime = parseTime(fields["user_updated_time"])
	nt.EncryptionCipherText = fields["encryption_cipher_text"]
	nt.EncryptionApplied = parseInt(fields["encryption_applied"])
	nt.IsShared = parseInt(fields["is_shared"])
	return nt
}

// DeserializeResource parses text content into a Resource.
func DeserializeResource(content string) *Resource {
	_, fields := Deserialize(content)
	r := &Resource{}
	r.ID = fields["id"]
	r.Title = fields["title"]
	r.Mime = fields["mime"]
	r.Filename = fields["filename"]
	r.CreatedTime = parseTime(fields["created_time"])
	r.UpdatedTime = parseTime(fields["updated_time"])
	r.UserCreatedTime = parseTime(fields["user_created_time"])
	r.UserUpdatedTime = parseTime(fields["user_updated_time"])
	r.FileExtension = fields["file_extension"]
	r.EncryptionCipherText = fields["encryption_cipher_text"]
	r.EncryptionApplied = parseInt(fields["encryption_applied"])
	r.EncryptionBlobEncrypted = parseInt(fields["encryption_blob_encrypted"])
	r.Size = parseInt64(fields["size"])
	r.IsShared = parseInt(fields["is_shared"])
	r.ShareID = fields["share_id"]
	r.MasterKeyID = fields["master_key_id"]
	r.UserData = fields["user_data"]
	r.BlobUpdatedTime = parseTime(fields["blob_updated_time"])
	return r
}

// SerializeEncryptedEnvelope serializes a reduced item for sync (encrypted case): empty title/body + metadata only.
// Used when uploading an encrypted item: only keep-keys and encryption fields are included.
func SerializeEncryptedEnvelope(meta map[string]string) string {
	var b strings.Builder
	b.WriteString("\n\n\n\n") // empty title, empty body, blank line before metadata
	writeMetadata(&b, meta)
	return b.String()
}

// SerializeMasterKey converts a MasterKey to Joplin's text format.
func SerializeMasterKey(mk *MasterKey) string {
	var b strings.Builder
	writeMetadata(&b, map[string]string{
		"id":                 mk.ID,
		"created_time":       fmtTime(mk.CreatedTime),
		"updated_time":       fmtTime(mk.UpdatedTime),
		"source_application": mk.SourceApplication,
		"encryption_method":  strconv.Itoa(mk.EncryptionMethod),
		"checksum":           mk.Checksum,
		"content":            mk.Content,
		"type_":              strconv.Itoa(TypeMasterKey),
	})
	return b.String()
}

// DeserializeMasterKey parses text content into a MasterKey.
func DeserializeMasterKey(content string) *MasterKey {
	_, fields := Deserialize(content)
	mk := &MasterKey{}
	mk.ID = fields["id"]
	mk.CreatedTime = parseTime(fields["created_time"])
	mk.UpdatedTime = parseTime(fields["updated_time"])
	mk.SourceApplication = fields["source_application"]
	mk.EncryptionMethod = parseInt(fields["encryption_method"])
	mk.Checksum = fields["checksum"]
	mk.Content = fields["content"]
	return mk
}

// Metadata key order for serialization (matching Joplin's order).
var metadataKeyOrder = []string{
	"id", "parent_id", "note_id", "tag_id",
	"created_time", "updated_time",
	"is_conflict", "latitude", "longitude", "altitude",
	"author", "source_url", "is_todo", "todo_due", "todo_completed",
	"source", "source_application", "application_data", "order",
	"user_created_time", "user_updated_time",
	"encryption_cipher_text", "encryption_applied",
	"markup_language", "is_shared", "share_id", "conflict_original_id",
	"master_key_id", "icon", "user_data", "deleted_time",
	"mime", "filename", "file_extension",
	"encryption_blob_encrypted", "size", "blob_updated_time",
	"encryption_method", "checksum", "content",
	"type_",
}

// escapeMetaValue escapes newlines in metadata values per Joplin format (\n -> \\n, \r -> \\r).
func escapeMetaValue(val string) string {
	val = strings.ReplaceAll(val, "\\", "\\\\")
	val = strings.ReplaceAll(val, "\n", "\\n")
	val = strings.ReplaceAll(val, "\r", "\\r")
	return val
}

// unescapeMetaValue reverses escapeMetaValue when parsing metadata (\\n -> \n, \\r -> \r).
func unescapeMetaValue(val string) string {
	var b strings.Builder
	for i := 0; i < len(val); i++ {
		if val[i] == '\\' && i+1 < len(val) {
			switch val[i+1] {
			case 'n':
				b.WriteByte('\n')
				i++
				continue
			case 'r':
				b.WriteByte('\r')
				i++
				continue
			case '\\':
				b.WriteByte('\\')
				i++
				continue
			}
		}
		b.WriteByte(val[i])
	}
	return b.String()
}

func writeMetadata(b *strings.Builder, meta map[string]string) {
	var lines []string
	for _, key := range metadataKeyOrder {
		if val, ok := meta[key]; ok {
			lines = append(lines, key+": "+escapeMetaValue(val))
		}
	}
	if len(lines) > 0 {
		b.WriteString(strings.Join(lines, "\n"))
	}
}

func fmtTime(t int64) string {
	return FmtTimeForSync(t)
}

// FmtTimeForSync formats a timestamp as ISO 8601 UTC for sync (used by push and serialization).
func FmtTimeForSync(t int64) string {
	if t == 0 {
		return ""
	}
	return time.Unix(0, t*int64(time.Millisecond)).UTC().Format("2006-01-02T15:04:05.000Z")
}

func fmtFloat(f float64) string {
	return strconv.FormatFloat(f, 'f', 8, 64)
}

func parseInt64(s string) int64 {
	v, _ := strconv.ParseInt(s, 10, 64)
	return v
}

// parseTime parses a timestamp from metadata: ISO 8601 (YYYY-MM-DDTHH:mm:ss.SSSZ) or raw epoch milliseconds.
func parseTime(s string) int64 {
	if s == "" {
		return 0
	}
	if strings.Contains(s, "T") && (strings.HasSuffix(s, "Z") || strings.Contains(s, "+") || len(s) > 20) {
		t, err := time.Parse(time.RFC3339Nano, s)
		if err != nil {
			t, _ = time.Parse("2006-01-02T15:04:05.000Z", s)
		}
		if !t.IsZero() {
			return t.UnixNano() / int64(time.Millisecond)
		}
	}
	return parseInt64(s)
}

func parseInt(s string) int {
	v, _ := strconv.Atoi(s)
	return v
}
