package xeno

import (
	"gitlab.com/peerdb/peerdb/core"
)

// Vocabulary is an entry of one of the test data's controlled vocabularies, and also the shape the
// extra measurement units take. Which vocabulary an entry belongs to is what its instance-of claim
// says, so one type serves them all.
type Vocabulary struct {
	core.VocabularyFields
	core.DocumentFields
}

// Galaxy is the top of the containment chain.
type Galaxy struct {
	GalaxyFields
	core.DocumentFields
}

// Sector is an administrative slice of a galaxy.
type Sector struct {
	SectorFields
	core.DocumentFields
}

// StarSystem is a star system inside a sector.
type StarSystem struct {
	StarSystemFields
	core.DocumentFields
}

// Planet is a world orbiting a star.
type Planet struct {
	WorldFields
	core.DocumentFields
}

// Moon is a world orbiting a planet. It carries the same fields as a planet, because the difference
// between the two is what it orbits and nothing else the catalogue records.
type Moon struct {
	WorldFields
	core.DocumentFields
}

// Region is a stretch of a world's surface.
type Region struct {
	RegionFields
	core.DocumentFields
}

// Site is a place where people are actually met.
type Site struct {
	SiteFields
	core.DocumentFields
}

// Species is a kind of being.
type Species struct {
	SpeciesFields
	core.DocumentFields
}

// Individual is one being of a species which has beings.
type Individual struct {
	IndividualFields
	core.DocumentFields
}

// Collective is a body which acts as one and which the discipline does not split into individuals.
type Collective struct {
	CollectiveFields
	core.DocumentFields
}

// Culture is the unit the discipline compares.
type Culture struct {
	CultureFields
	core.DocumentFields
}

// Practice is something a culture does.
type Practice struct {
	PracticeFields
	core.DocumentFields
}

// CommunicationSystem is a way a species communicates.
type CommunicationSystem struct {
	CommunicationSystemFields
	core.DocumentFields
}

// Artifact is a made object held in a collection.
type Artifact struct {
	ArtifactFields
	core.DocumentFields
}

// Narrative is a told or inscribed thing which was written down.
type Narrative struct {
	NarrativeFields
	core.DocumentFields
}

// Organism is non-sapient life.
type Organism struct {
	OrganismFields
	core.DocumentFields
}

// Institute is a member body of the Consortium.
type Institute struct {
	InstituteFields
	core.DocumentFields
}

// Researcher is one xenoanthropologist.
type Researcher struct {
	ResearcherFields
	core.DocumentFields
}

// Expedition is a funded trip somewhere.
type Expedition struct {
	ExpeditionFields
	core.DocumentFields
}

// Observation is one field note. Most are public; the ones taken where somebody asked for restraint
// carry permission claims of their own and are then readable only by the researchers they name.
type Observation struct {
	ObservationFields
	PermissionFields
	core.DocumentFields
}

// Interview is a recorded conversation. Interviews are never public: the site does not grant reading
// this class to anybody, so an interview is reachable only through the permission claims it carries.
type Interview struct {
	InterviewFields
	PermissionFields
	core.DocumentFields
}

// Publication is a paper.
type Publication struct {
	PublicationFields
	core.DocumentFields
}
