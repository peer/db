package xeno

import (
	"gitlab.com/peerdb/peerdb/core"
)

// Endonym is a name the inhabitants use, carrying what it means and which communication system it
// belongs to. It is the shape a name-with-context takes throughout the catalogue.
//
//nolint:lll
type Endonym struct {
	Value string `json:"value" value:""`

	Gloss    *string    `cardinality:"0..1" json:"gloss,omitempty"    order:"1" property:"GLOSS"`
	InSystem []core.Ref `cardinality:"0.."  json:"inSystem,omitempty" order:"2" property:"IN_COMMUNICATION_SYSTEM" values:"core.peerdb.org,INSTANCE_OF=xeno.peerdb.org,COMMUNICATION_SYSTEM"`
}

// Image is an attached picture with the file's name, the caption shown under it, and a note on where
// it came from. The name is the file name the picture was accessioned under, which the interface
// shows in place of the link it would otherwise have to print, so it is not a field of its own to a
// reader.
type Image struct {
	Value core.File `json:"value" value:""`

	Name    *string    `cardinality:"0..1" context:"edit" json:"name,omitempty"    order:"1" property:"NAME"`
	Caption *string    `cardinality:"0..1"                json:"caption,omitempty" order:"2" property:"CAPTION"`
	Source  *core.HTML `cardinality:"0..1"                json:"source,omitempty"  order:"3" property:"SOURCE"`
}

// Attachment is an attached document (a report, a scan, a data table) with its file name and its
// caption.
type Attachment struct {
	Value core.File `json:"value" value:""`

	Name    *string `cardinality:"0..1" context:"edit" json:"name,omitempty"    order:"1" property:"NAME"`
	Caption *string `cardinality:"0..1"                json:"caption,omitempty" order:"2" property:"CAPTION"`
}

// Recording is an attached audio recording with its file name, its caption, and how long it runs.
//
//nolint:lll
type Recording struct {
	Value core.File `json:"value" value:""`

	Name     *string                    `cardinality:"0..1" context:"edit" json:"name,omitempty"     order:"1" property:"NAME"`
	Caption  *string                    `cardinality:"0..1"                json:"caption,omitempty"  order:"2" property:"CAPTION"`
	Duration *core.AmountWithUnit[int]  `cardinality:"0..1"                json:"duration,omitempty" order:"3" property:"DURATION"`
	InSystem []core.Ref                 `cardinality:"0.."                 json:"inSystem,omitempty" order:"4" property:"IN_COMMUNICATION_SYSTEM" values:"core.peerdb.org,INSTANCE_OF=xeno.peerdb.org,COMMUNICATION_SYSTEM"`
	Note     []core.RawHTMLWithLanguage `cardinality:"0.."                 json:"note,omitempty"     order:"5" property:"NOTES"`
}

// Dimension is one measurement of a made object, which is only meaningful together with the axis it
// was taken along, so the axis is a sub-claim of the measurement rather than a field of its own.
type Dimension struct {
	Value core.Amount[float64] `json:"value" value:""`

	Axis   string     `cardinality:"1"   json:"axis"             order:"1" property:"AXIS"`
	InUnit []core.Ref `cardinality:"0.." json:"inUnit,omitempty" order:"2" property:"IN_UNIT" values:"core.peerdb.org,INSTANCE_OF=core.peerdb.org,UNIT"`
}

// Permission is one document-level permission claim: the action it grants, the users it grants it
// to, and the scope which makes it apply to the document carrying it. Documents which are not
// public carry these, and nothing else lets anybody but an administrator read them.
type Permission struct {
	Action core.Ref `json:"action" value:"" values:"core.peerdb.org,INSTANCE_OF=core.peerdb.org,PERMISSION_ACTIONS"`

	User  []core.Identifier `cardinality:"1.." json:"user"  order:"1" property:"PERMISSION_USER"`
	Scope []string          `cardinality:"1.." json:"scope" order:"2" property:"PERMISSION_SCOPE"`
}

// PermissionFields is embedded by every class whose documents can be restricted. The permission
// claims are hidden from the field schema (order "-"): they are written and read through the
// permissions tab of a document, never as an ordinary field. Requested permissions carry the same
// shape and are what somebody asking for access leaves behind, waiting for whoever decides them.
type PermissionFields struct {
	Permission          []Permission `cardinality:"0.." json:"permission,omitempty"          order:"-" property:"HAS_PERMISSION"`
	RequestedPermission []Permission `cardinality:"0.." json:"requestedPermission,omitempty" order:"-" property:"HAS_REQUESTED_PERMISSION"`
}

// PlaceFields is what every place in the catalogue has, from a galaxy down to a hearth cluster.
type PlaceFields struct {
	Name          []core.StringWithLanguage  `cardinality:"1.."                  json:"name"                    order:"1"  property:"NAME"`
	Endonym       []Endonym                  `cardinality:"0.."  duplicate:"top" json:"endonym,omitempty"       order:"2"  property:"ENDONYM"`
	CatalogueCode *core.Identifier           `cardinality:"0..1"                 json:"catalogueCode,omitempty" order:"3"  property:"CATALOGUE_CODE"`
	Description   []core.RawHTMLWithLanguage `cardinality:"0.."                  json:"description,omitempty"   order:"4"  property:"DESCRIPTION"`
	Image         []Image                    `cardinality:"0.."                  json:"image,omitempty"         order:"90" property:"IMAGE"`
	Website       []core.Link                `cardinality:"0.."                  json:"website,omitempty"       order:"95" property:"WEBSITE"`
	Notes         []core.RawHTMLWithLanguage `cardinality:"0.."                  json:"notes,omitempty"         order:"99" property:"NOTES"`
}

// GalaxyFields describes a galaxy, the top of the containment chain.
type GalaxyFields struct {
	PlaceFields

	Diameter     *core.AmountWithUnit[float64] `cardinality:"0..1" json:"diameter,omitempty"     order:"10" property:"DIAMETER"`
	SurveyPeriod *core.Interval[core.Time]     `cardinality:"0..1" json:"surveyPeriod,omitempty" order:"11" property:"SURVEY_PERIOD"`
}

// SectorFields describes a survey sector, an administrative slice of a galaxy which the Consortium
// draws and redraws for its own convenience.
//
//nolint:lll
type SectorFields struct {
	PlaceFields

	ContainedIn  core.Ref                  `cardinality:"1"    inverseProperty:"CONTAINS" json:"containedIn"            order:"5"  property:"CONTAINED_IN"  values:"core.peerdb.org,INSTANCE_OF=xeno.peerdb.org,GALAXY"`
	SurveyPeriod *core.Interval[core.Time] `cardinality:"0..1"                            json:"surveyPeriod,omitempty" order:"11" property:"SURVEY_PERIOD"`
}

