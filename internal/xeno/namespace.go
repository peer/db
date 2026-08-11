// Package xeno provides the schema (properties, classes and entity types) of the optional test
// data set: a made-up catalogue of a xenoanthropology research consortium. It exists so that a
// development instance can be populated with data which exercises every part of PeerDB.
package xeno

// Namespace is the namespace of the test data entities.
const Namespace = "xeno.peerdb.org"

// FilesStorage is the segment between the namespace and a file's path in the ID under which a test
// data attachment is stored. Documents link to an attachment as "/f/" followed by the identifier
// derived from the namespace, this segment, and the file's path inside the test data files
// directory.
const FilesStorage = "TEST_STORAGE"
