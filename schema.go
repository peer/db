package peerdb

import (
	"encoding/json"
	"net/http"

	"gitlab.com/tozd/waf"

	"gitlab.com/peerdb/peerdb/document"
)

// SchemaJSONGet serves the editor schema (document/schema.json) at /schema.json. The frontend
// fetches it when the editor loads and builds its ProseMirror editor schema from it, so the editor
// and the backend share a single schema definition.
func (s *Service) SchemaJSONGet(w http.ResponseWriter, req *http.Request, _ waf.Params) {
	s.WriteJSON(w, req, json.RawMessage(document.SchemaJSON), nil)
}
