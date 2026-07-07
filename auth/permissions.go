package auth

// RoleEveryone is a reserved role name under which sites can declare permissions which apply
// to every caller, authenticated or not. It is not a real role: tokens cannot claim it (empty
// role names are dropped at authentication time), it is never attached to the context, and it
// is not granted by mock sign-in.
//
// Keep in sync with src/auth/index.ts.
const RoleEveryone = ""

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