// StarSystemFields describes a star system.
//
//nolint:lll
type StarSystemFields struct {
	PlaceFields

	ContainedIn     core.Ref                      `cardinality:"1"    inverseProperty:"CONTAINS" json:"containedIn"               order:"5"  property:"CONTAINED_IN"      values:"core.peerdb.org,INSTANCE_OF=xeno.peerdb.org,SECTOR"`
	SpectralClass   *string                       `cardinality:"0..1"                            json:"spectralClass,omitempty"   order:"10" property:"SPECTRAL_CLASS"`
	StarCount       *core.Amount[int]             `cardinality:"0..1"                            json:"starCount,omitempty"       order:"11" property:"STAR_COUNT"`
	PlanetCount     *core.Amount[int]             `cardinality:"0..1"                            json:"planetCount,omitempty"     order:"12" property:"PLANET_COUNT"`
	DistanceFromSol *core.AmountWithUnit[float64] `cardinality:"0..1"                            json:"distanceFromSol,omitempty" order:"13" property:"DISTANCE_FROM_SOL"`
	FirstSurveyed   *core.Time                    `cardinality:"0..1"                            json:"firstSurveyed,omitempty"   order:"14" property:"FIRST_SURVEYED"`
}

// WorldIdentification groups what identifies a world: what it is called and what kind of world it
// is.
//
//nolint:lll
type WorldIdentification struct {
	Name          []core.StringWithLanguage  `cardinality:"1.."                                             json:"name"                    order:"1" property:"NAME"`
	Endonym       []Endonym                  `cardinality:"0.."  duplicate:"top"                            json:"endonym,omitempty"       order:"2" property:"ENDONYM"`
	CatalogueCode *core.Identifier           `cardinality:"0..1"                                            json:"catalogueCode,omitempty" order:"3" property:"CATALOGUE_CODE"`
	Description   []core.RawHTMLWithLanguage `cardinality:"0.."                                             json:"description,omitempty"   order:"4" property:"DESCRIPTION"`
	ContainedIn   core.Ref                   `cardinality:"1"                    inverseProperty:"CONTAINS" json:"containedIn"             order:"5" property:"CONTAINED_IN"    values:"core.peerdb.org,INSTANCE_OF=xeno.peerdb.org,STAR_SYSTEM&core.peerdb.org,INSTANCE_OF=xeno.peerdb.org,PLANET"`
	PlanetType    *core.Ref                  `cardinality:"0..1"                                            json:"planetType,omitempty"    order:"6" property:"HAS_PLANET_TYPE" values:"core.peerdb.org,INSTANCE_OF=xeno.peerdb.org,PLANET_TYPE"`
	Biome         []core.Ref                 `cardinality:"0.."                                             json:"biome,omitempty"         order:"7" property:"HAS_BIOME"       values:"core.peerdb.org,INSTANCE_OF=xeno.peerdb.org,BIOME"`
}

// WorldPhysical groups the bulk properties a survey measures from orbit before anybody lands.
type WorldPhysical struct {
	Radius         *core.AmountWithUnit[float64] `cardinality:"0..1" json:"radius,omitempty"         order:"1" property:"RADIUS"`
	Mass           *core.AmountWithUnit[float64] `cardinality:"0..1" json:"mass,omitempty"           order:"2" property:"MASS"`
	SurfaceGravity *core.AmountWithUnit[float64] `cardinality:"0..1" json:"surfaceGravity,omitempty" order:"3" property:"SURFACE_GRAVITY"`
	DayLength      *core.AmountWithUnit[float64] `cardinality:"0..1" json:"dayLength,omitempty"      order:"4" property:"DAY_LENGTH"`
	OrbitalPeriod  *core.AmountWithUnit[float64] `cardinality:"0..1" json:"orbitalPeriod,omitempty"  order:"5" property:"ORBITAL_PERIOD"`
	RingSystem     bool                          `cardinality:"0..1" json:"ringSystem,omitempty"     order:"6" property:"HAS_RING_SYSTEM"`
	TidallyLocked  bool                          `cardinality:"0..1" json:"tidallyLocked,omitempty"  order:"7" property:"TIDALLY_LOCKED"`
}

// WorldEnvironment groups what the surface is like and what, if anything, lives on it.
//
// The biosphere is the catalogue's worked example of the three ways a property can be absent: a
// world with life carries a description, a world surveyed and found sterile carries the property
// with no value, and a world nobody has looked at closely carries it as unknown.
type WorldEnvironment struct {
	MeanTemperature  *core.AmountWithUnit[float64] `cardinality:"0..1" json:"meanTemperature,omitempty"  order:"1" property:"MEAN_TEMPERATURE"`
	Hydrosphere      *core.AmountWithUnit[float64] `cardinality:"0..1" json:"hydrosphere,omitempty"      order:"2" property:"HYDROSPHERE"`
	Atmosphere       []core.RawHTMLWithLanguage    `cardinality:"0.."  json:"atmosphere,omitempty"       order:"3" property:"ATMOSPHERE"`
	Biosphere        []core.RawHTMLWithLanguage    `cardinality:"0.."  json:"biosphere,omitempty"        order:"4" property:"BIOSPHERE"`
	BiosphereNone    core.None                     `cardinality:"0..1" json:"biosphereNone,omitempty"    order:"5" property:"BIOSPHERE"`
	BiosphereUnknown core.Unknown                  `cardinality:"0..1" json:"biosphereUnknown,omitempty" order:"6" property:"BIOSPHERE"`
}

// WorldSurvey groups what the Consortium has done about the world so far.
//
//nolint:lll
type WorldSurvey struct {
	ContactStatus      *core.Ref                         `cardinality:"0..1" json:"contactStatus,omitempty"      order:"1" property:"HAS_CONTACT_STATUS"  values:"core.peerdb.org,INSTANCE_OF=xeno.peerdb.org,CONTACT_STATUS"`
	FirstSurveyed      *core.Time                        `cardinality:"0..1" json:"firstSurveyed,omitempty"      order:"2" property:"FIRST_SURVEYED"`
	SurveyPeriod       *core.Interval[core.Time]         `cardinality:"0..1" json:"surveyPeriod,omitempty"       order:"3" property:"SURVEY_PERIOD"`
	PopulationEstimate *core.AmountIntervalWithUnit[int] `cardinality:"0..1" json:"populationEstimate,omitempty" order:"4" property:"POPULATION_ESTIMATE"`
	Image              []Image                           `cardinality:"0.."  json:"image,omitempty"              order:"5" property:"IMAGE"`
	Notes              []core.RawHTMLWithLanguage        `cardinality:"0.."  json:"notes,omitempty"              order:"6" property:"NOTES"`
}

// WorldFields is the field schema shared by planets and moons, split into the four sections a world
// record is read in.
type WorldFields struct {
	WorldIdentification `order:"1" section:"identification"`
	WorldPhysical       `order:"2" section:"physical"`
	WorldEnvironment    `order:"3" section:"environment"`
	WorldSurvey         `order:"4" section:"survey"`
}

// RegionFields describes a stretch of a world's surface.
//
//nolint:lll
type RegionFields struct {
	PlaceFields

	ContainedIn    core.Ref                          `cardinality:"1"    embed:"xeno.peerdb.org,HAS_BIOME=xeno.peerdb.org,HAS_BIOME" inverseProperty:"CONTAINS" json:"containedIn"              order:"5"  property:"CONTAINED_IN"    values:"core.peerdb.org,INSTANCE_OF=xeno.peerdb.org,PLANET&core.peerdb.org,INSTANCE_OF=xeno.peerdb.org,MOON"`
	Biome          []core.Ref                        `cardinality:"0.."                                                                                         json:"biome,omitempty"          order:"6"  property:"HAS_BIOME"       values:"core.peerdb.org,INSTANCE_OF=xeno.peerdb.org,BIOME"`
	Area           *core.AmountWithUnit[float64]     `cardinality:"0..1"                                                                                        json:"area,omitempty"           order:"10" property:"AREA"`
	ElevationRange *core.AmountIntervalWithUnit[int] `cardinality:"0..1"                                                                                        json:"elevationRange,omitempty" order:"11" property:"ELEVATION_RANGE"`
}

