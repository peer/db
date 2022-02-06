package wikipedia

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	//nolint:lll
	xmlJSON = `{
		"xml": "<?xml version=\"1.0\" ?>\n<!DOCTYPE DjVuXML PUBLIC \"-//W3C//DTD DjVuXML 1.1//EN\" \"pubtext/DjVuXML-s.dtd\">\n<mw-djvu><DjVuXML>\n<HEAD></HEAD>\n<BODY><OBJECT height=\"4850\" width=\"3079\">\n<PARAM name=\"DPI\" value=\"550\" />\n<PARAM name=\"GAMMA\" value=\"2.2\" />\n</OBJECT>\n<OBJECT height=\"4850\" width=\"3079\">\n<PARAM name=\"DPI\" value=\"550\" />\n<PARAM name=\"GAMMA\" value=\"2.2\" />\n</OBJECT>\n<OBJECT height=\"4850\" width=\"3079\">\n<PARAM name=\"DPI\" value=\"550\" />\n<PARAM name=\"GAMMA\" value=\"2.2\" />\n</OBJECT>\n<OBJECT height=\"4850\" width=\"3079\">\n<PARAM name=\"DPI\" value=\"550\" />\n<PARAM name=\"GAMMA\" value=\"2.2\" />\n</OBJECT>\n</BODY>\n</DjVuXML>\n<DjVuTxt>\n<HEAD></HEAD>\n<BODY>\n<PAGE value=\"17de Mai 1872 &#10;\" />\n<PAGE value=\"&quot;^tfa vi elsker dette Landet, &#10;* Som det stiger frem &#10;Furet, veirbidt over Våndet &#10;Med de tusen Hjem, &#10;Elsker, elsker det og tænker &#10;Paa vor Far og Mor &#10;Og den Saganat, som sænker &#10;Drømmer paa vor Jord. &#10;Dette Land har Harald bjerget &#10;Med sin Kjæmperad, &#10;Dette Land har Haakon værget, &#10;Medens Øjvind kvad; &#10;Paa det Land har Olav malet &#10;Korset med sit Blod, &#10;Fra dets Høie Sverre talet &#10;Koma midt imod. &#10;Bønder sine Øxer brynte, &#10;Hvor en Hær drog frem; &#10;Tordenskjold langs Kysten lynte, &#10;Saa den lystes hjem. &#10;Kvinder selv stod op og strede, &#10;Som de vare Mænd; &#10;Andre kunde bare græde, &#10;Men det kom igjen! &#10;\" />\n<PAGE value=\"Haarde Tider har vi døiet, &#10;Blev tilsidst forstødt; &#10;Men i værste Nød blaaøiet &#10;Frihed blev os født. &#10;Det gav Faderkraft at bære &#10;Hungersnød og Krig, &#10;Det gav Døden selv sin ære — &#10;Og det gav Forlig! &#10;Fienden sit Vaaben kasted, &#10;Op Visiret foer, &#10;Vi med Undren mod ham hasted; &#10;Thi han var vor Bror. &#10;Drevne frem paa Stand af Skammen &#10;Gik vi søder paa; &#10;Nu vi staar tre Brødre sammen &#10;Og skal saadan staa! &#10;Norske Mand i Hus og Hytte, &#10;Tak din store Gud! &#10;Landet vilde han beskytte, &#10;Skjønt det mørkt saa ud. &#10;Alt, hvad Fædrene har kjæmpet, &#10;Mødrene har grædt, &#10;Har den Herre stille læmpet, &#10;Saa vi vandt vor Ret! &#10;Ja, vi elsker dette Landet, &#10;Som elet stiger frem &#10;Furet, veirbidt over Våndet &#10;Mecl de tusen Hjem. &#10;Og som Fædres Kamp har hævet &#10;Det af Nød til Seir, &#10;Ogsaa vi, nåar det blir krævet, &#10;For dets Fred slaar Leir! &#10;Bjørnstjerne Bjørnson. &#10;\" />\n<PAGE value=\"4 &#10;I KRISTIANIA. H . B.Larsene Bogtrykkeri. &#10;/ &#10;\" />\n</BODY>\n</DjVuTxt>\n</mw-djvu>"
	}`
)

func TestGetXMLPageCount(t *testing.T) {
	var metadata map[string]interface{}
	err := json.Unmarshal([]byte(xmlJSON), &metadata)
	require.NoError(t, err)
	pages := getXMLPageCount(metadata, []string{"xml"})
	assert.Equal(t, 4, pages)
}

func TestFitBoxWidth(t *testing.T) {
	width := fitBoxWidth(691, 1097)
	assert.Equal(t, 161, width)
}
