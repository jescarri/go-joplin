package models

// Item type constants matching Joplin's ModelType enum.
const (
	TypeNote      = 1
	TypeFolder    = 2
	TypeSetting   = 3
	TypeResource  = 4
	TypeTag       = 5
	TypeNoteTag   = 6
	TypeMasterKey = 9
	TypeRevision  = 13 // Note history / revision (sync item; we don't persist content)
)

// Note represents a Joplin note.
type Note struct {
	ID                string `json:"id"`
	ParentID          string `json:"parent_id"`
	Title             string `json:"title"`
	Body              string `json:"body,omitempty"`
	CreatedTime       int64  `json:"created_time"`
	UpdatedTime       int64  `json:"updated_time"`
	IsConflict        int    `json:"is_conflict"`
	Latitude          float64 `json:"latitude"`
	Longitude         float64 `json:"longitude"`
	Altitude          float64 `json:"altitude"`
	Author            string `json:"author"`
	SourceURL         string `json:"source_url"`
	IsTodo            int    `json:"is_todo"`
	TodoDue           int64  `json:"todo_due"`
	TodoCompleted     int64  `json:"todo_completed"`
	Source            string `json:"source"`
	SourceApplication string `json:"source_application"`
	ApplicationData   string `json:"application_data"`
	Order             int64  `json:"order"`
	UserCreatedTime   int64  `json:"user_created_time"`
	UserUpdatedTime   int64  `json:"user_updated_time"`
	EncryptionCipherText string `json:"encryption_cipher_text"`
	EncryptionApplied int    `json:"encryption_applied"`
	MarkupLanguage    int    `json:"markup_language"`
	IsShared          int    `json:"is_shared"`
	ShareID           string `json:"share_id"`
	ConflictOriginalID string `json:"conflict_original_id"`
	MasterKeyID       string `json:"master_key_id"`
	UserData          string `json:"user_data"`
	DeletedTime       int64  `json:"deleted_time"`

	// Virtual fields for API responses
	Type_ int `json:"type_,omitempty"`
}

// Folder represents a Joplin notebook/folder.
type Folder struct {
	ID                string `json:"id"`
	Title             string `json:"title"`
	CreatedTime       int64  `json:"created_time"`
	UpdatedTime       int64  `json:"updated_time"`
	UserCreatedTime   int64  `json:"user_created_time"`
	UserUpdatedTime   int64  `json:"user_updated_time"`
	EncryptionCipherText string `json:"encryption_cipher_text"`
	EncryptionApplied int    `json:"encryption_applied"`
	ParentID          string `json:"parent_id"`
	IsShared          int    `json:"is_shared"`
	ShareID           string `json:"share_id"`
	MasterKeyID       string `json:"master_key_id"`
	Icon              string `json:"icon"`
	UserData          string `json:"user_data"`
	DeletedTime       int64  `json:"deleted_time"`

	// Virtual fields
	Type_    int       `json:"type_,omitempty"`
	Children []*Folder `json:"children,omitempty"`
	NoteCount int      `json:"note_count,omitempty"`
}

// Tag represents a Joplin tag.
type Tag struct {
	ID                string `json:"id"`
	Title             string `json:"title"`
	CreatedTime       int64  `json:"created_time"`
	UpdatedTime       int64  `json:"updated_time"`
	UserCreatedTime   int64  `json:"user_created_time"`
	UserUpdatedTime   int64  `json:"user_updated_time"`
	EncryptionCipherText string `json:"encryption_cipher_text"`
	EncryptionApplied int    `json:"encryption_applied"`
	IsShared          int    `json:"is_shared"`
	ParentID          string `json:"parent_id"`
	UserData          string `json:"user_data"`

	Type_ int `json:"type_,omitempty"`
}

// NoteTag represents the junction between notes and tags.
type NoteTag struct {
	ID                string `json:"id"`
	NoteID            string `json:"note_id"`
	TagID             string `json:"tag_id"`
	CreatedTime       int64  `json:"created_time"`
	UpdatedTime       int64  `json:"updated_time"`
	UserCreatedTime   int64  `json:"user_created_time"`
	UserUpdatedTime   int64  `json:"user_updated_time"`
	EncryptionCipherText string `json:"encryption_cipher_text"`
	EncryptionApplied int    `json:"encryption_applied"`
	IsShared          int    `json:"is_shared"`

	Type_ int `json:"type_,omitempty"`
}

// Resource represents a Joplin resource (attachment).
type Resource struct {
	ID                string `json:"id"`
	Title             string `json:"title"`
	Mime              string `json:"mime"`
	Filename          string `json:"filename"`
	CreatedTime       int64  `json:"created_time"`
	UpdatedTime       int64  `json:"updated_time"`
	UserCreatedTime   int64  `json:"user_created_time"`
	UserUpdatedTime   int64  `json:"user_updated_time"`
	FileExtension     string `json:"file_extension"`
	EncryptionCipherText string `json:"encryption_cipher_text"`
	EncryptionApplied int    `json:"encryption_applied"`
	EncryptionBlobEncrypted int `json:"encryption_blob_encrypted"`
	Size              int64  `json:"size"`
	IsShared          int    `json:"is_shared"`
	ShareID           string `json:"share_id"`
	MasterKeyID       string `json:"master_key_id"`
	UserData          string `json:"user_data"`
	BlobUpdatedTime   int64  `json:"blob_updated_time"`

	Type_ int `json:"type_,omitempty"`
}

// SyncItem tracks the sync state of an item.
type SyncItem struct {
	ID            int    `json:"id"`
	SyncTarget    int    `json:"sync_target"`
	SyncTime      int64  `json:"sync_time"`
	ItemType      int    `json:"item_type"`
	ItemID        string `json:"item_id"`
	SyncDisabled  int    `json:"sync_disabled"`
	SyncDisabledReason string `json:"sync_disabled_reason"`
	ForceSync     int    `json:"force_sync"`
	ItemLocation  int    `json:"item_location"`
}

// DeletedItem tracks items that need to be deleted on the server.
type DeletedItem struct {
	ID         int    `json:"id"`
	ItemType   int    `json:"item_type"`
	ItemID     string `json:"item_id"`
	DeletedTime int64 `json:"deleted_time"`
	SyncTarget int    `json:"sync_target"`
}

// ItemChange tracks changes to items for the events API.
type ItemChange struct {
	ID        int    `json:"id"`
	ItemType  int    `json:"item_type"`
	ItemID    string `json:"item_id"`
	Type      int    `json:"type"` // 1=create, 2=update, 3=delete
	CreatedTime int64 `json:"created_time"`
}

// MasterKey represents a Joplin E2EE master key.
type MasterKey struct {
	ID                string `json:"id"`
	CreatedTime       int64  `json:"created_time"`
	UpdatedTime       int64  `json:"updated_time"`
	SourceApplication string `json:"source_application"`
	EncryptionMethod  int    `json:"encryption_method"`
	Checksum          string `json:"checksum"`
	Content           string `json:"content"` // encrypted master key content (JSON)
}

// PaginatedResponse wraps paginated API responses.
type PaginatedResponse struct {
	Items   interface{} `json:"items"`
	HasMore bool        `json:"has_more"`
}
