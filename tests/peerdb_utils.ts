import type { Page } from "@playwright/test"

import { Identifier } from "@tozd/identifier"

import * as core from "@/core"
import { createNamed as createNamedDocument, type CreateNamedOptions, searchByProperty, type SearchOptions, startCreate as startCreateClass } from "./utils"

// The namespace the test data documents live in (Namespace in internal/xeno/namespace.go), and the
// segment between it and a file's path in the identifier a test data attachment is stored under
// (FilesStorage there).
export const Namespace = "xeno.peerdb.org"
export const FilesStorage = "TEST_STORAGE"

// The identifier a document of the test data is stored under, derived from the parts of its base the way
// the application derives it. A test names a document by the class mnemonic and the key which say which
// document of the test data it is, instead of by the opaque string the two hash to. The parts are exactly
// the "id" of the document's JSON file in testdata, without the leading namespace.
export async function documentIdOf(...base: Array<string>): Promise<string> {
  return (await Identifier.from(Namespace, ...base)).toString()
}

// The same for a document of the core namespace: the schema documents (classes, properties, value types)
// and the vocabularies PeerDB inserts itself (the languages and the units among them).
export async function coreDocumentIdOf(...base: Array<string>): Promise<string> {
  return (await Identifier.from(core.Namespace, ...base)).toString()
}

// The address a test data attachment is served from. The file is stored under an identifier derived from
// its path inside the test data files directory, so the address follows from that path alone and a test
// can ask for a file without reading it out of a document it may not be allowed to read.
export async function filePathOf(path: string): Promise<string> {
  return `/f/${(await Identifier.from(Namespace, FilesStorage, path)).toString()}`
}

// Derives the identifier of every mnemonic of a namespace at once, so that the tables below stay a list of
// mnemonics (which is what the Go schema declares) rather than a list of opaque strings.
async function mnemonicIds<T extends string>(namespace: string, mnemonics: ReadonlyArray<T>): Promise<Record<T, string>> {
  const ids = {} as Record<T, string>
  for (const mnemonic of mnemonics) {
    ids[mnemonic] = (await Identifier.from(namespace, mnemonic)).toString()
  }
  return ids
}

// The classes the test data declares which hold documents of their own, in the order the containment
// chain and then the catalogue go (classes.go in internal/xeno). Tests which walk every class iterate over
// this, so a class added to the schema without a test here is easy to notice.
const DOCUMENT_CLASS_MNEMONICS = [
  "GALAXY",
  "SECTOR",
  "STAR_SYSTEM",
  "PLANET",
  "MOON",
  "REGION",
  "SITE",
  "SPECIES",
  "INDIVIDUAL",
  "COLLECTIVE",
  "CULTURE",
  "PRACTICE",
  "COMMUNICATION_SYSTEM",
  "ARTIFACT",
  "NARRATIVE",
  "ORGANISM",
  "INSTITUTE",
  "RESEARCHER",
  "EXPEDITION",
  "OBSERVATION",
  "INTERVIEW",
  "PUBLICATION",
] as const

// The classes the test data declares which no document is an instance of: they exist to gather the ones
// below them, which is what makes the class tree of the create view and the class facet nest.
const ABSTRACT_CLASS_MNEMONICS = ["PLACE", "WORLD", "BEING", "CULTURAL_ELEMENT", "RESEARCH_RECORD"] as const

// The controlled vocabularies of the test data (vocabularyClasses in internal/xeno/classes.go). An entry of
// one of them is a document like any other, so they are classes a test can search by and create in.
const VOCABULARY_CLASS_MNEMONICS = [
  "PLANET_TYPE",
  "BIOME",
  "SITE_TYPE",
  "CONTACT_STATUS",
  "SENSORY_MODALITY",
  "SUBSISTENCE_MODE",
  "SOCIAL_ORGANISATION",
  "KINSHIP_SYSTEM",
  "INDIVIDUALITY_MODE",
  "ORGANISM_CATEGORY",
  "ARTIFACT_CATEGORY",
  "PRACTICE_CATEGORY",
  "NARRATIVE_GENRE",
  "RESEARCH_METHOD",
  "ETHICS_PROTOCOL",
  "COMMUNICATION_MODALITY",
] as const

