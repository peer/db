package main

import (
	"github.com/olivere/elastic/v7"

	"gitlab.com/peerdb/search"
)

func saveDocument(globals *Globals, processor *elastic.BulkProcessor, doc *search.Document) {
	req := elastic.NewBulkIndexRequest().Index("docs").Id(string(doc.ID)).Doc(doc)
	processor.Add(req)
}

func updateDocument(globals *Globals, processor *elastic.BulkProcessor, seqNo, primaryTerm int64, doc *search.Document) {
	req := elastic.NewBulkIndexRequest().Index("docs").Id(string(doc.ID)).IfSeqNo(seqNo).IfPrimaryTerm(primaryTerm).Doc(&doc)
	processor.Add(req)
}
