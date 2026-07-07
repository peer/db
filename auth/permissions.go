package auth

// PeerDB permissions.
//
// Keep in sync with src/auth/index.ts.
const (
	// CanEditDocument allows creating new documents and editing existing documents.
	CanEditDocument = "canEditDocument"
	// CanDeleteDocument allows deleting documents.
	CanDeleteDocument  = "canDeleteDocument"
	CanChangesDocument = "canChangesDocument"
	CanBulkGetFile     = "canBulkGetFile"
	CanChangesFile     = "canChangesFile"
	// CanEditFile allows adding (uploading) files.
	CanEditFile = "canEditFile"
	// CanDeleteFile allows removing files.
	CanDeleteFile = "canDeleteFile"
)