// Every property the test data schema declares (properties.go in internal/xeno). A test addresses a
// property by its mnemonic, because both the search filters and the form fields carry the identifier of
// the property they hold in their class name, and a search can be narrowed by passing the property as a
// query parameter, so addressing one by anything else would depend on its label, which differs between
// the three interface languages.
const PROPERTY_MNEMONICS = [
  "ABOUT",
  "ABSTRACT",
  "ACCESSION_CODE",
  "ACTIVE_PERIOD",
  "AFFILIATED_WITH",
  "ALSO_FOUND_ON",
  "AREA",
  "ATMOSPHERE",
  "ATTACHED_DOCUMENT",
  "AUDIO",
  "AXIS",
  "BELONGS_TO_CULTURE",
  "BIOSPHERE",
  "BIRTHPLACE",
  "BODY_PLAN",
  "BORN",
  "BUDGET",
  "CAPTION",
  "CATALOGUE_CODE",
  "CITED_BY",
  "CITES",
  "CLASSIFIED_AS",
  "COLLECTED_BY",
  "CONSENT_NOTE",
  "CONTAINED_IN",
  "CONTAINS",
  "DATE_MADE",
  "DAY_LENGTH",
  "DIAMETER",
  "DIED",
  "DIMENSION",
  "DISTANCE_FROM_SOL",
  "DOI",
  "DURATION",
  "ELEVATION_RANGE",
  "ENDONYM",
  "FAMILY_NAME",
  "FIELD_CONDITIONS",
  "FIRST_CONTACT",
  "FIRST_DOCUMENTED",
  "FIRST_SURVEYED",
  "FORM_OF_ADDRESS",
  "FOUND_AT",
  "FOUNDED",
  "FOUND_ON",
  "GLOSS",
  "GRID_REFERENCE",
  "HAS_AFFILIATE",
  "HAS_ARTIFACT_CATEGORY",
  "HAS_AUTHOR",
  "HAS_BIOME",
  "HAS_CLEARED_READER",
  "HAS_COMMUNICATION_MODALITY",
  "HAS_CONTACT_STATUS",
  "HAS_CULTURAL_ELEMENT",
  "HAS_DESTINATION",
  "HAS_DOCUMENTED_MEMBER",
  "HAS_HOMEWORLD",
  "HAS_INDIVIDUALITY_MODE",
  "HAS_INTERVIEWEE",
  "HAS_INTERVIEWER",
  "HAS_KINSHIP_SYSTEM",
  "HAS_MEMBER",
  "HAS_NARRATIVE_GENRE",
  "HAS_NOTATION_SYSTEM",
  "HAS_OBSERVATION",
  "HAS_ORGANISM_CATEGORY",
  "HAS_PLANET_TYPE",
  "HAS_PRACTICE_CATEGORY",
  "HAS_REPORT",
  "HAS_RING_SYSTEM",
  "HAS_SENSORY_MODALITY",
  "HAS_SITE_TYPE",
  "HAS_SOCIAL_ORGANISATION",
  "HAS_SUBSISTENCE_MODE",
  "HAS_TEAM_MEMBER",
  "HOME_TO_SPECIES",
  "HOSTS_CULTURE",
  "HYDROSPHERE",
  "IMAGE",
  "IN_COMMUNICATION_SYSTEM",
  "JOURNAL",
  "LED_BY",
  "LIFESPAN",
  "LINEAGE_NAME",
  "LOCATED_AT",
  "MASS",
  "MATERIAL",
  "MEAN_TEMPERATURE",
  "MEMBER_COUNT",
  "NOTES",
  "OBSERVED_AT",
  "OBSERVED_BY",
  "OBSERVED_ON",
  "OCCUPATION_PERIOD",
  "OF_CULTURE",
  "OF_SPECIES",
  "ORBITAL_PERIOD",
  "ORGANISED_BY",
  "PARTICIPANT_COUNT",
  "PART_OF_EXPEDITION",
  "PERIOD",
  "PERIODICITY",
  "PLANET_COUNT",
  "POPULATION_ESTIMATE",
  "PRACTISED_BY",
  "PRESENT_AT",
  "PUBLISHED_ON",
  "RADIUS",
  "RAN_EXPEDITION",
  "RECORDED_AT",
  "RECORDED_BY",
  "RECORDED_ON",
  "RELATED_LOCATION",
  "RELATED_PERSON",
  "RELATED_PRACTICE",
  "RESEARCHER_CODE",
  "ROLE",
  "SAMPLE_GLOSS",
  "SOURCE",
  "SPEAKER_ESTIMATE",
  "SPECIALISES_IN",
  "SPECTRAL_CLASS",
  "STAFF_COUNT",
  "STAR_COUNT",
  "SURFACE_GRAVITY",
  "SURVEY_PERIOD",
  "TAXON_CODE",
  "TIDALLY_LOCKED",
  "TYPICAL_HEIGHT",
  "TYPICAL_MASS",
  "TYPICAL_SIZE",
  "UNDER_ETHICS_PROTOCOL",
  "USED_BY_SPECIES",
  "USES_COMMUNICATION_SYSTEM",
  "USES_METHOD",
  "WEBSITE",
] as const

