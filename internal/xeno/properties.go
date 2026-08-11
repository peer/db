package xeno

import (
	"gitlab.com/tozd/go/errors"

	"gitlab.com/peerdb/peerdb/core"
)

// Properties returns the properties of the test data schema.
//
// The properties are grouped the way the catalogue thinks about them: a few generic ones, then
// the ones about places, beings, culture, and the research apparatus itself. Several of them are
// deliberately arranged into subproperty trees (see RELATED_LOCATION, RELATED_PERSON, CLASSIFIED_AS
// and PERIOD), so that a site which indexes ancestor properties has something to show for it: a
// search for "related location" then finds a homeworld, a find spot and an expedition destination
// alike.
//
//nolint:funlen,maintidx,lll
func Properties() ([]any, errors.E) {
	properties := []*core.Property{} //nolint:prealloc

	// Generic properties, shared across most classes.

	properties = append(properties,
		property("ENDONYM", "endonym", "endonim", "endónimo",
			describes(
				"<p>What the inhabitants call the thing, as opposed to the name the survey gave it.</p>",
				"<p>Ime, ki ga uporabljajo prebivalci, za razliko od imena, ki ga je določil pregled.</p>", "<p>O nome que os habitantes dão à coisa, por oposição ao nome que o levantamento lhe deu.</p>",
			),
			alternativeNames("local name", "krajevno ime", "nome local"),
		),
		property("GLOSS", "gloss", "pomen", "glosa",
			describes(
				"<p>What a name means in the language it comes from, rendered plainly.</p>",
				"<p>Pomen imena v jeziku, iz katerega izhaja, izražen preprosto.</p>", "<p>O que um nome significa na língua de onde vem, dito de forma simples.</p>",
			),
		),
		property("CATALOGUE_CODE", "catalogue code", "kataloška oznaka", "código de catálogo",
			describes(
				"<p>The code under which the Consortium registry lists the entry.</p>",
				"<p>Oznaka, pod katero je vnos zaveden v registru konzorcija.</p>", "<p>O código sob o qual o registo do Consórcio lista a entrada.</p>",
			),
			linkTemplate("https://registry.ccx.example/entry/{value}"),
			notTextSearchable(),
		),
		property("CONTAINED_IN", "contained in", "vsebovan v", "contido em",
			subpropertyOfCore("SUBENTITY_OF"),
			describes(
				"<p>The larger place this place is part of. Following the chain upwards reaches a galaxy.</p>",
				"<p>Večji kraj, katerega del je ta kraj. Veriga navzgor se konča pri galaksiji.</p>", "<p>O lugar maior de que este lugar faz parte. Seguindo a cadeia para cima chega-se a uma galáxia.</p>",
			),
		),
		property("CONTAINS", "contains", "vsebuje", "contém", inverseOf("CONTAINED_IN")),
		property("NOTES", "notes", "opombe", "notas"),
		property("CAPTION", "caption", "napis", "legenda"),
		property("SOURCE", "source", "vir", "fonte"),
		property("IMAGE", "image", "slika", "imagem"),
		property("ATTACHED_DOCUMENT", "attached document", "priloženi dokument", "documento anexo"),
		property("AUDIO", "audio recording", "zvočni posnetek", "gravação áudio"),
		property("WEBSITE", "website", "spletna stran", "página web"),
		property("PERIOD", "period", "obdobje", "período",
			describes(
				"<p>A stretch of time something spans. Specialised periods are subproperties of this one.</p>",
				"<p>Časovni razpon, ki ga nekaj obsega. Posebna obdobja so podlastnosti te lastnosti.</p>", "<p>Um intervalo de tempo que algo abrange. Os períodos especializados são subpropriedades desta.</p>",
			),
		),
		property("CLASSIFIED_AS", "classified as", "uvrščen v", "classificado como",
			describes(
				"<p>Any of the controlled vocabularies the catalogue files an entry under. The specific classifications are subproperties of this one.</p>",
				"<p>Katerikoli nadzorovani besednjak, po katerem katalog razvršča vnos. Posamezne razvrstitve so podlastnosti te lastnosti.</p>", "<p>Qualquer um dos vocabulários controlados sob os quais o catálogo arquiva uma entrada. As classificações específicas são subpropriedades desta.</p>",
			),
		),
		property("RELATED_LOCATION", "related place", "povezani kraj", "lugar relacionado",
			describes(
				"<p>Any place an entry is tied to. The specific ties, from homeworld to find spot, are subproperties of this one.</p>",
				"<p>Katerikoli kraj, s katerim je vnos povezan. Posamezne vezi, od matičnega sveta do najdišča, so podlastnosti te lastnosti.</p>", "<p>Qualquer lugar a que uma entrada esteja ligada. As ligações específicas, do mundo natal ao local do achado, são subpropriedades desta.</p>",
			),
		),
		property("RELATED_PERSON", "related researcher", "povezani raziskovalec", "investigador relacionado",
			describes(
				"<p>Any researcher an entry is tied to, whoever led, collected, recorded or wrote it.</p>",
				"<p>Katerikoli raziskovalec, s katerim je vnos povezan, naj je vodil, zbiral, snemal ali pisal.</p>", "<p>Qualquer investigador a que uma entrada esteja ligada, quem quer que a tenha liderado, recolhido, registado ou escrito.</p>",
			),
		),
	)

	// Places: from galaxies down to field sites.

	properties = append(properties,
		property("SURVEY_PERIOD", "survey period", "obdobje pregleda", "período de levantamento", subpropertyOf("PERIOD")),
		property("FIRST_SURVEYED", "first surveyed", "prvič pregledano", "primeiro levantamento"),
		property("DIAMETER", "diameter", "premer", "diâmetro"),
		property("SPECTRAL_CLASS", "spectral class", "spektralni razred", "classe espectral"),
		property("STAR_COUNT", "number of stars", "število zvezd", "número de estrelas"),
		property("DISTANCE_FROM_SOL", "distance from Sol", "razdalja od Sonca", "distância ao Sol"),
		property("PLANET_COUNT", "number of planets", "število planetov", "número de planets"),
		property("HAS_PLANET_TYPE", "world type", "vrsta sveta", "tipo de mundo", subpropertyOf("CLASSIFIED_AS")),
		property("HAS_BIOME", "biome", "biom", "bioma", subpropertyOf("CLASSIFIED_AS")),
		property("RADIUS", "radius", "polmer", "raio"),
		property("MASS", "mass", "masa", "massa"),
		property("SURFACE_GRAVITY", "surface gravity", "težnost na površju", "gravidade à superfície"),
		property("MEAN_TEMPERATURE", "mean surface temperature", "povprečna temperatura površja", "temperatura média à superfície"),
		property("DAY_LENGTH", "length of day", "dolžina dneva", "duração do dia"),
		property("ORBITAL_PERIOD", "orbital period", "obhodna doba", "período orbital"),
		property("ATMOSPHERE", "atmosphere", "ozračje", "atmosfera"),
		property("HYDROSPHERE", "surface liquid cover", "delež tekočine na površju", "cobertura líquida da superfície"),
		property("BIOSPHERE", "biosphere", "biosfera", "biosfera",
			describes(
				"<p>What lives there, in summary. A world surveyed and found sterile carries this with no value at all; a world not yet looked at carries it as unknown.</p>",
				"<p>Povzetek tega, kaj tam živi. Svet, ki je bil pregledan in je brez življenja, ima to lastnost brez vrednosti, nepregledan svet pa kot neznano.</p>", "<p>Resumo do que ali vive. Um mundo cujo levantamento o encontrou estéril tem esta propriedade sem valor algum; um mundo ainda não observado tem-na como desconhecida.</p>",
			),
		),
		property("HAS_RING_SYSTEM", "has a ring system", "ima obroče", "tem sistema de anéis"),
		property("TIDALLY_LOCKED", "tidally locked", "vezane rotacije", "rotação síncrona"),
		property("HAS_CONTACT_STATUS", "contact status", "stanje stikov", "estado de contacto", subpropertyOf("CLASSIFIED_AS")),
		property("AREA", "area", "površina", "área"),
		property("ELEVATION_RANGE", "elevation range", "višinski razpon", "intervalo de altitude"),
		property("HAS_SITE_TYPE", "site type", "vrsta najdišča", "tipo de sítio", subpropertyOf("CLASSIFIED_AS")),
		property("GRID_REFERENCE", "grid reference", "koordinata mreže", "referência de grelha", notTextSearchable()),
		property("FOUNDED", "founded", "ustanovljeno", "fundado"),
		property("OCCUPATION_PERIOD", "period of occupation", "obdobje poselitve", "período de ocupação", subpropertyOf("PERIOD")),
		property("POPULATION_ESTIMATE", "estimated population", "ocena števila prebivalcev", "população estimada"),
	)

	// Beings: species, the individuals some of them have, and the collectives the rest have instead.

	properties = append(properties,
		property("HAS_HOMEWORLD", "homeworld", "matični svet", "mundo natal", subpropertyOf("RELATED_LOCATION")),
		property("HOME_TO_SPECIES", "home to species", "matični svet vrste", "espécie nativa", inverseOf("HAS_HOMEWORLD")),
		property("ALSO_FOUND_ON", "also found on", "najdena tudi na", "também encontrada em", subpropertyOf("RELATED_LOCATION")),
		property("TAXON_CODE", "taxon code", "taksonska oznaka", "código taxonómico", linkTemplate("https://registry.ccx.example/taxon/{value}"), notTextSearchable()),
		property("BODY_PLAN", "body plan", "telesni ustroj", "plano corporal"),
		property("LIFESPAN", "lifespan", "življenjska doba", "tempo de vida"),
		property("TYPICAL_MASS", "typical mass", "značilna masa", "massa típica"),
		property("TYPICAL_HEIGHT", "typical height", "značilna višina", "altura típica"),
		property("TYPICAL_SIZE", "typical size", "značilna velikost", "tamanho típico"),
		property("HAS_SENSORY_MODALITY", "sensory modality", "čutna modalnost", "modalidade sensorial", subpropertyOf("CLASSIFIED_AS")),
		property("HAS_SUBSISTENCE_MODE", "subsistence mode", "način preživljanja", "modo de subsistência", subpropertyOf("CLASSIFIED_AS")),
		property("HAS_SOCIAL_ORGANISATION", "social organisation", "družbena ureditev", "organização social", subpropertyOf("CLASSIFIED_AS")),
		property("HAS_KINSHIP_SYSTEM", "kinship system", "sorodstveni sistem", "sistema de parentesco", subpropertyOf("CLASSIFIED_AS")),
		property("HAS_INDIVIDUALITY_MODE", "mode of individuality", "način posameznosti", "modo de individualidade", subpropertyOf("CLASSIFIED_AS")),
		property("FIRST_CONTACT", "first contact", "prvi stik", "primeiro contacto"),
		property("USES_COMMUNICATION_SYSTEM", "communication system", "sporazumevalni sistem", "sistema de comunicação"),
		property("USED_BY_SPECIES", "used by species", "uporablja ga vrsta", "usado pela espécie", inverseOf("USES_COMMUNICATION_SYSTEM")),
		property("OF_SPECIES", "species", "vrsta", "espécie"),
		property("HAS_DOCUMENTED_MEMBER", "documented member", "dokumentirani pripadnik", "membro documentado", inverseOf("OF_SPECIES")),
		property("BELONGS_TO_CULTURE", "culture", "kultura", "cultura"),
		property("HAS_MEMBER", "member", "pripadnik", "membro", inverseOf("BELONGS_TO_CULTURE")),
		property("BORN", "born", "rojen", "nascido"),
		property("DIED", "died", "umrl", "falecido"),
		property("BIRTHPLACE", "place of birth", "kraj rojstva", "local de nascimento", subpropertyOf("RELATED_LOCATION")),
		property("LINEAGE_NAME", "lineage name", "rodovno ime", "nome de linhagem"),
		property("ROLE", "role", "vloga", "papel"),
		property("FORM_OF_ADDRESS", "form of address", "nagovor", "forma de tratamento",
			describes(
				"<p>How the person is properly addressed, which is not always by name and not always in speech.</p>",
				"<p>Kako osebo pravilno nagovorimo, kar ni vedno z imenom in ne vedno govorno.</p>", "<p>Como se trata corretamente a pessoa, o que nem sempre é pelo nome e nem sempre de viva voz.</p>",
			),
		),
		property("MEMBER_COUNT", "estimated membership", "ocena števila članov", "número estimado de membros"),
		property("ACTIVE_PERIOD", "period of activity", "obdobje delovanja", "período de atividade", subpropertyOf("PERIOD")),
	)

	// Culture: what the beings do, say, make and tell.

	properties = append(properties,
		property("PRACTISED_BY", "practised by", "nosilci", "praticado por"),
		property("PRESENT_AT", "present at", "prisotna na", "presente em", subpropertyOf("RELATED_LOCATION")),
		property("HOSTS_CULTURE", "hosts culture", "gosti kulturo", "acolhe cultura", inverseOf("PRESENT_AT")),
		property("OF_CULTURE", "culture", "kultura", "cultura"),
		property("HAS_CULTURAL_ELEMENT", "cultural element", "kulturni element", "elemento cultural", inverseOf("OF_CULTURE")),
		property("HAS_PRACTICE_CATEGORY", "kind of practice", "vrsta prakse", "tipo de prática", subpropertyOf("CLASSIFIED_AS")),
		property("PERIODICITY", "periodicity", "pogostost", "periodicidade"),
		property("PARTICIPANT_COUNT", "participants", "število udeležencev", "número de participants"),
		property("FIRST_DOCUMENTED", "first documented", "prvič dokumentirano", "primeira documentação"),
		property("RELATED_PRACTICE", "related practice", "sorodna praksa", "prática relacionada"),
		property("HAS_COMMUNICATION_MODALITY", "modality", "modalnost", "modalidade", subpropertyOf("CLASSIFIED_AS")),
		property("SPEAKER_ESTIMATE", "estimated users", "ocena števila uporabnikov", "número estimado de utilizadores"),
		property("HAS_NOTATION_SYSTEM", "has a notation system", "ima zapisovalni sistem", "tem sistema de notação"),
		property("SAMPLE_GLOSS", "sample with gloss", "vzorec s pomenom", "amostra com glosa"),
		property("HAS_ARTIFACT_CATEGORY", "kind of artifact", "vrsta predmeta", "tipo de artefacto", subpropertyOf("CLASSIFIED_AS")),
		property("FOUND_AT", "found at", "najden na", "encontrado em", subpropertyOf("RELATED_LOCATION")),
		property("MATERIAL", "material", "material", "material"),
		property("DIMENSION", "dimension", "mera", "dimensão"),
		property("AXIS", "axis", "os", "eixo"),
		property("DATE_MADE", "date made", "datum izdelave", "data de fabrico"),
		property("COLLECTED_BY", "collected by", "zbral", "recolhido por", subpropertyOf("RELATED_PERSON")),
		property("ACCESSION_CODE", "accession code", "pristopna oznaka", "código de inventário", linkTemplate("https://collections.ccx.example/object/{value}"), notTextSearchable()),
		property("HAS_NARRATIVE_GENRE", "genre", "zvrst", "género", subpropertyOf("CLASSIFIED_AS")),
		property("RECORDED_BY", "recorded by", "posnel", "registada por", subpropertyOf("RELATED_PERSON")),
		property("RECORDED_AT", "recorded at", "posneto na", "registada em", subpropertyOf("RELATED_LOCATION")),
		property("RECORDED_ON", "recorded on", "posneto dne", "registada a"),
		property("HAS_ORGANISM_CATEGORY", "kind of organism", "vrsta organizma", "tipo de organismo", subpropertyOf("CLASSIFIED_AS")),
		property("FOUND_ON", "found on", "najden na", "encontrado em", subpropertyOf("RELATED_LOCATION")),
	)

	// The research apparatus: who went where, what they wrote down, and under which clearance.

	properties = append(properties,
		property("FAMILY_NAME", "family name", "priimek", "apelido"),
		property("LOCATED_AT", "located at", "nahaja se na", "situado em", subpropertyOf("RELATED_LOCATION")),
		property("STAFF_COUNT", "staff", "število zaposlenih", "número de funcionários"),
		property("AFFILIATED_WITH", "affiliation", "pripadnost", "afiliação"),
		property("HAS_AFFILIATE", "affiliated researcher", "pridruženi raziskovalec", "investigador afiliado", inverseOf("AFFILIATED_WITH")),
		property("SPECIALISES_IN", "specialises in", "specializiran za", "especializa-se em", subpropertyOf("CLASSIFIED_AS")),
		property("RESEARCHER_CODE", "researcher code", "oznaka raziskovalca", "código de investigador", linkTemplate("https://registry.ccx.example/person/{value}"), notTextSearchable()),
		property("HAS_DESTINATION", "destination", "cilj", "destino", subpropertyOf("RELATED_LOCATION")),
		property("LED_BY", "led by", "vodil", "liderada por", subpropertyOf("RELATED_PERSON")),
		property("HAS_TEAM_MEMBER", "team member", "član ekipe", "membro da equipa", subpropertyOf("RELATED_PERSON")),
		property("ORGANISED_BY", "organised by", "organiziral", "organizada por"),
		property("RAN_EXPEDITION", "ran expedition", "izvedel odpravo", "realizou expedição", inverseOf("ORGANISED_BY")),
		property("BUDGET", "budget", "proračun", "orçamento"),
		property("USES_METHOD", "method", "metoda", "método", subpropertyOf("CLASSIFIED_AS")),
		property("UNDER_ETHICS_PROTOCOL", "ethics protocol", "etični protokol", "protocolo ético", subpropertyOf("CLASSIFIED_AS")),
		property("HAS_REPORT", "report", "poročilo", "relatório"),
		property("PART_OF_EXPEDITION", "expedition", "odprava", "expedição"),
		property("HAS_OBSERVATION", "observation", "opazovanje", "observação", inverseOf("PART_OF_EXPEDITION")),
		property("OBSERVED_BY", "observed by", "opazoval", "observada por", subpropertyOf("RELATED_PERSON")),
		property("OBSERVED_AT", "observed at", "opazovano na", "observada em", subpropertyOf("RELATED_LOCATION")),
		property("OBSERVED_ON", "observed on", "opazovano dne", "observada a"),
		property("ABOUT", "about", "o", "sobre"),
		property("FIELD_CONDITIONS", "field conditions", "razmere na terenu", "condições de campo"),
		property("HAS_INTERVIEWEE", "interviewee", "sogovornik", "entrevistado"),
		property("HAS_INTERVIEWER", "interviewer", "spraševalec", "entrevistador", subpropertyOf("RELATED_PERSON")),
		property("IN_COMMUNICATION_SYSTEM", "conducted in", "izvedeno v", "realizada em"),
		property("DURATION", "duration", "trajanje", "duração"),
		property("CONSENT_NOTE", "consent note", "opomba o privolitvi", "nota de consentimento",
			describes(
				"<p>What the interviewee agreed to, in their own framing, and anything they asked to be withheld.</p>",
				"<p>Kaj je sogovornik dovolil, v svojem lastnem izrazju, in vse, kar je prosil, da se ne objavi.</p>", "<p>Aquilo com que o entrevistado concordou, nos seus próprios termos, e tudo o que pediu que não fosse divulgado.</p>",
			),
		),
		property("HAS_CLEARED_READER", "cleared reader", "odobreni bralec", "leitor autorizado",
			describes(
				"<p>A researcher cleared to read the restricted record, chosen from the users the record grants access to.</p>",
				"<p>Raziskovalec, ki sme brati omejeni zapis, izbran med uporabniki, ki jim zapis dovoljuje dostop.</p>", "<p>Um investigador autorizado a ler o registo restrito, escolhido entre os utilizadores a quem o registo concede acesso.</p>",
			),
			notTextSearchable(),
		),
		property("HAS_AUTHOR", "author", "avtor", "author", subpropertyOf("RELATED_PERSON")),
		property("PUBLISHED_ON", "published on", "objavljeno dne", "publicada a"),
		property("DOI", "DOI", "DOI", "DOI", linkTemplate("https://doi.example/{value}"), notTextSearchable()),
		property("JOURNAL", "journal", "revija", "revista"),
		property("CITES", "cites", "navaja", "cita"),
		property("CITED_BY", "cited by", "naveden v", "citada por", inverseOf("CITES")),
		property("ABSTRACT", "abstract", "povzetek", "resumo"),
	)

	documents := make([]any, 0, len(properties))
	seen := make(map[string]bool, len(properties))
	for _, prop := range properties {
		if seen[prop.Mnemonic] {
			errE := errors.New("duplicate test data property")
			errors.Details(errE)["mnemonic"] = prop.Mnemonic
			return nil, errE
		}
		seen[prop.Mnemonic] = true
		documents = append(documents, prop)
	}

	return documents, nil
}