// SiteEndonym is the local name of a field site. Unlike other endonyms it defaults to unknown: a
// site the survey named from orbit has no local name on record, and saying so is worth more than
// leaving the field empty.
//
//nolint:lll
type SiteEndonym struct {
	Value *string `default:"unknown" json:"value,omitempty" value:""`

	Gloss    *string    `cardinality:"0..1" json:"gloss,omitempty"    order:"1" property:"GLOSS"`
	InSystem []core.Ref `cardinality:"0.."  json:"inSystem,omitempty" order:"2" property:"IN_COMMUNICATION_SYSTEM" values:"core.peerdb.org,INSTANCE_OF=xeno.peerdb.org,COMMUNICATION_SYSTEM"`
}

// SiteFields describes a settlement, camp or other place people are actually met.
//
//nolint:lll
type SiteFields struct {
	Name          []core.StringWithLanguage         `cardinality:"1.."                                             json:"name"                    order:"1"  property:"NAME"`
	LocalName     []SiteEndonym                     `cardinality:"0.."  duplicate:"top"                            json:"localName,omitempty"     order:"2"  property:"ENDONYM"`
	CatalogueCode *core.Identifier                  `cardinality:"0..1"                                            json:"catalogueCode,omitempty" order:"3"  property:"CATALOGUE_CODE"`
	Description   []core.RawHTMLWithLanguage        `cardinality:"0.."                                             json:"description,omitempty"   order:"4"  property:"DESCRIPTION"`
	ContainedIn   core.Ref                          `cardinality:"1"                    inverseProperty:"CONTAINS" json:"containedIn"             order:"5"  property:"CONTAINED_IN"        values:"core.peerdb.org,INSTANCE_OF=xeno.peerdb.org,REGION"`
	SiteType      *core.Ref                         `cardinality:"0..1"                                            json:"siteType,omitempty"      order:"6"  property:"HAS_SITE_TYPE"       values:"core.peerdb.org,INSTANCE_OF=xeno.peerdb.org,SITE_TYPE"`
	GridReference *core.Identifier                  `cardinality:"0..1"                                            json:"gridReference,omitempty" order:"7"  property:"GRID_REFERENCE"`
	Population    *core.AmountIntervalWithUnit[int] `cardinality:"0..1"                                            json:"population,omitempty"    order:"8"  property:"POPULATION_ESTIMATE"`
	Founded       *core.Time                        `cardinality:"0..1"                                            json:"founded,omitempty"       order:"9"  property:"FOUNDED"`
	Occupation    *core.Interval[core.Time]         `cardinality:"0..1"                                            json:"occupation,omitempty"    order:"10" property:"OCCUPATION_PERIOD"`
	Image         []Image                           `cardinality:"0.."                                             json:"image,omitempty"         order:"90" property:"IMAGE"`
	Notes         []core.RawHTMLWithLanguage        `cardinality:"0.."                                             json:"notes,omitempty"         order:"99" property:"NOTES"`
}

// SpeciesIdentification groups what the species is called and where it is from.
//
//nolint:lll
type SpeciesIdentification struct {
	Name        []core.StringWithLanguage  `cardinality:"1.."                                                    json:"name"                  order:"1" property:"NAME"`
	Endonym     []Endonym                  `cardinality:"0.."  duplicate:"top"                                   json:"endonym,omitempty"     order:"2" property:"ENDONYM"`
	Exonym      []core.StringWithLanguage  `cardinality:"0.."                                                    json:"exonym,omitempty"      order:"3" property:"ALTERNATIVE_NAME"`
	TaxonCode   *core.Identifier           `cardinality:"0..1"                                                   json:"taxonCode,omitempty"   order:"4" property:"TAXON_CODE"`
	Description []core.RawHTMLWithLanguage `cardinality:"0.."                                                    json:"description,omitempty" order:"5" property:"DESCRIPTION"`
	Homeworld   *core.Ref                  `cardinality:"0..1"                 inverseProperty:"HOME_TO_SPECIES" json:"homeworld,omitempty"   order:"6" property:"HAS_HOMEWORLD"    values:"core.peerdb.org,INSTANCE_OF=xeno.peerdb.org,PLANET&core.peerdb.org,INSTANCE_OF=xeno.peerdb.org,MOON"`
	AlsoFoundOn []core.Ref                 `cardinality:"0.."  duplicate:"top"                                   json:"alsoFoundOn,omitempty" order:"7" property:"ALSO_FOUND_ON"    values:"core.peerdb.org,INSTANCE_OF=xeno.peerdb.org,PLANET&core.peerdb.org,INSTANCE_OF=xeno.peerdb.org,MOON"`
}

// SpeciesBiology groups the body and how it senses and feeds.
//
//nolint:lll
type SpeciesBiology struct {
	BodyPlan        []core.RawHTMLWithLanguage        `cardinality:"0.."  json:"bodyPlan,omitempty"        order:"1" property:"BODY_PLAN"`
	TypicalHeight   *core.AmountWithUnit[float64]     `cardinality:"0..1" json:"typicalHeight,omitempty"   order:"2" property:"TYPICAL_HEIGHT"`
	TypicalMass     *core.AmountWithUnit[float64]     `cardinality:"0..1" json:"typicalMass,omitempty"     order:"3" property:"TYPICAL_MASS"`
	Lifespan        *core.AmountIntervalWithUnit[int] `cardinality:"0..1" json:"lifespan,omitempty"        order:"4" property:"LIFESPAN"`
	SensoryModality []core.Ref                        `cardinality:"0.."  json:"sensoryModality,omitempty" order:"5" property:"HAS_SENSORY_MODALITY" values:"core.peerdb.org,INSTANCE_OF=xeno.peerdb.org,SENSORY_MODALITY"`
	Subsistence     []core.Ref                        `cardinality:"0.."  json:"subsistence,omitempty"     order:"6" property:"HAS_SUBSISTENCE_MODE" values:"core.peerdb.org,INSTANCE_OF=xeno.peerdb.org,SUBSISTENCE_MODE"`
}

