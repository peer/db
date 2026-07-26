package icu_test

// This file holds assertions ported from Lucene's ICU analysis tests (TestICUFoldingFilter
// and TestICUTokenizer), the reference behaviour of the Elasticsearch icu_folding filter and
// icu_tokenizer. The strings are kept as the original Unicode so they can be compared against the
// Lucene source. Cases the UAX#29 approximation cannot reproduce are in TestTokenizeLuceneGaps.

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"gitlab.com/peerdb/peerdb/internal/icu"
)

// TestFoldLucene ports Lucene TestICUFoldingFilter.testDefaults.
func TestFoldLucene(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		in   string
		want []string
	}{
		{"This is a test", []string{"this", "is", "a", "test"}},
		{"Ruß", []string{"russ"}},
		{"ΜΆΪΟΣ", []string{"μαιοσ"}},
		{"Μάϊος", []string{"μαιοσ"}},
		{"𐐖", []string{"𐐾"}},
		{"ﴳﴺﰧ", []string{"طمطمطم"}},
		{"क्\u200dष", []string{"कष"}},
		{"résumé", []string{"resume"}},
		{"résumé", []string{"resume"}},
		{"৭০৬", []string{"706"}},
		{"đis is cræzy", []string{"dis", "is", "craezy"}},
		{"ELİF", []string{"elif"}},
		{"eli̇f", []string{"elif"}},
	} {
		assert.Equal(t, tt.want, foldWhitespace(tt.in), "input %q", tt.in)
	}
}