export type DocumentClass = (typeof DOCUMENT_CLASS_MNEMONICS)[number]
export type AbstractClass = (typeof ABSTRACT_CLASS_MNEMONICS)[number]
export type VocabularyClass = (typeof VOCABULARY_CLASS_MNEMONICS)[number]
export type EntityClass = DocumentClass | AbstractClass | VocabularyClass

// The classes which hold documents, which is what a test walking the catalogue iterates over.
export const DOCUMENT_CLASSES: ReadonlyArray<DocumentClass> = DOCUMENT_CLASS_MNEMONICS
// The vocabularies, whose documents are all of the same shape, so one test over them covers them all.
export const VOCABULARY_CLASSES: ReadonlyArray<VocabularyClass> = VOCABULARY_CLASS_MNEMONICS
// Every class of the test data schema, the abstract ones included.
export const ENTITY_CLASSES: ReadonlyArray<EntityClass> = [...DOCUMENT_CLASS_MNEMONICS, ...ABSTRACT_CLASS_MNEMONICS, ...VOCABULARY_CLASS_MNEMONICS]

// The document identifier of every class of the test data schema, so a test can pick a class without
// depending on its label, which differs between the three languages the site is served in.
export const CLASS_IDS: Record<EntityClass, string> = {
  ...(await mnemonicIds(Namespace, DOCUMENT_CLASS_MNEMONICS)),
  ...(await mnemonicIds(Namespace, ABSTRACT_CLASS_MNEMONICS)),
  ...(await mnemonicIds(Namespace, VOCABULARY_CLASS_MNEMONICS)),
}

// The document identifier of the core classes a test addresses. They are the classes of the schema itself,
// which every site holds whatever data it carries.
export const CORE_CLASS_IDS = {
  CLASS: core.CLASS,
  LANGUAGE: core.LANGUAGE,
  PAGE: core.PAGE,
  PERMISSION_ACTIONS: core.PERMISSION_ACTIONS,
  PROPERTY: core.PROPERTY,
  UNIT: core.UNIT,
  VALUE_TYPE: core.VALUE_TYPE,
  VOCABULARY: core.VOCABULARY,
} as const

// The document identifier of every property a test addresses an element by: the ones the test data schema
// declares, and the core ones it builds on, which are imported from the application's own constants so
// that a change to a namespace or to a mnemonic reaches the tests through them.
export const PROPERTY_IDS = {
  ...(await mnemonicIds(Namespace, PROPERTY_MNEMONICS)),
  ALTERNATIVE_NAME: core.ALTERNATIVE_NAME,
  CARDINALITY: core.CARDINALITY,
  CODE: core.CODE,
  DESCRIPTION: core.DESCRIPTION,
  FIELD: core.FIELD,
  FIELDS: core.FIELDS,
  HAS_PERMISSION: core.HAS_PERMISSION,
  HAS_PROPERTY: core.HAS_PROPERTY,
  HAS_REQUESTED_PERMISSION: core.HAS_REQUESTED_PERMISSION,
  HAS_VALUE_TYPE: core.HAS_VALUE_TYPE,
  INSTANCE_OF: core.INSTANCE_OF,
  IN_LANGUAGE: core.IN_LANGUAGE,
  IN_UNIT: core.IN_UNIT,
  LIST: core.LIST,
  MNEMONIC: core.MNEMONIC,
  NAME: core.NAME,
  ORDER_IN_LIST: core.ORDER_IN_LIST,
  PERMISSION_SCOPE: core.PERMISSION_SCOPE,
  PERMISSION_USER: core.PERMISSION_USER,
  SEARCH_SHORTCUT: core.SEARCH_SHORTCUT,
  SECTION: core.SECTION,
  SUBCLASS_OF: core.SUBCLASS_OF,
  SUBENTITY_OF: core.SUBENTITY_OF,
  SUBPROPERTY_OF: core.SUBPROPERTY_OF,
  SUB_FIELD: core.SUB_FIELD,
  TITLE: core.TITLE,
} as const

// The languages the site is served in (languagePriority in config.yml). Tests which have to look the same
// in all of them iterate over this.
export const LANGUAGES = ["en", "sl", "pt"] as const

export type Language = (typeof LANGUAGES)[number]

// The roles the site declares (roles in config.yml), which are the ones the mock authenticator offers to
// sign in with. The reserved empty role is not among them: it is what a request holds without signing in
// at all, and what every signed-in user holds on top of whatever else they were given.
export const ROLES = ["admin", "author", "bulk", "curator", "ethics", "researcher", "surveyor"] as const