// SpeciesSociety groups how the species organises itself and talks.
//
//nolint:lll
type SpeciesSociety struct {
	Individuality *core.Ref `cardinality:"0..1" json:"individuality,omitempty" order:"1" property:"HAS_INDIVIDUALITY_MODE" values:"core.peerdb.org,INSTANCE_OF=xeno.peerdb.org,INDIVIDUALITY_MODE"`
	// The minority reading of how individual the species is, where the discipline has not settled it.
	// It is recorded at reduced confidence, so it stands beside the accepted reading rather than
	// competing with it.
	ContestedIndividuality *core.Ref                         `cardinality:"0..1" confidence:"0.4"                                   json:"contestedIndividuality,omitempty" order:"1.5" property:"HAS_INDIVIDUALITY_MODE"    values:"core.peerdb.org,INSTANCE_OF=xeno.peerdb.org,INDIVIDUALITY_MODE"`
	SocialOrganisation     *core.Ref                         `cardinality:"0..1"                                                    json:"socialOrganisation,omitempty"     order:"2"   property:"HAS_SOCIAL_ORGANISATION"   values:"core.peerdb.org,INSTANCE_OF=xeno.peerdb.org,SOCIAL_ORGANISATION"`
	KinshipSystem          *core.Ref                         `cardinality:"0..1"                                                    json:"kinshipSystem,omitempty"          order:"3"   property:"HAS_KINSHIP_SYSTEM"        values:"core.peerdb.org,INSTANCE_OF=xeno.peerdb.org,KINSHIP_SYSTEM"`
	Communication          []core.Ref                        `cardinality:"0.."                   inverseProperty:"USED_BY_SPECIES" json:"communication,omitempty"          order:"4"   property:"USES_COMMUNICATION_SYSTEM" values:"core.peerdb.org,INSTANCE_OF=xeno.peerdb.org,COMMUNICATION_SYSTEM"`
	Population             *core.AmountIntervalWithUnit[int] `cardinality:"0..1"                                                    json:"population,omitempty"             order:"5"   property:"POPULATION_ESTIMATE"`
}

// SpeciesContact groups the Consortium's dealings with the species.
//
//nolint:lll
type SpeciesContact struct {
	ContactStatus *core.Ref  `cardinality:"0..1" json:"contactStatus,omitempty" order:"1" property:"HAS_CONTACT_STATUS" values:"core.peerdb.org,INSTANCE_OF=xeno.peerdb.org,CONTACT_STATUS"`
	FirstContact  *core.Time `cardinality:"0..1" json:"firstContact,omitempty"  order:"2" property:"FIRST_CONTACT"`
	Image         []Image    `cardinality:"0.."  json:"image,omitempty"         order:"3" property:"IMAGE"`

	Notes []core.RawHTMLWithLanguage `cardinality:"0.." json:"notes,omitempty" order:"4" property:"NOTES"`
}

// SpeciesFields is the field schema of a species, in the four sections a species record is read in.
type SpeciesFields struct {
	SpeciesIdentification `order:"1" section:"identification"`
	SpeciesBiology        `order:"2" section:"biology"`
	SpeciesSociety        `order:"3" section:"society"`
	SpeciesContact        `order:"4" section:"contact"`
}

// BeingFields is what an individual and a collective have in common: both are a documented member
// of a species, both may belong to a culture, and both are described the same way.
//
//nolint:lll
type BeingFields struct {
	Name        []core.StringWithLanguage  `cardinality:"1.."                                                           json:"name"                  order:"1"  property:"NAME"`
	LineageName []core.StringWithLanguage  `cardinality:"0.."                                                           json:"lineageName,omitempty" order:"2"  property:"LINEAGE_NAME"`
	Description []core.RawHTMLWithLanguage `cardinality:"0.."                                                           json:"description,omitempty" order:"3"  property:"DESCRIPTION"`
	Species     core.Ref                   `cardinality:"1"                     inverseProperty:"HAS_DOCUMENTED_MEMBER" json:"species"               order:"4"  property:"OF_SPECIES"         values:"core.peerdb.org,INSTANCE_OF=xeno.peerdb.org,SPECIES"`
	Culture     []core.Ref                 `cardinality:"0.."                   inverseProperty:"HAS_MEMBER"            json:"culture,omitempty"     order:"5"  property:"BELONGS_TO_CULTURE" values:"core.peerdb.org,INSTANCE_OF=xeno.peerdb.org,CULTURE"`
	Role        []core.StringWithLanguage  `cardinality:"0.." duplicate:"allow"                                         json:"role,omitempty"        order:"6"  property:"ROLE"`
	Image       []Image                    `cardinality:"0.."                                                           json:"image,omitempty"       order:"90" property:"IMAGE"`
	Notes       []core.RawHTMLWithLanguage `cardinality:"0.."                                                           json:"notes,omitempty"       order:"99" property:"NOTES"`
}

// IndividualFields describes one being of a species which has beings.
//
//nolint:lll
type IndividualFields struct {
	BeingFields

	FormOfAddress *string    `cardinality:"0..1" json:"formOfAddress,omitempty" order:"7"  property:"FORM_OF_ADDRESS"`
	Born          *core.Time `cardinality:"0..1" json:"born,omitempty"          order:"8"  property:"BORN"`
	Died          *core.Time `cardinality:"0..1" json:"died,omitempty"          order:"9"  property:"DIED"`
	Birthplace    *core.Ref  `cardinality:"0..1" json:"birthplace,omitempty"    order:"10" property:"BIRTHPLACE"      values:"core.peerdb.org,INSTANCE_OF=xeno.peerdb.org,SITE"`
}

// CollectiveFields describes a body which acts as one and which the discipline cannot sensibly
// split into individuals.
//
//nolint:lll
type CollectiveFields struct {
	BeingFields

	MemberCount  *core.AmountIntervalWithUnit[int] `cardinality:"0..1" json:"memberCount,omitempty"  order:"7" property:"MEMBER_COUNT"`
	ActivePeriod *core.Interval[core.Time]         `cardinality:"0..1" json:"activePeriod,omitempty" order:"8" property:"ACTIVE_PERIOD"`
	Seat         *core.Ref                         `cardinality:"0..1" json:"seat,omitempty"         order:"9" property:"LOCATED_AT"    values:"core.peerdb.org,INSTANCE_OF=xeno.peerdb.org,SITE"`
}

// CultureFields describes a culture: the unit the discipline actually compares.
//
//nolint:lll
type CultureFields struct {
	Name               []core.StringWithLanguage         `cardinality:"1.."                                                  json:"name"                         order:"1"  property:"NAME"`
	Endonym            []Endonym                         `cardinality:"0.."  duplicate:"top"                                 json:"endonym,omitempty"            order:"2"  property:"ENDONYM"`
	CatalogueCode      *core.Identifier                  `cardinality:"0..1"                                                 json:"catalogueCode,omitempty"      order:"3"  property:"CATALOGUE_CODE"`
	Description        []core.RawHTMLWithLanguage        `cardinality:"0.."                                                  json:"description,omitempty"        order:"4"  property:"DESCRIPTION"`
	PractisedBy        []core.Ref                        `cardinality:"1.."                                                  json:"practisedBy"                  order:"5"  property:"PRACTISED_BY"              values:"core.peerdb.org,INSTANCE_OF=xeno.peerdb.org,SPECIES"`
	PresentAt          []core.Ref                        `cardinality:"0.."                  inverseProperty:"HOSTS_CULTURE" json:"presentAt,omitempty"          order:"6"  property:"PRESENT_AT"                values:"core.peerdb.org,INSTANCE_OF=xeno.peerdb.org,SITE"`
	KinshipSystem      *core.Ref                         `cardinality:"0..1"                                                 json:"kinshipSystem,omitempty"      order:"7"  property:"HAS_KINSHIP_SYSTEM"        values:"core.peerdb.org,INSTANCE_OF=xeno.peerdb.org,KINSHIP_SYSTEM"`
	SocialOrganisation *core.Ref                         `cardinality:"0..1"                                                 json:"socialOrganisation,omitempty" order:"8"  property:"HAS_SOCIAL_ORGANISATION"   values:"core.peerdb.org,INSTANCE_OF=xeno.peerdb.org,SOCIAL_ORGANISATION"`
	Subsistence        []core.Ref                        `cardinality:"0.."                                                  json:"subsistence,omitempty"        order:"9"  property:"HAS_SUBSISTENCE_MODE"      values:"core.peerdb.org,INSTANCE_OF=xeno.peerdb.org,SUBSISTENCE_MODE"`
	Communication      []core.Ref                        `cardinality:"0.."                                                  json:"communication,omitempty"      order:"10" property:"USES_COMMUNICATION_SYSTEM" values:"core.peerdb.org,INSTANCE_OF=xeno.peerdb.org,COMMUNICATION_SYSTEM"`
	Population         *core.AmountIntervalWithUnit[int] `cardinality:"0..1"                                                 json:"population,omitempty"         order:"11" property:"POPULATION_ESTIMATE"`
	Period             *core.Interval[core.Time]         `cardinality:"0..1"                                                 json:"period,omitempty"             order:"12" property:"PERIOD"`
	Image              []Image                           `cardinality:"0.."                                                  json:"image,omitempty"              order:"90" property:"IMAGE"`
	Notes              []core.RawHTMLWithLanguage        `cardinality:"0.."                                                  json:"notes,omitempty"              order:"99" property:"NOTES"`
}

