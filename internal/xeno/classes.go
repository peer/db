package xeno

import (
	"gitlab.com/tozd/go/errors"

	"gitlab.com/peerdb/peerdb/transform"
)

// placeLabelTemplate defines a template rendering a place recursively as "Name, Parent, Grandparent"
// up the containment chain. The recursion is capped by the "remaining" count the caller passes in,
// because the chain is data and nothing stops data from pointing at itself.
var placeLabelTemplate = `{{define "placeLabel"}}` + //nolint:gochecknoglobals
	`{{$doc := .doc}}{{$remaining := .remaining}}` +
	`{{$placeName := bestString ` + namePropID + ` $doc}}` +
	`{{$parent := bestReferenceDoc ` + propID("CONTAINED_IN") + ` $doc}}` +
	`{{if and $placeName $parent (gt $remaining 1)}}{{$placeName}}, {{template "placeLabel" (dict "doc" $parent "remaining" (sub $remaining 1))}}` +
	`{{else if $placeName}}{{$placeName}}` +
	`{{end}}` +
	`{{end}}`

// placeLabel is the display label of the places whose name only makes sense together with what
// contains them: two hundred worlds have a "Northern Shelf" and the catalogue has to tell them
// apart.
func placeLabel(depth string) string {
	return placeLabelTemplate + `{{template "placeLabel" (dict "doc" . "remaining" ` + depth + `)}}`
}

// namedInParent renders "Name (Parent)", used where the parent is worth showing but the whole chain
// is not.
func namedInParent(parentProp string) string {
	return `{{$name := bestString ` + namePropID + ` .}}` +
		`{{$parentDoc := bestReferenceDoc ` + parentProp + ` .}}` +
		`{{$parentName := bestString ` + namePropID + ` $parentDoc}}` +
		`{{if and $name $parentName}}{{$name}} ({{$parentName}}){{else}}{{$name}}{{end}}`
}

// datedRecord renders "Title (date)" for the records whose date is how they are told apart.
func datedRecord(dateProp string) string {
	return `{{$title := bestString ` + namePropID + ` .}}` +
		`{{$when := index (splitList "-" (bestTimeString ` + dateProp + ` .)) 0}}` +
		`{{if and $title $when}}{{$title}} ({{$when}}){{else}}{{$title}}{{end}}`
}

// personLabel renders a researcher as the given name followed by the family name. The order is the
// same in both languages: which way round a person's name goes is a convention of the catalogue
// rather than of the language, and both languages this site speaks write a person given name first.
func personLabel() string {
	return `{{$given := bestString ` + namePropID + ` .}}` +
		`{{$family := bestString ` + propID("FAMILY_NAME") + ` .}}` +
		`{{if and $given $family}}{{$given}} {{$family}}{{else if $family}}{{$family}}{{else}}{{$given}}{{end}}`
}

