package peerdb

import (
	"net"
	"net/http"
	"strings"

	"github.com/olivere/elastic/v7"
)

func hasConnectionUpgrade(req *http.Request) bool {
	for _, value := range strings.Split(req.Header.Get("Connection"), ",") {
		if strings.ToLower(strings.TrimSpace(value)) == "upgrade" {
			return true
		}
	}
	return false
}

// Same as in zerolog/hlog/hlog.go.
func getHost(hostPort string) string {
	if hostPort == "" {
		return ""
	}

	host, _, err := net.SplitHostPort(hostPort)
	if err != nil {
		return hostPort
	}
	return host
}

// InsertOrReplaceDocument inserts or replaces the document based on its ID.
func InsertOrReplaceDocument(processor *elastic.BulkProcessor, index string, doc *Document) {
	req := elastic.NewBulkIndexRequest().Index(index).Id(doc.ID.String()).Doc(doc)
	processor.Add(req)
}

// UpdateDocument updates the document in the index, if it has not changed in the database since it was fetched (based on seqNo and primaryTerm).
func UpdateDocument(processor *elastic.BulkProcessor, index string, seqNo, primaryTerm int64, doc *Document) {
	// TODO: Update to use PostgreSQL store.
	req := elastic.NewBulkIndexRequest().Index(index).Id(doc.ID.String()).IfSeqNo(seqNo).IfPrimaryTerm(primaryTerm).Doc(&doc)
	processor.Add(req)
}