// PracticeFields describes something a culture does.
//
//nolint:lll
type PracticeFields struct {
	Name            []core.StringWithLanguage         `cardinality:"1.."                                                         json:"name"                      order:"1"  property:"NAME"`
	Endonym         []Endonym                         `cardinality:"0.."  duplicate:"top"                                        json:"endonym,omitempty"         order:"2"  property:"ENDONYM"`
	Description     []core.RawHTMLWithLanguage        `cardinality:"0.."                                                         json:"description,omitempty"     order:"3"  property:"DESCRIPTION"`
	Culture         []core.Ref                        `cardinality:"1.."                  inverseProperty:"HAS_CULTURAL_ELEMENT" json:"culture"                   order:"4"  property:"OF_CULTURE"            values:"core.peerdb.org,INSTANCE_OF=xeno.peerdb.org,CULTURE"`
	Category        *core.Ref                         `cardinality:"0..1"                                                        json:"category,omitempty"        order:"5"  property:"HAS_PRACTICE_CATEGORY" values:"core.peerdb.org,INSTANCE_OF=xeno.peerdb.org,PRACTICE_CATEGORY"`
	Periodicity     *string                           `cardinality:"0..1"                                                        json:"periodicity,omitempty"     order:"6"  property:"PERIODICITY"`
	Participants    *core.AmountIntervalWithUnit[int] `cardinality:"0..1"                                                        json:"participants,omitempty"    order:"7"  property:"PARTICIPANT_COUNT"`
	FirstDocumented *core.Time                        `cardinality:"0..1"                                                        json:"firstDocumented,omitempty" order:"8"  property:"FIRST_DOCUMENTED"`
	RelatedPractice []core.Ref                        `cardinality:"0.."  duplicate:"top"                                        json:"relatedPractice,omitempty" order:"9"  property:"RELATED_PRACTICE"      values:"core.peerdb.org,INSTANCE_OF=xeno.peerdb.org,PRACTICE"`
	Image           []Image                           `cardinality:"0.."                                                         json:"image,omitempty"           order:"90" property:"IMAGE"`
	Notes           []core.RawHTMLWithLanguage        `cardinality:"0.."                                                         json:"notes,omitempty"           order:"99" property:"NOTES"`
}

// CommunicationSystemFields describes a way a species communicates, which is only sometimes a
// language in the sense the word usually carries.
//
//nolint:lll
type CommunicationSystemFields struct {
	Name        []core.StringWithLanguage         `cardinality:"1.."                  json:"name"                  order:"1"  property:"NAME"`
	Endonym     []Endonym                         `cardinality:"0.."  duplicate:"top" json:"endonym,omitempty"     order:"2"  property:"ENDONYM"`
	Description []core.RawHTMLWithLanguage        `cardinality:"0.."                  json:"description,omitempty" order:"3"  property:"DESCRIPTION"`
	Modality    []core.Ref                        `cardinality:"1.."                  json:"modality"              order:"4"  property:"HAS_COMMUNICATION_MODALITY" values:"core.peerdb.org,INSTANCE_OF=xeno.peerdb.org,COMMUNICATION_MODALITY"`
	Users       *core.AmountIntervalWithUnit[int] `cardinality:"0..1"                 json:"users,omitempty"       order:"5"  property:"SPEAKER_ESTIMATE"`
	Notation    bool                              `cardinality:"0..1"                 json:"notation,omitempty"    order:"6"  property:"HAS_NOTATION_SYSTEM"`
	SampleGloss []core.RawHTMLWithLanguage        `cardinality:"0.."                  json:"sampleGloss,omitempty" order:"7"  property:"SAMPLE_GLOSS"`
	Recording   []Recording                       `cardinality:"0.."                  json:"recording,omitempty"   order:"90" property:"AUDIO"`
	Notes       []core.RawHTMLWithLanguage        `cardinality:"0.."                  json:"notes,omitempty"       order:"99" property:"NOTES"`
}

// ArtifactFields describes a made object held in a Consortium collection.
//
//nolint:lll
type ArtifactFields struct {
	Name          []core.StringWithLanguage  `cardinality:"1.."                                                         json:"name"                    order:"1"  property:"NAME"`
	Endonym       []Endonym                  `cardinality:"0.."  duplicate:"top"                                        json:"endonym,omitempty"       order:"2"  property:"ENDONYM"`
	AccessionCode *core.Identifier           `cardinality:"0..1"                                                        json:"accessionCode,omitempty" order:"3"  property:"ACCESSION_CODE"`
	Description   []core.RawHTMLWithLanguage `cardinality:"0.."                                                         json:"description,omitempty"   order:"4"  property:"DESCRIPTION"`
	Category      *core.Ref                  `cardinality:"0..1"                                                        json:"category,omitempty"      order:"5"  property:"HAS_ARTIFACT_CATEGORY" values:"core.peerdb.org,INSTANCE_OF=xeno.peerdb.org,ARTIFACT_CATEGORY"`
	Culture       []core.Ref                 `cardinality:"0.."                  inverseProperty:"HAS_CULTURAL_ELEMENT" json:"culture,omitempty"       order:"6"  property:"OF_CULTURE"            values:"core.peerdb.org,INSTANCE_OF=xeno.peerdb.org,CULTURE"`
	FoundAt       *core.Ref                  `cardinality:"0..1"                                                        json:"foundAt,omitempty"       order:"7"  property:"FOUND_AT"              values:"core.peerdb.org,INSTANCE_OF=xeno.peerdb.org,SITE"`
	Material      []core.StringWithLanguage  `cardinality:"0.."                                                         json:"material,omitempty"      order:"8"  property:"MATERIAL"`
	Dimension     []Dimension                `cardinality:"0.."                                                         json:"dimension,omitempty"     order:"9"  property:"DIMENSION"`
	DateMade      *core.Time                 `cardinality:"0..1"                                                        json:"dateMade,omitempty"      order:"10" property:"DATE_MADE"`
	CollectedBy   []core.Ref                 `cardinality:"0.."                                                         json:"collectedBy,omitempty"   order:"11" property:"COLLECTED_BY"          values:"core.peerdb.org,INSTANCE_OF=xeno.peerdb.org,RESEARCHER"`
	Image         []Image                    `cardinality:"0.."                                                         json:"image,omitempty"         order:"90" property:"IMAGE"`
	Document      []Attachment               `cardinality:"0.."                                                         json:"document,omitempty"      order:"91" property:"ATTACHED_DOCUMENT"`
	Notes         []core.RawHTMLWithLanguage `cardinality:"0.."                                                         json:"notes,omitempty"         order:"99" property:"NOTES"`
}