// TestTokenizeLucene ports the Lucene TestICUTokenizer assertions this UAX#29 approximation
// reproduces (Latin including the Hebrew acronym rules, CJK per ideograph, and the space-separated
// alphabetic scripts).
func TestTokenizeLucene(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		in   string
		want []string
	}{
		{"Վիքիպեդիայի 13 միլիոն հոդվածները (4,600` հայերեն վիքիպեդիայում) գրվել են կամավորների կողմից ու համարյա բոլոր հոդվածները կարող է խմբագրել ցանկաց մարդ ով կարող է բացել Վիքիպեդիայի կայքը։", []string{"Վիքիպեդիայի", "13", "միլիոն", "հոդվածները", "4,600", "հայերեն", "վիքիպեդիայում", "գրվել", "են", "կամավորների", "կողմից", "ու", "համարյա", "բոլոր", "հոդվածները", "կարող", "է", "խմբագրել", "ցանկաց", "մարդ", "ով", "կարող", "է", "բացել", "Վիքիպեդիայի", "կայքը"}}, //nolint:lll
		{"ዊኪፔድያ የባለ ብዙ ቋንቋ የተሟላ ትክክለኛና ነጻ መዝገበ ዕውቀት (ኢንሳይክሎፒዲያ) ነው። ማንኛውም", []string{"ዊኪፔድያ", "የባለ", "ብዙ", "ቋንቋ", "የተሟላ", "ትክክለኛና", "ነጻ", "መዝገበ", "ዕውቀት", "ኢንሳይክሎፒዲያ", "ነው", "ማንኛውም"}},                                                                                                                                                                                                                                                                                            //nolint:lll
		{"الفيلم الوثائقي الأول عن ويكيبيديا يسمى \"الحقيقة بالأرقام: قصة ويكيبيديا\" (بالإنجليزية: Truth in Numbers: The Wikipedia Story)، سيتم إطلاقه في 2008.", []string{"الفيلم", "الوثائقي", "الأول", "عن", "ويكيبيديا", "يسمى", "الحقيقة", "بالأرقام", "قصة", "ويكيبيديا", "بالإنجليزية", "Truth", "in", "Numbers", "The", "Wikipedia", "Story", "سيتم", "إطلاقه", "في", "2008"}},                                                                                           //nolint:lll
		{"ܘܝܩܝܦܕܝܐ (ܐܢܓܠܝܐ: Wikipedia) ܗܘ ܐܝܢܣܩܠܘܦܕܝܐ ܚܐܪܬܐ ܕܐܢܛܪܢܛ ܒܠܫܢ̈ܐ ܣܓܝܐ̈ܐ܂ ܫܡܗ ܐܬܐ ܡܢ ܡ̈ܠܬܐ ܕ\"ܘܝܩܝ\" ܘ\"ܐܝܢܣܩܠܘܦܕܝܐ\"܀", []string{"ܘܝܩܝܦܕܝܐ", "ܐܢܓܠܝܐ", "Wikipedia", "ܗܘ", "ܐܝܢܣܩܠܘܦܕܝܐ", "ܚܐܪܬܐ", "ܕܐܢܛܪܢܛ", "ܒܠܫܢ̈ܐ", "ܣܓܝܐ̈ܐ", "ܫܡܗ", "ܐܬܐ", "ܡܢ", "ܡ̈ܠܬܐ", "ܕ", "ܘܝܩܝ", "ܘ", "ܐܝܢܣܩܠܘܦܕܝܐ"}},                                                                                                                                                                         //nolint:lll
		{"এই বিশ্বকোষ পরিচালনা করে উইকিমিডিয়া ফাউন্ডেশন (একটি অলাভজনক সংস্থা)। উইকিপিডিয়ার শুরু ১৫ জানুয়ারি, ২০০১ সালে। এখন পর্যন্ত ২০০টিরও বেশী ভাষায় উইকিপিডিয়া রয়েছে।", []string{"এই", "বিশ্বকোষ", "পরিচালনা", "করে", "উইকিমিডিয়া", "ফাউন্ডেশন", "একটি", "অলাভজনক", "সংস্থা", "উইকিপিডিয়ার", "শুরু", "১৫", "জানুয়ারি", "২০০১", "সালে", "এখন", "পর্যন্ত", "২০০টিরও", "বেশী", "ভাষায়", "উইকিপিডিয়া", "রয়েছে"}},                                                       //nolint:lll
		{"ویکی پدیای انگلیسی در تاریخ ۲۵ دی ۱۳۷۹ به صورت مکملی برای دانشنامهٔ تخصصی نوپدیا نوشته شد.", []string{"ویکی", "پدیای", "انگلیسی", "در", "تاریخ", "۲۵", "دی", "۱۳۷۹", "به", "صورت", "مکملی", "برای", "دانشنامهٔ", "تخصصی", "نوپدیا", "نوشته", "شد"}},                                                                                                                                                                                                                     //nolint:lll
		{"Γράφεται σε συνεργασία από εθελοντές με το λογισμικό wiki, κάτι που σημαίνει ότι άρθρα μπορεί να προστεθούν ή να αλλάξουν από τον καθένα.", []string{"Γράφεται", "σε", "συνεργασία", "από", "εθελοντές", "με", "το", "λογισμικό", "wiki", "κάτι", "που", "σημαίνει", "ότι", "άρθρα", "μπορεί", "να", "προστεθούν", "ή", "να", "αλλάξουν", "από", "τον", "καθένα"}},                                                                                                      //nolint:lll
		{"སྣོན་མཛོད་དང་ལས་འདིས་བོད་ཡིག་མི་ཉམས་གོང་འཕེལ་དུ་གཏོང་བར་ཧ་ཅང་དགེ་མཚན་མཆིས་སོ། །", []string{"སྣོན", "མཛོད", "དང", "ལས", "འདིས", "བོད", "ཡིག", "མི", "ཉམས", "གོང", "འཕེལ", "དུ", "གཏོང", "བར", "ཧ", "ཅང", "དགེ", "མཚན", "མཆིས", "སོ"}},                                                                                                                                                                                                                                    //nolint:lll
		{"我是中国人。 １２３４ Ｔｅｓｔｓ ", []string{"我", "是", "中", "国", "人", "１２３４", "Ｔｅｓｔｓ"}}, //nolint:gosmopolitan
		{"דנקנר תקף את הדו\"ח", []string{"דנקנר", "תקף", "את", "הדו\"ח"}},
		{"חברת בת של מודי'ס", []string{"חברת", "בת", "של", "מודי'ס"}},
		{"", nil},
		{".", nil},
		{" ", nil},
		{"moͤchte", []string{"moͤchte"}},
		{"B2B", []string{"B2B"}},
		{"2B", []string{"2B"}},
		{"some-dashed-phrase", []string{"some", "dashed", "phrase"}},
		{"dogs,chase,cats", []string{"dogs", "chase", "cats"}},
		{"ac/dc", []string{"ac", "dc"}},
		{"O'Reilly", []string{"O'Reilly"}},
		{"you're", []string{"you're"}},
		{"she's", []string{"she's"}},
		{"Jim's", []string{"Jim's"}},
		{"don't", []string{"don't"}},
		{"O'Reilly's", []string{"O'Reilly's"}},
		{"21.35", []string{"21.35"}},
		{"R2D2 C3PO", []string{"R2D2", "C3PO"}},
		{"216.239.63.104", []string{"216.239.63.104"}},
		{"216.239.63.104", []string{"216.239.63.104"}},
		{"David has 5000 bones", []string{"David", "has", "5000", "bones"}},
		{"C embedded developers wanted", []string{"C", "embedded", "developers", "wanted"}},
		{"foo bar FOO BAR", []string{"foo", "bar", "FOO", "BAR"}},
		{"foo      bar .  FOO <> BAR", []string{"foo", "bar", "FOO", "BAR"}},
		{"\"QUOTED\" word", []string{"QUOTED", "word"}},
		{"안녕하세요 한글입니다", []string{"안녕하세요", "한글입니다"}},
		{"སྣོན་མཛོད་དང་ལས་འདིས་བོད་ཡིག་མི་ཉམས་གོང་འཕེལ་དུ་གཏོང་བར་ཧ་ཅང་དགེ་མཚན་མཆིས་སོ། །", []string{"སྣོན", "མཛོད", "དང", "ལས", "འདིས", "བོད", "ཡིག", "མི", "ཉམས", "གོང", "འཕེལ", "དུ", "གཏོང", "བར", "ཧ", "ཅང", "དགེ", "མཚན", "མཆིས", "སོ"}}, //nolint:lll
		{"David has 5000 bones", []string{"David", "has", "5000", "bones"}},
		{"David has 5000 bones", []string{"David", "has", "5000", "bones"}},
		{"훈민정음", []string{"훈민정음"}},
		{"仮名遣い カタカナ", []string{"仮", "名", "遣", "い", "カタカナ"}}, //nolint:gosmopolitan
		{"3️⃣", []string{"3️⃣"}},
		{"𑅗०", []string{"𑅗०"}},
		{"𑅗ा", []string{"𑅗ा"}},
		{"𑅗᪾", []string{"𑅗᪾"}},
	} {
		assert.Equal(t, tt.want, icu.Tokenize(tt.in), "input %q", tt.in)
	}
}