// vocabularyClasses are the controlled vocabularies of the test data: the sets of terms the
// catalogue classifies entries with. They are all subclasses of the core vocabulary class and all
// hold the same fields, so they are declared as a table rather than one by one.
//
//nolint:gochecknoglobals,lll
var vocabularyClasses = []struct {
	Mnemonic string
	EN       string
	SL       string
	PT       string
	Describe [3]string
}{
	{"PLANET_TYPE", "world type", "vrsta sveta", "tipo de mundo", [3]string{
		"<p>Classes of world by bulk composition and surface state.</p>",
		"<p>Razredi svetov glede na sestavo in stanje površja.</p>", "<p>Classes de mundos por composição global e estado da superfície.</p>",
	}},
	{"BIOME", "biome", "biom", "bioma", [3]string{
		"<p>Kinds of living environment. Most of them have no Earth equivalent, which is the point.</p>",
		"<p>Vrste življenjskih okolij. Večina jih na Zemlji nima ustreznice, kar je tudi bistvo.</p>", "<p>Tipos de ambiente natural. A maioria não tem equivalente terrestre, e é esse o ponto.</p>",
	}},
	{"SITE_TYPE", "site type", "vrsta najdišča", "tipo de sítio", [3]string{
		"<p>What kind of place a field site is, from a seasonal aggregation ground to a decommissioned relay station.</p>",
		"<p>Kakšna vrsta kraja je terensko najdišče, od sezonskega zbirališča do opuščene relejne postaje.</p>", "<p>Que tipo de lugar é um sítio de campo, desde um terreno de agregação sazonal até uma estação de retransmissão desativada.</p>",
	}},
	{"CONTACT_STATUS", "contact status", "stanje stikov", "estado de contacto", [3]string{
		"<p>The Consortium's contact ladder, from no contact at all to reciprocal exchange, and back down again when contact is suspended.</p>",
		"<p>Lestvica stikov konzorcija, od popolne odsotnosti stikov do vzajemne izmenjave in nazaj, ko so stiki prekinjeni.</p>", "<p>A escala de contacto do Consórcio, desde nenhum contacto até à troca recíproca, e outra vez para baixo quando o contacto é suspension.</p>",
	}},
	{"SENSORY_MODALITY", "sensory modality", "čutna modalnost", "modalidade sensorial", [3]string{
		"<p>Ways of perceiving. A species usually has several and rarely the ones a visitor expects.</p>",
		"<p>Načini zaznavanja. Vrsta jih ima navadno več in redko tiste, ki jih obiskovalec pričakuje.</p>", "<p>Formas de perceber. Uma espécie tem normalmente várias e raramente as que um visitante espera.</p>",
	}},
	{"SUBSISTENCE_MODE", "subsistence mode", "način preživljanja", "modo de subsistência", [3]string{
		"<p>How a culture feeds itself.</p>",
		"<p>Kako se kultura prehranjuje.</p>", "<p>Como uma cultura se alimenta.</p>",
	}},
	{"SOCIAL_ORGANISATION", "social organisation", "družbena ureditev", "organização social", [3]string{
		"<p>The shape a polity takes, where there is one.</p>",
		"<p>Oblika, ki jo prevzame skupnost, če jo sploh ima.</p>", "<p>A forma que assume uma comunidade política, quando existe.</p>",
	}},
	{"KINSHIP_SYSTEM", "kinship system", "sorodstveni sistem", "sistema de parentesco", [3]string{
		"<p>How relatedness is reckoned, including the systems which do not reckon by descent at all.</p>",
		"<p>Kako se računa sorodstvo, vključno s sistemi, ki sploh ne računajo po rodu.</p>", "<p>Como se conta o parentesco, incluindo os sistemas que não contam de todo por descendência.</p>",
	}},
	{"INDIVIDUALITY_MODE", "mode of individuality", "način posameznosti", "modo de individualidade", [3]string{
		"<p>Whether a species has individuals, and in what sense. The contested term is used more often than anybody is comfortable with.</p>",
		"<p>Ali ima vrsta posameznike in v kakšnem smislu. Sporni izraz se uporablja pogosteje, kot je komu ljubo.</p>", "<p>Se uma espécie tem indivíduos, e em que sentido. O termo contestado é usado com mais frequência do que qualquer um gostaria.</p>",
	}},
	{"ORGANISM_CATEGORY", "kind of organism", "vrsta organizma", "tipo de organismo", [3]string{
		"<p>Broad categories for non-sapient life.</p>",
		"<p>Široke kategorije za nerazumno življenje.</p>", "<p>Categorias amplas para a vida não sapiente.</p>",
	}},
	{"ARTIFACT_CATEGORY", "kind of artifact", "vrsta predmeta", "tipo de artefacto", [3]string{
		"<p>Kinds of made thing.</p>",
		"<p>Vrste izdelanih predmetov.</p>", "<p>Tipos de coisa fabricada.</p>",
	}},
	{"PRACTICE_CATEGORY", "kind of practice", "vrsta prakse", "tipo de prática", [3]string{
		"<p>Kinds of cultural practice.</p>",
		"<p>Vrste kulturnih praks.</p>", "<p>Tipos de prática cultural.</p>",
	}},
	{"NARRATIVE_GENRE", "genre", "zvrst", "género", [3]string{
		"<p>Forms a told or inscribed thing takes.</p>",
		"<p>Oblike, ki jih prevzame pripovedovano ali zapisano.</p>", "<p>Formas que assume aquilo que é contado ou inscrito.</p>",
	}},
	{"RESEARCH_METHOD", "research method", "raziskovalna metoda", "método de investigação", [3]string{
		"<p>How the Consortium works, on paper.</p>",
		"<p>Kako konzorcij dela, vsaj na papirju.</p>", "<p>Como o Consórcio trabalha, no papel.</p>",
	}},
	{"ETHICS_PROTOCOL", "ethics protocol", "etični protokol", "protocolo ético", [3]string{
		"<p>The clearances under which field material may be collected, held and read.</p>",
		"<p>Dovoljenja, pod katerimi se sme terensko gradivo zbirati, hraniti in brati.</p>", "<p>As autorizações ao abrigo das quais o material de campo pode ser recolhido, conservado e lido.</p>",
	}},
	{"COMMUNICATION_MODALITY", "modality", "modalnost", "modalidade", [3]string{
		"<p>Channels a communication system runs on.</p>",
		"<p>Kanali, po katerih teče sporazumevalni sistem.</p>", "<p>Canais em que funciona um sistema de comunicação.</p>",
	}},
}