// NarrativeFields describes a told or inscribed thing which was written down.
//
//nolint:lll
type NarrativeFields struct {
	Title       []core.StringWithLanguage  `cardinality:"1.."                                                         json:"title"                 order:"1"  property:"NAME"`
	Endonym     []Endonym                  `cardinality:"0.."  duplicate:"top"                                        json:"endonym,omitempty"     order:"2"  property:"ENDONYM"`
	Description []core.RawHTMLWithLanguage `cardinality:"0.."                                                         json:"description,omitempty" order:"3"  property:"DESCRIPTION"`
	Genre       *core.Ref                  `cardinality:"0..1"                                                        json:"genre,omitempty"       order:"4"  property:"HAS_NARRATIVE_GENRE"     values:"core.peerdb.org,INSTANCE_OF=xeno.peerdb.org,NARRATIVE_GENRE"`
	Culture     []core.Ref                 `cardinality:"0.."                  inverseProperty:"HAS_CULTURAL_ELEMENT" json:"culture,omitempty"     order:"5"  property:"OF_CULTURE"              values:"core.peerdb.org,INSTANCE_OF=xeno.peerdb.org,CULTURE"`
	RecordedBy  []core.Ref                 `cardinality:"0.."                                                         json:"recordedBy,omitempty"  order:"6"  property:"RECORDED_BY"             values:"core.peerdb.org,INSTANCE_OF=xeno.peerdb.org,RESEARCHER"`
	RecordedAt  *core.Ref                  `cardinality:"0..1"                                                        json:"recordedAt,omitempty"  order:"7"  property:"RECORDED_AT"             values:"core.peerdb.org,INSTANCE_OF=xeno.peerdb.org,SITE"`
	RecordedOn  *core.Time                 `cardinality:"0..1"                                                        json:"recordedOn,omitempty"  order:"8"  property:"RECORDED_ON"`
	InSystem    []core.Ref                 `cardinality:"0.."                                                         json:"inSystem,omitempty"    order:"9"  property:"IN_COMMUNICATION_SYSTEM" values:"core.peerdb.org,INSTANCE_OF=xeno.peerdb.org,COMMUNICATION_SYSTEM"`
	Content     []core.RawHTMLWithLanguage `cardinality:"0.."                                                         json:"content,omitempty"     order:"10" property:"CONTENT"`
	Recording   []Recording                `cardinality:"0.."                                                         json:"recording,omitempty"   order:"90" property:"AUDIO"`
	Notes       []core.RawHTMLWithLanguage `cardinality:"0.."                                                         json:"notes,omitempty"       order:"99" property:"NOTES"`
}

// OrganismFields describes non-sapient life, which the catalogue keeps because a culture cannot be
// read without the things it eats, avoids and weaves with.
//
//nolint:lll
type OrganismFields struct {
	Name        []core.StringWithLanguage     `cardinality:"1.."                  json:"name"                  order:"1"  property:"NAME"`
	Endonym     []Endonym                     `cardinality:"0.."  duplicate:"top" json:"endonym,omitempty"     order:"2"  property:"ENDONYM"`
	TaxonCode   *core.Identifier              `cardinality:"0..1"                 json:"taxonCode,omitempty"   order:"3"  property:"TAXON_CODE"`
	Description []core.RawHTMLWithLanguage    `cardinality:"0.."                  json:"description,omitempty" order:"4"  property:"DESCRIPTION"`
	Category    *core.Ref                     `cardinality:"0..1"                 json:"category,omitempty"    order:"5"  property:"HAS_ORGANISM_CATEGORY" values:"core.peerdb.org,INSTANCE_OF=xeno.peerdb.org,ORGANISM_CATEGORY"`
	FoundOn     []core.Ref                    `cardinality:"0.."  duplicate:"top" json:"foundOn,omitempty"     order:"6"  property:"FOUND_ON"              values:"core.peerdb.org,INSTANCE_OF=xeno.peerdb.org,PLANET&core.peerdb.org,INSTANCE_OF=xeno.peerdb.org,MOON"`
	Biome       []core.Ref                    `cardinality:"0.."                  json:"biome,omitempty"       order:"7"  property:"HAS_BIOME"             values:"core.peerdb.org,INSTANCE_OF=xeno.peerdb.org,BIOME"`
	TypicalSize *core.AmountWithUnit[float64] `cardinality:"0..1"                 json:"typicalSize,omitempty" order:"8"  property:"TYPICAL_SIZE"`
	TypicalMass *core.AmountWithUnit[float64] `cardinality:"0..1"                 json:"typicalMass,omitempty" order:"9"  property:"TYPICAL_MASS"`
	Image       []Image                       `cardinality:"0.."                  json:"image,omitempty"       order:"90" property:"IMAGE"`
	Notes       []core.RawHTMLWithLanguage    `cardinality:"0.."                  json:"notes,omitempty"       order:"99" property:"NOTES"`
}

// InstituteFields describes a member body of the Consortium.
//
//nolint:lll
type InstituteFields struct {
	Name        []core.StringWithLanguage  `cardinality:"1.."  json:"name"                  order:"1"  property:"NAME"`
	ShortName   []core.StringWithLanguage  `cardinality:"0.."  json:"shortName,omitempty"   order:"2"  property:"SHORT_NAME"`
	Description []core.RawHTMLWithLanguage `cardinality:"0.."  json:"description,omitempty" order:"3"  property:"DESCRIPTION"`
	Founded     *core.Time                 `cardinality:"0..1" json:"founded,omitempty"     order:"4"  property:"FOUNDED"`
	LocatedAt   *core.Ref                  `cardinality:"0..1" json:"locatedAt,omitempty"   order:"5"  property:"LOCATED_AT"  values:"core.peerdb.org,INSTANCE_OF=xeno.peerdb.org,SITE&core.peerdb.org,INSTANCE_OF=xeno.peerdb.org,PLANET&core.peerdb.org,INSTANCE_OF=xeno.peerdb.org,MOON"`
	StaffCount  *core.Amount[int]          `cardinality:"0..1" json:"staffCount,omitempty"  order:"6"  property:"STAFF_COUNT"`
	Website     []core.Link                `cardinality:"0.."  json:"website,omitempty"     order:"7"  property:"WEBSITE"`
	Notes       []core.RawHTMLWithLanguage `cardinality:"0.."  json:"notes,omitempty"       order:"99" property:"NOTES"`
}