// TestTokenizeLuceneGaps holds the Lucene TestICUTokenizer assertions this UAX#29 approximation
// does NOT reproduce: dictionary word segmentation for the spaceless scripts (Thai/Lao/Khmer/Myanmar)
// and emoji sequences (the emoji cases here mix with CJK, but the CJK itself is handled). These need
// ICU's bundled dictionaries and rule break data. It pins what Tokenize produces and asserts it differs
// from the ICU result, so each gap is explicit. The icu field is the ICU output.
func TestTokenizeLuceneGaps(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		in   string
		icu  []string
		ours []string
	}{
		{"ផ្ទះស្កឹមស្កៃបីបួនខ្នងនេះ", []string{"ផ្ទះ", "ស្កឹមស្កៃ", "បី", "បួន", "ខ្នង", "នេះ"}, []string{"ផ្ទះស្កឹមស្កៃបីបួនខ្នងនេះ"}},
		{"ກວ່າດອກ", []string{"ກວ່າ", "ດອກ"}, []string{"ກວ່າດອກ"}},
		{"ພາສາລາວ", []string{"ພາສາ", "ລາວ"}, []string{"ພາສາລາວ"}},
		{"သက်ဝင်လှုပ်ရှားစေပြီး", []string{"သက်ဝင်", "လှုပ်ရှား", "စေ", "ပြီး"}, []string{"သက်ဝင်လှုပ်ရှားစေပြီး"}},
		{"การที่ได้ต้องแสดงว่างานดี. แล้วเธอจะไปไหน? ๑๒๓๔", []string{"การ", "ที่", "ได้", "ต้อง", "แสดง", "ว่า", "งาน", "ดี", "แล้ว", "เธอ", "จะ", "ไป", "ไหน", "๑๒๓๔"}, []string{"การที่ได้ต้องแสดงว่างานดี", "แล้วเธอจะไปไหน", "๑๒๓๔"}}, //nolint:lll
		{"💩 💩💩", []string{"💩", "💩", "💩"}, nil},
		{"👩\u200d❤️\u200d👩", []string{"👩\u200d❤️\u200d👩"}, nil},
		{"👨🏼\u200d⚕️", []string{"👨🏼\u200d⚕️"}, nil},
		{"🇺🇸🇺🇸", []string{"🇺🇸", "🇺🇸"}, nil},
		{"#️⃣", []string{"#️⃣"}, nil},
		{"🏴\U000e0067\U000e0062\U000e0065\U000e006e\U000e0067\U000e007f", []string{"🏴\U000e0067\U000e0062\U000e0065\U000e006e\U000e0067\U000e007f"}, nil},
		{"poo💩poo", []string{"poo", "💩", "poo"}, []string{"poo", "poo"}},
		{"💩中國💩", []string{"💩", "中", "國", "💩"}, []string{"中", "國"}}, //nolint:gosmopolitan
	} {
		got := icu.Tokenize(tt.in)
		assert.Equal(t, tt.ours, got, "input %q", tt.in)
		assert.NotEqual(t, tt.icu, got, "input %q should be a known gap", tt.in)
	}
}

// foldWhitespace folds each whitespace-separated word of s, mirroring the Lucene ICUFoldingFilter
// test setup (a whitespace tokenizer followed by the folding filter).
func foldWhitespace(s string) []string {
	words := strings.Fields(s)
	out := make([]string, 0, len(words))
	for _, w := range words {
		out = append(out, icu.Fold(w))
	}
	return out
}