// worldSections and the section maps below name the sections of the classes whose fields are grouped,
// once per language, the way transform.Fields wants them.
//
//nolint:gochecknoglobals
var (
	worldSections = map[string]map[string]string{
		"identification": {languageEN: "Identification", languageSL: "Določitev", languagePT: "Identificação"},
		"physical":       {languageEN: "Physical properties", languageSL: "Fizikalne lastnosti", languagePT: "Propriedades físicas"},
		"environment":    {languageEN: "Environment", languageSL: "Okolje", languagePT: "Ambiente"},
		"survey":         {languageEN: "Survey", languageSL: "Pregled", languagePT: "Levantamento"},
	}

	speciesSections = map[string]map[string]string{
		"identification": {languageEN: "Identification", languageSL: "Določitev", languagePT: "Identificação"},
		"biology":        {languageEN: "Biology", languageSL: "Biologija", languagePT: "Biological"},
		"society":        {languageEN: "Society", languageSL: "Družba", languagePT: "Sociedade"},
		"contact":        {languageEN: "Contact", languageSL: "Stiki", languagePT: "Contacto"},
	}

	interviewSections = map[string]map[string]string{
		"subject":   {languageEN: "Who and where", languageSL: "Kdo in kje", languagePT: "Quem e onde"},
		"record":    {languageEN: "The record", languageSL: "Zapis", languagePT: "O registo"},
		"clearance": {languageEN: "Clearance", languageSL: "Dovoljenja", languagePT: "Autorizações"},
	}
)