// ResearcherFields describes one xenoanthropologist. The given name and the family name are separate
// fields so that the display label can put them together itself.
//
//nolint:lll
type ResearcherFields struct {
	Name           []core.StringWithLanguage  `cardinality:"1.."                                                 json:"name"                     order:"1"  property:"NAME"`
	FamilyName     []core.StringWithLanguage  `cardinality:"1.."  default:"none"                                 json:"familyName"               order:"2"  property:"FAMILY_NAME"`
	ResearcherCode *core.Identifier           `cardinality:"0..1"                                                json:"researcherCode,omitempty" order:"3"  property:"RESEARCHER_CODE"`
	Description    []core.RawHTMLWithLanguage `cardinality:"0.."                                                 json:"description,omitempty"    order:"4"  property:"DESCRIPTION"`
	AffiliatedWith []core.Ref                 `cardinality:"0.."                 inverseProperty:"HAS_AFFILIATE" json:"affiliatedWith,omitempty" order:"5"  property:"AFFILIATED_WITH" values:"core.peerdb.org,INSTANCE_OF=xeno.peerdb.org,INSTITUTE"`
	SpecialisesIn  []core.Ref                 `cardinality:"0.."                                                 json:"specialisesIn,omitempty"  order:"6"  property:"SPECIALISES_IN"  values:"core.peerdb.org,INSTANCE_OF=xeno.peerdb.org,RESEARCH_METHOD"`
	ActivePeriod   *core.Interval[core.Time]  `cardinality:"0..1"                                                json:"activePeriod,omitempty"   order:"7"  property:"ACTIVE_PERIOD"`
	Born           *core.Time                 `cardinality:"0..1"                                                json:"born,omitempty"           order:"8"  property:"BORN"`
	Website        []core.Link                `cardinality:"0.."                                                 json:"website,omitempty"        order:"9"  property:"WEBSITE"`
	Image          []Image                    `cardinality:"0.."                                                 json:"image,omitempty"          order:"90" property:"IMAGE"`
	Notes          []core.RawHTMLWithLanguage `cardinality:"0.."                                                 json:"notes,omitempty"          order:"99" property:"NOTES"`
}

// ExpeditionFields describes a funded trip somewhere, which is the unit the Consortium budgets in.
//
//nolint:lll
type ExpeditionFields struct {
	Name          []core.StringWithLanguage  `cardinality:"1.."                                                   json:"name"                    order:"1"  property:"NAME"`
	CatalogueCode *core.Identifier           `cardinality:"0..1"                                                  json:"catalogueCode,omitempty" order:"2"  property:"CATALOGUE_CODE"`
	Description   []core.RawHTMLWithLanguage `cardinality:"0.."                                                   json:"description,omitempty"   order:"3"  property:"DESCRIPTION"`
	Destination   []core.Ref                 `cardinality:"1.."  duplicate:"top"                                  json:"destination"             order:"4"  property:"HAS_DESTINATION"       values:"core.peerdb.org,INSTANCE_OF=xeno.peerdb.org,PLANET&core.peerdb.org,INSTANCE_OF=xeno.peerdb.org,MOON&core.peerdb.org,INSTANCE_OF=xeno.peerdb.org,REGION"`
	Period        *core.Interval[core.Time]  `cardinality:"0..1"                                                  json:"period,omitempty"        order:"5"  property:"PERIOD"`
	LedBy         []core.Ref                 `cardinality:"0.."                                                   json:"ledBy,omitempty"         order:"6"  property:"LED_BY"                values:"core.peerdb.org,INSTANCE_OF=xeno.peerdb.org,RESEARCHER"`
	TeamMember    []core.Ref                 `cardinality:"0.."  duplicate:"top"                                  json:"teamMember,omitempty"    order:"7"  property:"HAS_TEAM_MEMBER"       values:"core.peerdb.org,INSTANCE_OF=xeno.peerdb.org,RESEARCHER"`
	OrganisedBy   []core.Ref                 `cardinality:"0.."                  inverseProperty:"RAN_EXPEDITION" json:"organisedBy,omitempty"   order:"8"  property:"ORGANISED_BY"          values:"core.peerdb.org,INSTANCE_OF=xeno.peerdb.org,INSTITUTE"`
	Method        []core.Ref                 `cardinality:"0.."                                                   json:"method,omitempty"        order:"9"  property:"USES_METHOD"           values:"core.peerdb.org,INSTANCE_OF=xeno.peerdb.org,RESEARCH_METHOD"`
	Ethics        *core.Ref                  `cardinality:"0..1"                                                  json:"ethics,omitempty"        order:"10" property:"UNDER_ETHICS_PROTOCOL" values:"core.peerdb.org,INSTANCE_OF=xeno.peerdb.org,ETHICS_PROTOCOL"`
	Budget        *core.AmountWithUnit[int]  `cardinality:"0..1"                                                  json:"budget,omitempty"        order:"11" property:"BUDGET"`
	Report        []Attachment               `cardinality:"0.."                                                   json:"report,omitempty"        order:"90" property:"HAS_REPORT"`
	Notes         []core.RawHTMLWithLanguage `cardinality:"0.."                                                   json:"notes,omitempty"         order:"99" property:"NOTES"`
}

// ObservationFields describes one field note: the smallest thing the catalogue records.
//
//nolint:lll
type ObservationFields struct {
	Title           []core.StringWithLanguage  `cardinality:"1.."                                                                               json:"title"                     order:"1"  property:"NAME"`
	Description     []core.RawHTMLWithLanguage `cardinality:"0.."                                                                               json:"description,omitempty"     order:"2"  property:"DESCRIPTION"`
	Content         []core.RawHTMLWithLanguage `cardinality:"0.."                                                                               json:"content,omitempty"         order:"3"  property:"CONTENT"`
	Expedition      *core.Ref                  `cardinality:"0..1"                                            inverseProperty:"HAS_OBSERVATION" json:"expedition,omitempty"      order:"4"  property:"PART_OF_EXPEDITION" values:"core.peerdb.org,INSTANCE_OF=xeno.peerdb.org,EXPEDITION"`
	ObservedBy      []core.Ref                 `cardinality:"0.."  duplicate:"top"                                                              json:"observedBy,omitempty"      order:"5"  property:"OBSERVED_BY"        values:"core.peerdb.org,INSTANCE_OF=xeno.peerdb.org,RESEARCHER"`
	ObservedAt      *core.Ref                  `cardinality:"0..1"                                                                              json:"observedAt,omitempty"      order:"6"  property:"OBSERVED_AT"        values:"core.peerdb.org,INSTANCE_OF=xeno.peerdb.org,SITE"`
	ObservedOn      *core.Time                 `cardinality:"0..1"                                                                              json:"observedOn,omitempty"      order:"7"  property:"OBSERVED_ON"`
	About           []core.Ref                 `cardinality:"0.."  duplicate:"top"                                                              json:"about,omitempty"           order:"8"  property:"ABOUT"              values:"core.peerdb.org,INSTANCE_OF=xeno.peerdb.org,SPECIES&core.peerdb.org,INSTANCE_OF=xeno.peerdb.org,CULTURE&core.peerdb.org,INSTANCE_OF=xeno.peerdb.org,PRACTICE&core.peerdb.org,INSTANCE_OF=xeno.peerdb.org,ORGANISM"`
	Method          []core.Ref                 `cardinality:"0.."                                                                               json:"method,omitempty"          order:"9"  property:"USES_METHOD"        values:"core.peerdb.org,INSTANCE_OF=xeno.peerdb.org,RESEARCH_METHOD"`
	FieldConditions *string                    `cardinality:"0..1"                 inputComponent:"InputHTML"                                   json:"fieldConditions,omitempty" order:"10" property:"FIELD_CONDITIONS"`
	Image           []Image                    `cardinality:"0.."                                                                               json:"image,omitempty"           order:"90" property:"IMAGE"`
	Notes           []core.RawHTMLWithLanguage `cardinality:"0.."                                                                               json:"notes,omitempty"           order:"99" property:"NOTES"`
}