export type Role = (typeof ROLES)[number]

// Which classes each role may start a new document of, as identifiers, because a role which may create a
// class of the schema itself (the curator opens new units) cannot be written as a mnemonic of the test
// data. Creating is the action the working roles differ on, so a test asserting what the create view
// offers reads what it expects out of the same table the site is configured by. A role which is not
// listed here creates nothing.
export const ROLE_CREATE_IDS: Partial<Record<Role, ReadonlyArray<string>>> = {}

// The classes of the test data each role may start a new document of, by mnemonic.
const ROLE_CREATES_MNEMONICS: Partial<Record<Role, ReadonlyArray<EntityClass>>> = {
  surveyor: ["GALAXY", "SECTOR", "STAR_SYSTEM", "PLANET", "MOON", "REGION", "SITE"],
  researcher: ["OBSERVATION", "INTERVIEW", "SPECIES", "INDIVIDUAL", "COLLECTIVE", "CULTURE", "PRACTICE", "COMMUNICATION_SYSTEM", "NARRATIVE", "ORGANISM"],
  author: ["PUBLICATION"],
  curator: [
    "ARTIFACT",
    "INSTITUTE",
    "RESEARCHER",
    "EXPEDITION",
    "PLANET_TYPE",
    "BIOME",
    "SITE_TYPE",
    "CONTACT_STATUS",
    "SENSORY_MODALITY",
    "SUBSISTENCE_MODE",
    "SOCIAL_ORGANISATION",
    "KINSHIP_SYSTEM",
    "INDIVIDUALITY_MODE",
    "ORGANISM_CATEGORY",
    "ARTIFACT_CATEGORY",
    "PRACTICE_CATEGORY",
    "NARRATIVE_GENRE",
    "RESEARCH_METHOD",
    "COMMUNICATION_MODALITY",
  ],
  ethics: ["ETHICS_PROTOCOL"],
}

// The curator also opens new units, which are documents of the core schema rather than of the test data,
// so they are added to the table as an identifier once it is built.
for (const [role, classes] of Object.entries(ROLE_CREATES_MNEMONICS) as Array<[Role, ReadonlyArray<EntityClass>]>) {
  ROLE_CREATE_IDS[role] = classes.map((entityClass) => CLASS_IDS[entityClass])
}
ROLE_CREATE_IDS.curator = [...(ROLE_CREATE_IDS.curator ?? []), CORE_CLASS_IDS.UNIT]

// Which classes of the test data each role may start a new document of, for a test which needs the
// mnemonics rather than the identifiers.
export const ROLE_CREATES = ROLE_CREATES_MNEMONICS

// The class the site keeps out of the public read scope, so that the test data has something to
// demonstrate document-level permissions with: an interview is reachable only through the permission
// claims it carries itself, or through the two roles which are granted the class outright.
export const RESTRICTED_CLASS: DocumentClass = "INTERVIEW"

// Searches for all documents which are an instance of the given class of the test data schema.
export async function searchByClass(page: Page, entityClass: EntityClass, options: SearchOptions = {}): Promise<void> {
  await searchByProperty(page, PROPERTY_IDS.INSTANCE_OF, CLASS_IDS[entityClass], options)
}

// Searches for all documents which are an instance of the given core class, which is how the documents of
// the schema itself (the classes, the properties, the units) are reached.
export async function searchByCoreClass(page: Page, coreClass: keyof typeof CORE_CLASS_IDS, options: SearchOptions = {}): Promise<void> {
  await searchByProperty(page, PROPERTY_IDS.INSTANCE_OF, CORE_CLASS_IDS[coreClass], options)
}

// Starts creating a document of the given class of the test data schema.
export async function startCreate(page: Page, entityClass: EntityClass): Promise<void> {
  await startCreateClass(page, CLASS_IDS[entityClass])
}

// What createNamed may be asked to do beyond what creating a named document already takes.
export interface CreateNamedClassOptions extends CreateNamedOptions {
  // The property the name is written into, for a class which names its documents by something other than
  // their name. Defaults to the name property, which is what every class of the test data names by.
  property?: string
}

// Creates a document of the given class with nothing but its name filled in and returns the identifier it
// was saved under, leaving the browser on the document view of what was created.
export async function createNamed(page: Page, entityClass: EntityClass, name: string, options: CreateNamedClassOptions = {}): Promise<string> {
  return await createNamedDocument(page, CLASS_IDS[entityClass], options.property ?? PROPERTY_IDS.NAME, name, options)
}