// Classes returns the classes of the test data schema.
//
// The mnemonics parameter maps property mnemonic names to property document base IDs; when it is nil
// the classes are returned complete in every other respect but without their field schema.
//
//nolint:funlen,maintidx,lll
func Classes(mnemonics map[string][]string) ([]any, errors.E) {
	documents := []any{}

	// The controlled vocabularies. Their entries are what everything else is classified by, so they
	// come first.

	vocabularyFields, errE := transform.Fields[Vocabulary](mnemonics, nil, nil)
	if errE != nil {
		return nil, errE
	}
	for _, v := range vocabularyClasses {
		documents = append(documents, class(v.Mnemonic, v.EN, v.SL, v.PT, vocabularyFields,
			classDescribes(v.Describe[0], v.Describe[1], v.Describe[2]),
			subclassOfCore("VOCABULARY"),
		))
	}

	// Places, from the galaxy down to the hearth cluster.

	documents = append(documents, class("PLACE", "place", "kraj", "lugar", nil,
		classDescribes(
			"<p>Anything the catalogue can point at as somewhere: a galaxy, a sector, a system, a world, a stretch of ground, a camp.</p>",
			"<p>Vse, na kar katalog lahko pokaže kot na kraj: galaksija, sektor, sistem, svet, kos tal, taborišče.</p>", "<p>Tudo o que o catálogo pode apontar como um lugar: uma galáxia, um setor, um sistema, um mundo, um pedaço de terreno, um acampamento.</p>",
		),
		abstract(),
	))

	galaxyFields, errE := transform.Fields[Galaxy](mnemonics, nil, nil)
	if errE != nil {
		return nil, errE
	}
	documents = append(documents, class("GALAXY", "galaxy", "galaksija", "galáxia", galaxyFields,
		subclassOf("PLACE"),
		shortcuts(
			backlink("CONTAINED_IN", "SECTOR", "Sectors in this galaxy", "Sektorji v tej galaksiji", "Setores nesta galáxia"),
		),
	))

	sectorFields, errE := transform.Fields[Sector](mnemonics, nil, nil)
	if errE != nil {
		return nil, errE
	}
	documents = append(documents, class("SECTOR", "survey sector", "pregledni sektor", "setor de levantamento", sectorFields,
		classDescribes(
			"<p>An administrative slice of a galaxy. Sector boundaries are drawn by committee and are renamed more often than they are resurveyed.</p>",
			"<p>Upravni izsek galaksije. Meje sektorjev določa odbor in jih preimenuje pogosteje, kot jih ponovno pregleda.</p>", "<p>Uma fatia administrativa de uma galáxia. As fronteiras dos setores são traçadas por comité e mudam de nome mais vezes do que voltam a ser levantadas.</p>",
		),
		subclassOf("PLACE"),
		displayLabel(placeLabel("2"), placeLabel("2"), placeLabel("2")),
		shortcuts(
			backlink("CONTAINED_IN", "STAR_SYSTEM", "Star systems in this sector", "Zvezdni sistemi v tem sektorju", "Sistemas estelares neste setor"),
		),
	))

	starSystemFields, errE := transform.Fields[StarSystem](mnemonics, nil, nil)
	if errE != nil {
		return nil, errE
	}
	documents = append(documents, class("STAR_SYSTEM", "star system", "zvezdni sistem", "sistema estelar", starSystemFields,
		subclassOf("PLACE"),
		displayLabel(placeLabel("2"), placeLabel("2"), placeLabel("2")),
		shortcuts(
			backlinkCreate("CONTAINED_IN", "PLANET", "Planets in this system", "Planeti v tem sistemu", "Planets neste sistema"),
		),
	))

	documents = append(documents, class("WORLD", "world", "svet", "mundo", nil,
		classDescribes(
			"<p>A body with a surface somebody could in principle stand on. Planets and moons differ only in what they orbit.</p>",
			"<p>Telo s površjem, na katerem bi kdo načeloma lahko stal. Planeti in lune se razlikujejo le po tem, kaj obkrožajo.</p>", "<p>Um corpo com uma superfície onde alguém poderia, em princípio, pôr-se de pé. Planets e luas diferem apenas naquilo que orbitam.</p>",
		),
		subclassOf("PLACE"),
		abstract(),
	))

	worldInstructions := map[string]map[string]string{
		"WorldFields.WorldEnvironment.Biosphere": {
			languageEN: "<p>Summarise what lives here. Leave this empty and mark it as having no value when the world was surveyed and found sterile, " +
				"and as unknown when nobody has looked closely yet. An empty field says neither.</p>",
			languageSL: "<p>Povzemite, kaj tu živi. Pustite prazno in označite kot brez vrednosti, če je bil svet pregledan in je brez življenja, " +
				"in kot neznano, če ga še ni nihče natančno pogledal. Prazno polje ne pove ne enega ne drugega.</p>", languagePT: "<p>Resuma o que aqui vive. Deixe este campo vazio e marque-o como sem valor quando o levantamento do mundo o encontrou estéril, e como desconhecido quando ainda ninguém olhou de perto. Um campo vazio não diz nem uma coisa nem outra.</p>",
		},
		"WorldFields.WorldSurvey.PopulationEstimate": {
			languageEN: "<p>A range, not a number. Orbital counts and ground counts disagree by an order of magnitude often enough that a single figure is a lie.</p>",
			languageSL: "<p>Razpon, ne število. Orbitalna in prizemna štetja se dovolj pogosto razlikujejo za velikostni red, da je eno samo število laž.</p>", languagePT: "<p>Um intervalo, não um número. As contagens em órbita e as contagens no solo divergem numa ordem de grandeza com frequência suficiente para que um único valor seja uma mentira.</p>",
		},
	}

	planetFields, errE := transform.Fields[Planet](mnemonics, worldSections, worldInstructions)
	if errE != nil {
		return nil, errE
	}
	documents = append(documents, class("PLANET", "planet", "planet", "planeta", planetFields,
		subclassOf("WORLD"),
		displayLabel(namedInParent(propID("CONTAINED_IN")), namedInParent(propID("CONTAINED_IN")), namedInParent(propID("CONTAINED_IN"))),
		shortcuts(
			backlinkCreate("CONTAINED_IN", "REGION", "Regions of this world", "Pokrajine tega sveta", "Regiões deste mundo"),
			backlink("CONTAINED_IN", "MOON", "Moons of this planet", "Lune tega planeta", "Luas deste planeta"),
			backlink("HAS_HOMEWORLD", "SPECIES", "Species native to this world", "Vrste, doma na tem svetu", "Espécies nativas deste mundo"),
			backlink("FOUND_ON", "ORGANISM", "Organisms recorded here", "Tu zabeleženi organizmi", "Organismos registados aqui"),
			backlink("HAS_DESTINATION", "EXPEDITION", "Expeditions to this world", "Odprave na ta svet", "Expedições a este mundo"),
		),
	))

	moonFields, errE := transform.Fields[Moon](mnemonics, worldSections, worldInstructions)
	if errE != nil {
		return nil, errE
	}
	documents = append(documents, class("MOON", "moon", "luna", "lua", moonFields,
		subclassOf("WORLD"),
		displayLabel(namedInParent(propID("CONTAINED_IN")), namedInParent(propID("CONTAINED_IN")), namedInParent(propID("CONTAINED_IN"))),
		shortcuts(
			backlinkCreate("CONTAINED_IN", "REGION", "Regions of this moon", "Pokrajine te lune", "Regiões desta lua"),
			backlink("HAS_HOMEWORLD", "SPECIES", "Species native to this moon", "Vrste, doma na tej luni", "Espécies nativas desta lua"),
		),
	))

	regionFields, errE := transform.Fields[Region](mnemonics, nil, nil)
	if errE != nil {
		return nil, errE
	}
	documents = append(documents, class("REGION", "region", "pokrajina", "região", regionFields,
		classDescribes(
			"<p>A stretch of a world's surface treated as one place. The biomes of the world it is on are pulled in alongside its own.</p>",
			"<p>Del površja sveta, obravnavan kot en kraj. Biomi sveta, na katerem leži, se privzamejo poleg njegovih lastnih.</p>", "<p>Uma extensão da superfície de um mundo tratada como um só lugar. Os biomas do mundo onde se encontra são trazidos juntamente com os seus.</p>",
		),
		subclassOf("PLACE"),
		displayLabel(placeLabel("3"), placeLabel("3"), placeLabel("3")),
		shortcuts(
			backlinkCreate("CONTAINED_IN", "SITE", "Sites in this region", "Najdišča v tej pokrajini", "Sítios nesta região"),
		),
	))

	siteInstructions := map[string]map[string]string{
		"SiteFields.LocalName": {
			languageEN: "<p>What the inhabitants call the place. A site named from orbit has no local name on record, and saying so is worth more than leaving this empty.</p>",
			languageSL: "<p>Kako kraju pravijo prebivalci. Kraj, poimenovan iz orbite, nima zabeleženega krajevnega imena, in to povedati je vredno več kot pustiti prazno.</p>", languagePT: "<p>O nome que os habitantes dão ao lugar. Um sítio nomeado a partir da órbita não tem nome local registado, e dizê-lo vale mais do que deixar isto vazio.</p>",
		},
	}

	siteFields, errE := transform.Fields[Site](mnemonics, nil, siteInstructions)
	if errE != nil {
		return nil, errE
	}
	documents = append(documents, class("SITE", "field site", "terensko najdišče", "sítio de campo", siteFields,
		classAlternativeNames("settlement", "naselje", "povoação"),
		subclassOf("PLACE"),
		displayLabel(placeLabel("3"), placeLabel("3"), placeLabel("3")),
		shortcuts(
			backlink("PRESENT_AT", "CULTURE", "Cultures present here", "Kulture, prisotne tukaj", "Culturas presents aqui"),
			backlink("FOUND_AT", "ARTIFACT", "Artifacts found here", "Predmeti, najdeni tukaj", "Artefactos encontrados aqui"),
			backlinkCreate("OBSERVED_AT", "OBSERVATION", "Observations made here", "Opazovanja, opravljena tukaj", "Observações feitas aqui"),
		),
	))

	// Beings.

	speciesInstructions := map[string]map[string]string{
		"SpeciesFields.SpeciesBiology.BodyPlan": {
			languageEN: "<p>Describe the body as a body, not as a comparison. \"Radially symmetric, six limbs\" is useful; \"a bit like a crab\" is not.</p>",
			languageSL: "<p>Telo opišite kot telo, ne kot primerjavo. \"Radialno simetrično, šest okončin\" je uporabno, \"nekako kot rak\" ni.</p>", languagePT: "<p>Descreva o corpo como corpo, não como comparação. \"Simetria radial, seis membros\" é útil; \"parecido com um caranguejo\" não é.</p>",
		},
		"SpeciesFields.SpeciesSociety.Individuality": {
			languageEN: "<p>Where the discipline cannot agree whether the species has individuals at all, say so with the contested term rather than picking a side.</p>",
			languageSL: "<p>Kadar se stroka ne more zediniti, ali ima vrsta sploh posameznike, to povejte s spornim izrazom, namesto da izberete stran.</p>", languagePT: "<p>Quando a disciplina não consegue chegar a acordo sobre se a espécie tem sequer indivíduos, diga-o com o termo contestado em vez de tomar partido.</p>",
		},
	}

	speciesFields, errE := transform.Fields[Species](mnemonics, speciesSections, speciesInstructions)
	if errE != nil {
		return nil, errE
	}
	documents = append(documents, class("SPECIES", "species", "vrsta", "espécie", speciesFields,
		shortcuts(
			backlinkCreate("OF_SPECIES", "INDIVIDUAL", "Documented members", "Dokumentirani pripadniki", "Membros documentados"),
			backlink("PRACTISED_BY", "CULTURE", "Cultures of this species", "Kulture te vrste", "Culturas desta espécie"),
			backlink("USED_BY_SPECIES", "COMMUNICATION_SYSTEM", "Communication systems", "Sporazumevalni sistemi", "Sistemas de comunicação"),
			backlink("ABOUT", "PUBLICATION", "Publications about this species", "Objave o tej vrsti", "Publicações sobre esta espécie"),
		),
	))

	documents = append(documents, class("BEING", "being", "bitje", "ser", nil,
		classDescribes(
			"<p>Somebody the catalogue can record having spoken to, whether or not the discipline agrees they are one somebody.</p>",
			"<p>Nekdo, za katerega katalog lahko zabeleži pogovor z njim, ne glede na to, ali se stroka strinja, da je en sam nekdo.</p>", "<p>Alguém com quem o catálogo pode registar que se falou, concorde ou não a disciplina que se trate de um só alguém.</p>",
		),
		abstract(),
	))

	individualFields, errE := transform.Fields[Individual](mnemonics, nil, nil)
	if errE != nil {
		return nil, errE
	}
	documents = append(documents, class("INDIVIDUAL", "individual", "posameznik", "indivíduo", individualFields,
		subclassOf("BEING"),
		displayLabel(namedInParent(propID("OF_SPECIES")), namedInParent(propID("OF_SPECIES")), namedInParent(propID("OF_SPECIES"))),
		shortcuts(
			backlink("HAS_INTERVIEWEE", "INTERVIEW", "Interviews with this person", "Pogovori s to osebo", "Entrevistas com esta pessoa"),
		),
	))

	collectiveFields, errE := transform.Fields[Collective](mnemonics, nil, nil)
	if errE != nil {
		return nil, errE
	}
	documents = append(documents, class("COLLECTIVE", "collective", "kolektiv", "coletivo", collectiveFields,
		classDescribes(
			"<p>A body which acts as one and which the discipline does not split into individuals, either because it cannot or because the collective asked it not to.</p>",
			"<p>Telo, ki deluje kot eno in ki ga stroka ne deli na posameznike, bodisi ker ne more bodisi ker je kolektiv prosil, naj tega ne počne.</p>", "<p>Um corpo que age como um só e que a disciplina não divide em indivíduos, seja porque não consegue, seja porque o coletivo lhe pediu que não o fizesse.</p>",
		),
		subclassOf("BEING"),
		displayLabel(namedInParent(propID("OF_SPECIES")), namedInParent(propID("OF_SPECIES")), namedInParent(propID("OF_SPECIES"))),
		shortcuts(
			backlink("HAS_INTERVIEWEE", "INTERVIEW", "Interviews with this collective", "Pogovori s tem kolektivom", "Entrevistas com este coletivo"),
		),
	))

	// Culture and what it leaves behind.

	cultureFields, errE := transform.Fields[Culture](mnemonics, nil, nil)
	if errE != nil {
		return nil, errE
	}
	documents = append(documents, class("CULTURE", "culture", "kultura", "cultura", cultureFields,
		shortcuts(
			backlinkCreate("OF_CULTURE", "PRACTICE", "Practices of this culture", "Prakse te kulture", "Práticas desta cultura"),
			backlinkCreate("OF_CULTURE", "ARTIFACT", "Artifacts of this culture", "Predmeti te kulture", "Artefactos desta cultura"),
			backlinkCreate("OF_CULTURE", "NARRATIVE", "Narratives of this culture", "Pripovedi te kulture", "Narratives desta cultura"),
			backlink("BELONGS_TO_CULTURE", "INDIVIDUAL", "Members", "Pripadniki", "Membros"),
		),
	))

	documents = append(documents, class("CULTURAL_ELEMENT", "cultural element", "kulturni element", "elemento cultural", nil,
		classDescribes(
			"<p>Something a culture does, makes or tells, recorded as its own entry so it can be compared across cultures.</p>",
			"<p>Nekaj, kar kultura počne, izdeluje ali pripoveduje, zabeleženo kot samostojen vnos, da se lahko primerja med kulturami.</p>", "<p>Algo que uma cultura faz, fabrica ou conta, registado como entrada própria para poder ser comparado entre culturas.</p>",
		),
		abstract(),
	))

	practiceFields, errE := transform.Fields[Practice](mnemonics, nil, nil)
	if errE != nil {
		return nil, errE
	}
	documents = append(documents, class("PRACTICE", "practice", "praksa", "prática", practiceFields,
		subclassOf("CULTURAL_ELEMENT"),
	))

	communicationFields, errE := transform.Fields[CommunicationSystem](mnemonics, nil, nil)
	if errE != nil {
		return nil, errE
	}
	documents = append(documents, class("COMMUNICATION_SYSTEM", "communication system", "sporazumevalni sistem", "sistema de comunicação", communicationFields,
		classAlternativeNames("language", "jezik", "língua"),
		classDescribes(
			"<p>A way a species communicates. Calling all of them languages was settled against in 2244 and is still done in conversation.</p>",
			"<p>Način, kako se vrsta sporazumeva. Da bi vsem rekli jeziki, je bilo zavrnjeno leta 2244 in se v pogovoru še vedno počne.</p>", "<p>Uma forma de uma espécie comunicar. Chamar-lhes a todos línguas foi rejeitado em 2244 e continua a fazer-se na conversa.</p>",
		),
	))

	artifactFields, errE := transform.Fields[Artifact](mnemonics, nil, nil)
	if errE != nil {
		return nil, errE
	}
	documents = append(documents, class("ARTIFACT", "artifact", "predmet", "artefacto", artifactFields,
		subclassOf("CULTURAL_ELEMENT"),
	))

	narrativeFields, errE := transform.Fields[Narrative](mnemonics, nil, nil)
	if errE != nil {
		return nil, errE
	}
	documents = append(documents, class("NARRATIVE", "narrative", "pripoved", "narrativa", narrativeFields,
		subclassOf("CULTURAL_ELEMENT"),
		displayLabel(datedRecord(propID("RECORDED_ON")), datedRecord(propID("RECORDED_ON")), datedRecord(propID("RECORDED_ON"))),
	))

	organismFields, errE := transform.Fields[Organism](mnemonics, nil, nil)
	if errE != nil {
		return nil, errE
	}
	documents = append(documents, class("ORGANISM", "organism", "organizem", "organismo", organismFields,
		classDescribes(
			"<p>Non-sapient life, kept because a culture cannot be read without the things it eats, avoids and weaves with.</p>",
			"<p>Nerazumno življenje, ki ga hranimo, ker kulture ni mogoče brati brez tega, kar je, čemur se izogiba in iz česar tke.</p>", "<p>Vida não sapiente, guardada porque não se pode ler uma cultura sem as coisas que come, evita e com que tece.</p>",
		),
	))

	// The research apparatus.

	instituteFields, errE := transform.Fields[Institute](mnemonics, nil, nil)
	if errE != nil {
		return nil, errE
	}
	documents = append(documents, class("INSTITUTE", "institute", "inštitut", "institution", instituteFields,
		shortcuts(
			backlink("AFFILIATED_WITH", "RESEARCHER", "Researchers here", "Raziskovalci tukaj", "Investigadores aqui"),
			backlinkCreate("ORGANISED_BY", "EXPEDITION", "Expeditions run from here", "Odprave, izvedene od tod", "Expedições realizadas daqui"),
		),
	))

	researcherFields, errE := transform.Fields[Researcher](mnemonics, nil, nil)
	if errE != nil {
		return nil, errE
	}
	documents = append(documents, class("RESEARCHER", "researcher", "raziskovalec", "investigador", researcherFields,
		displayLabel(personLabel(), personLabel(), personLabel()),
		shortcuts(
			backlink("HAS_AUTHOR", "PUBLICATION", "Publications", "Objave", "Publicações"),
			backlink("OBSERVED_BY", "OBSERVATION", "Observations", "Opazovanja", "Observações"),
			backlink("LED_BY", "EXPEDITION", "Expeditions led", "Vodene odprave", "Expedições lideradas"),
			backlink("COLLECTED_BY", "ARTIFACT", "Artifacts collected", "Zbrani predmeti", "Artefactos recolhidos"),
		),
	))

	expeditionFields, errE := transform.Fields[Expedition](mnemonics, nil, nil)
	if errE != nil {
		return nil, errE
	}
	documents = append(documents, class("EXPEDITION", "expedition", "odprava", "expedição", expeditionFields,
		shortcuts(
			backlinkCreate("PART_OF_EXPEDITION", "OBSERVATION", "Observations from this expedition", "Opazovanja s te odprave", "Observações desta expedição"),
		),
	))

	documents = append(documents, class("RESEARCH_RECORD", "research record", "raziskovalni zapis", "registo de investigação", nil,
		classDescribes(
			"<p>Anything the Consortium produced rather than found: a field note, an interview, a paper.</p>",
			"<p>Vse, kar je konzorcij ustvaril in ne našel: terenska beležka, pogovor, članek.</p>", "<p>Tudo o que o Consórcio produziu em vez de encontrar: uma nota de campo, uma entrevista, um artigo.</p>",
		),
		abstract(),
	))

	observationInstructions := map[string]map[string]string{
		"ObservationFields.FieldConditions": {
			languageEN: "<p>Weather, light, who else was present, and anything else which would let a reader judge how much to trust the note.</p>",
			languageSL: "<p>Vreme, svetloba, kdo je bil še prisoten in vse drugo, kar bralcu omogoča presoditi, koliko naj beležki zaupa.</p>", languagePT: "<p>O tempo que fazia, a luz, quem mais estava presente e tudo o resto que permita a um leitor julgar quanto deve confiar na nota.</p>",
		},
	}

	observationFields, errE := transform.Fields[Observation](mnemonics, nil, observationInstructions)
	if errE != nil {
		return nil, errE
	}
	documents = append(documents, class("OBSERVATION", "observation", "opazovanje", "observação", observationFields,
		classAlternativeNames("field note", "terenska beležka", "nota de campo"),
		subclassOf("RESEARCH_RECORD"),
		displayLabel(datedRecord(propID("OBSERVED_ON")), datedRecord(propID("OBSERVED_ON")), datedRecord(propID("OBSERVED_ON"))),
	))

	interviewInstructions := map[string]map[string]string{
		"InterviewFields.InterviewClearance.ClearedReader": {
			languageEN: "<p>Name only researchers this record already grants access to on its permissions tab. Adding a name here does not grant anything by itself.</p>",
			languageSL: "<p>Navedite le raziskovalce, ki jim ta zapis dostop že dovoljuje na zavihku dovoljenj. Vpis imena sam po sebi ne podeli ničesar.</p>", languagePT: "<p>Indique apenas investigadores a quem este registo já concede acesso no separador de permissões. Acrescentar um nome aqui não concede nada por si só.</p>",
		},
		"InterviewFields.InterviewClearance.ConsentNote": {
			languageEN: "<p>What the interviewee agreed to, in their own framing, and anything they asked to be withheld. Write it before you write the transcript.</p>",
			languageSL: "<p>Kaj je sogovornik dovolil, v svojem lastnem izrazju, in vse, kar je prosil, naj se ne objavi. Zapišite to, preden zapišete prepis.</p>", languagePT: "<p>Aquilo com que o entrevistado concordou, nos seus próprios termos, e tudo o que pediu que não fosse divulgado. Escreva isto antes de escrever a transcrição.</p>",
		},
	}

	interviewFields, errE := transform.Fields[Interview](mnemonics, interviewSections, interviewInstructions)
	if errE != nil {
		return nil, errE
	}
	documents = append(documents, class("INTERVIEW", "interview", "pogovor", "entrevista", interviewFields,
		classDescribes(
			"<p>A recorded conversation. Interviews are restricted by default: the site grants nobody reading this class, so an interview is reachable only through the "+
				"permissions it carries.</p>",
			"<p>Posnet pogovor. Pogovori so privzeto omejeni: spletišče nikomur ne dovoli branja tega razreda, zato je pogovor dosegljiv le prek dovoljenj, ki jih nosi.</p>", "<p>Uma conversa gravada. As entrevistas são restritas por predefinição: o sítio não concede a ninguém a leitura desta classe, pelo que uma entrevista só é alcançável através das permissões que transporta.</p>",
		),
		subclassOf("RESEARCH_RECORD"),
		displayLabel(datedRecord(propID("RECORDED_ON")), datedRecord(propID("RECORDED_ON")), datedRecord(propID("RECORDED_ON"))),
	))

	publicationFields, errE := transform.Fields[Publication](mnemonics, nil, nil)
	if errE != nil {
		return nil, errE
	}
	documents = append(documents, class("PUBLICATION", "publication", "objava", "publicação", publicationFields,
		subclassOf("RESEARCH_RECORD"),
		displayLabel(datedRecord(propID("PUBLISHED_ON")), datedRecord(propID("PUBLISHED_ON")), datedRecord(propID("PUBLISHED_ON"))),
		shortcuts(
			backlink("CITES", "PUBLICATION", "Cited by", "Navedeno v", "Citada por"),
		),
	))

	return documents, nil
}
