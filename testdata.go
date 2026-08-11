package peerdb

import (
	"context"
	"io/fs"
	"path/filepath"

	"github.com/rs/zerolog"
	"gitlab.com/tozd/go/errors"

	"gitlab.com/peerdb/peerdb/internal/xeno"
	"gitlab.com/peerdb/peerdb/transform"
)

// testDataFilesDirectory is the sub-directory of the test data directory holding the attachments.
// Documents link to an attachment as "/f/" followed by the identifier derived from the test data
// namespace, its storage segment, and the file's path inside this sub-directory.
const testDataFilesDirectory = "files"

// fileToInsert describes a file to insert into the storage. Dir is the directory the contents are
// read from, Path is the file's path inside Dir and also the last segment of the ID under which the
// file is stored, and Filename is the name shown to the user.
type fileToInsert struct {
	Dir      string
	Path     string
	Filename string
}

// testDataClasses maps every class with test data to the sub-directory of the test data directory
// its documents are loaded from. Loading is per class because each class unmarshals into its own
// struct, which also rejects any field the class does not have.
//
// The controlled vocabularies all load into the same struct, since an entry of one is the same shape
// as an entry of another and only its instance-of claim says which vocabulary it belongs to. The
// units load into it as well: they are vocabulary entries of a core class rather than a test data
// one, which is again only a matter of what their instance-of claim says.
//
//nolint:gochecknoglobals
var testDataClasses = []struct {
	Directory string
	Load      func(ctx context.Context, path string) ([]any, errors.E)
}{
	{"artifact", transform.Load[xeno.Artifact]},
	{"artifact_category", transform.Load[xeno.Vocabulary]},
	{"biome", transform.Load[xeno.Vocabulary]},
	{"collective", transform.Load[xeno.Collective]},
	{"communication_modality", transform.Load[xeno.Vocabulary]},
	{"communication_system", transform.Load[xeno.CommunicationSystem]},
	{"contact_status", transform.Load[xeno.Vocabulary]},
	{"culture", transform.Load[xeno.Culture]},
	{"ethics_protocol", transform.Load[xeno.Vocabulary]},
	{"expedition", transform.Load[xeno.Expedition]},
	{"galaxy", transform.Load[xeno.Galaxy]},
	{"individual", transform.Load[xeno.Individual]},
	{"individuality_mode", transform.Load[xeno.Vocabulary]},
	{"institute", transform.Load[xeno.Institute]},
	{"interview", transform.Load[xeno.Interview]},
	{"kinship_system", transform.Load[xeno.Vocabulary]},
	{"moon", transform.Load[xeno.Moon]},
	{"narrative", transform.Load[xeno.Narrative]},
	{"narrative_genre", transform.Load[xeno.Vocabulary]},
	{"observation", transform.Load[xeno.Observation]},
	{"organism", transform.Load[xeno.Organism]},
	{"organism_category", transform.Load[xeno.Vocabulary]},
	{"planet", transform.Load[xeno.Planet]},
	{"planet_type", transform.Load[xeno.Vocabulary]},
	{"practice", transform.Load[xeno.Practice]},
	{"practice_category", transform.Load[xeno.Vocabulary]},
	{"publication", transform.Load[xeno.Publication]},
	{"region", transform.Load[xeno.Region]},
	{"research_method", transform.Load[xeno.Vocabulary]},
	{"researcher", transform.Load[xeno.Researcher]},
	{"sector", transform.Load[xeno.Sector]},
	{"sensory_modality", transform.Load[xeno.Vocabulary]},
	{"site", transform.Load[xeno.Site]},
	{"site_type", transform.Load[xeno.Vocabulary]},
	{"social_organisation", transform.Load[xeno.Vocabulary]},
	{"species", transform.Load[xeno.Species]},
	{"star_system", transform.Load[xeno.StarSystem]},
	{"subsistence_mode", transform.Load[xeno.Vocabulary]},
	{"unit", transform.Load[xeno.Vocabulary]},
}

// loadTestData loads the test data documents from the per-class sub-directories of dir. A
// sub-directory which does not exist contributes no documents, so a partial test data directory
// still populates.
func loadTestData(ctx context.Context, logger zerolog.Logger, dir string) ([]any, errors.E) {
	documents := []any{}

	for _, class := range testDataClasses {
		if ctx.Err() != nil {
			return nil, errors.WithStack(ctx.Err())
		}

		docs, errE := class.Load(ctx, filepath.Join(dir, class.Directory))
		if errE != nil {
			errors.Details(errE)["directory"] = class.Directory
			return nil, errE
		}
		documents = append(documents, docs...)

		logger.Debug().Str("directory", class.Directory).Int("count", len(docs)).Msg("loaded test data documents")
	}

	return documents, nil
}

// testDataFiles returns the attachments found in the files sub-directory of dir, to be inserted into
// the storage. A missing sub-directory yields no files.
func testDataFiles(dir string) ([]fileToInsert, errors.E) {
	root := filepath.Join(dir, testDataFilesDirectory)
	files := []fileToInsert{}

	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		// Skip if file or directory (even path) does not exist.
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		} else if err != nil {
			return errors.WithStack(err)
		}

		if d.IsDir() {
			return nil
		}

		path, err := filepath.Rel(root, p)
		if err != nil {
			return errors.WithStack(err)
		}

		files = append(files, fileToInsert{
			Dir:      root,
			Path:     path,
			Filename: d.Name(),
		})

		return nil
	})
	if err != nil {
		errE := errors.WithStack(err)
		errors.Details(errE)["path"] = root
		return nil, errE
	}

	return files, nil
}