// InterviewSubject groups who was spoken to and where.
//
//nolint:lll
type InterviewSubject struct {
	Title       []core.StringWithLanguage  `cardinality:"1.."                                                    json:"title"                 order:"1" property:"NAME"`
	Description []core.RawHTMLWithLanguage `cardinality:"0.."                                                    json:"description,omitempty" order:"2" property:"DESCRIPTION"`
	Interviewee []core.Ref                 `cardinality:"1.."  duplicate:"top"                                   json:"interviewee"           order:"3" property:"HAS_INTERVIEWEE"    values:"core.peerdb.org,INSTANCE_OF=xeno.peerdb.org,INDIVIDUAL&core.peerdb.org,INSTANCE_OF=xeno.peerdb.org,COLLECTIVE"`
	Interviewer []core.Ref                 `cardinality:"1.."  duplicate:"top"                                   json:"interviewer"           order:"4" property:"HAS_INTERVIEWER"    values:"core.peerdb.org,INSTANCE_OF=xeno.peerdb.org,RESEARCHER"`
	RecordedAt  *core.Ref                  `cardinality:"0..1"                                                   json:"recordedAt,omitempty"  order:"5" property:"RECORDED_AT"        values:"core.peerdb.org,INSTANCE_OF=xeno.peerdb.org,SITE"`
	RecordedOn  *core.Time                 `cardinality:"0..1"                                                   json:"recordedOn,omitempty"  order:"6" property:"RECORDED_ON"`
	Expedition  *core.Ref                  `cardinality:"0..1"                 inverseProperty:"HAS_OBSERVATION" json:"expedition,omitempty"  order:"7" property:"PART_OF_EXPEDITION" values:"core.peerdb.org,INSTANCE_OF=xeno.peerdb.org,EXPEDITION"`
}

// InterviewRecord groups the record itself: what was said and in which system.
//
//nolint:lll
type InterviewRecord struct {
	InSystem  []core.Ref                 `cardinality:"0.."  json:"inSystem,omitempty"  order:"1" property:"IN_COMMUNICATION_SYSTEM" values:"core.peerdb.org,INSTANCE_OF=xeno.peerdb.org,COMMUNICATION_SYSTEM"`
	Duration  *core.AmountWithUnit[int]  `cardinality:"0..1" json:"duration,omitempty"  order:"2" property:"DURATION"`
	Content   []core.RawHTMLWithLanguage `cardinality:"0.."  json:"content,omitempty"   order:"3" property:"CONTENT"`
	Recording []Recording                `cardinality:"0.."  json:"recording,omitempty" order:"4" property:"AUDIO"`
}

// InterviewClearance groups the ethics apparatus around a restricted record: under which protocol it
// was taken, what the interviewee agreed to, and who may read it.
//
// The cleared readers are edited with the input which offers the users the document itself grants
// access to, so naming a reader here means picking somebody who has already been given access on the
// permissions tab, rather than typing a subject by hand.
//
//nolint:lll
type InterviewClearance struct {
	Ethics        *core.Ref                  `cardinality:"0..1"                                                               json:"ethics,omitempty"        order:"1" property:"UNDER_ETHICS_PROTOCOL" values:"core.peerdb.org,INSTANCE_OF=xeno.peerdb.org,ETHICS_PROTOCOL"`
	ConsentNote   []core.RawHTMLWithLanguage `cardinality:"0.."                                                                json:"consentNote,omitempty"   order:"2" property:"CONSENT_NOTE"`
	ClearedReader []core.Identifier          `cardinality:"0.."  duplicate:"top" inputComponent:"InputIdentityFromPermissions" json:"clearedReader,omitempty" order:"3" property:"HAS_CLEARED_READER"`
	Notes         []core.RawHTMLWithLanguage `cardinality:"0.."                                                                json:"notes,omitempty"         order:"4" property:"NOTES"`
}

// InterviewFields is the field schema of an interview record, in the three sections it is read in.
type InterviewFields struct {
	InterviewSubject   `order:"1" section:"subject"`
	InterviewRecord    `order:"2" section:"record"`
	InterviewClearance `order:"3" section:"clearance"`
}

// PublicationFields describes a paper, which is what the whole apparatus is ultimately for.
//
//nolint:lll
type PublicationFields struct {
	Title       []core.StringWithLanguage  `cardinality:"1.."                                             json:"title"                 order:"1"  property:"NAME"`
	DOI         *core.Identifier           `cardinality:"0..1"                                            json:"doi,omitempty"         order:"2"  property:"DOI"`
	Abstract    []core.RawHTMLWithLanguage `cardinality:"0.."                                             json:"abstract,omitempty"    order:"3"  property:"ABSTRACT"`
	Author      []core.Ref                 `cardinality:"1.."  duplicate:"top"                            json:"author"                order:"4"  property:"HAS_AUTHOR"        values:"core.peerdb.org,INSTANCE_OF=xeno.peerdb.org,RESEARCHER"`
	Journal     *string                    `cardinality:"0..1"                                            json:"journal,omitempty"     order:"5"  property:"JOURNAL"`
	PublishedOn *core.Time                 `cardinality:"0..1"                                            json:"publishedOn,omitempty" order:"6"  property:"PUBLISHED_ON"`
	About       []core.Ref                 `cardinality:"0.."  duplicate:"top"                            json:"about,omitempty"       order:"7"  property:"ABOUT"             values:"core.peerdb.org,INSTANCE_OF=xeno.peerdb.org,SPECIES&core.peerdb.org,INSTANCE_OF=xeno.peerdb.org,CULTURE&core.peerdb.org,INSTANCE_OF=xeno.peerdb.org,PRACTICE&core.peerdb.org,INSTANCE_OF=xeno.peerdb.org,ORGANISM"`
	Cites       []core.Ref                 `cardinality:"0.."  duplicate:"top" inverseProperty:"CITED_BY" json:"cites,omitempty"       order:"8"  property:"CITES"             values:"core.peerdb.org,INSTANCE_OF=xeno.peerdb.org,PUBLICATION"`
	Document    []Attachment               `cardinality:"0.."                                             json:"document,omitempty"    order:"90" property:"ATTACHED_DOCUMENT"`
	Notes       []core.RawHTMLWithLanguage `cardinality:"0.."                                             json:"notes,omitempty"       order:"99" property:"NOTES"`
}
