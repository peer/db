package wikipedia

import (
	"net/url"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"gitlab.com/tozd/go/errors"
)

func ConvertArticle(input string) (string, errors.E) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(input))
	if err != nil {
		return "", errors.WithStack(err)
	}
	doc.Find("style, script, noscript, iframe").Remove()
	doc.Find(".noprint").Not("[role='presentation']").Remove()
	// TODO: Extract links into metadata.
	doc.Find("link, [role='navigation']").Remove()
	// TODO: Extract boxes into metadata.
	doc.Find(".infobox, .ambox, .ombox, .vcard, [role='note'], .sistersitebox").Remove()
	doc.Find(".sidebar").Has(".sidebar-list").Remove()
	// TODO: Extract coordinates into metadata.
	doc.Find("#coordinates").Remove()
	// TODO: Extract references into annotations.
	doc.Find(".reference").Remove()
	// Make URLs which start with // into https:// URLs. Easier to work with.
	for _, conf := range []struct {
		Tag       string
		Attribute string
	}{
		{"img", "src"},
		{"source", "src"},
		{"track", "src"},
		{"video", "poster"},
	} {
		doc.Find(conf.Tag).Each(func(_ int, el *goquery.Selection) {
			value, ok := el.Attr(conf.Attribute)
			if !ok {
				return
			}
			parsedValue, err := url.Parse(value)
			if err != nil {
				return
			}
			if parsedValue.Host != "" && parsedValue.Scheme == "" {
				parsedValue.Scheme = "https"
				el.SetAttr(conf.Attribute, parsedValue.String())
			}
		})
	}
	doc.Find("img").Each(func(_ int, img *goquery.Selection) {
		srcset, ok := img.Attr("srcset")
		if !ok {
			return
		}
		srcsetArray := strings.Split(srcset, ",")
		newSrcsetArray := []string{}
		for _, srcsetElement := range srcsetArray {
			elementArray := strings.Split(strings.TrimSpace(srcsetElement), " ")
			if len(elementArray) != 2 {
				newSrcsetArray = append(newSrcsetArray, srcsetElement)
				continue
			}
			parsedSrc, err := url.Parse(elementArray[0])
			if err != nil {
				newSrcsetArray = append(newSrcsetArray, srcsetElement)
				continue
			}
			if parsedSrc.Host != "" && parsedSrc.Scheme == "" {
				parsedSrc.Scheme = "https"
				newSrcsetArray = append(newSrcsetArray, strings.Join([]string{parsedSrc.String(), elementArray[1]}, " "))
			} else {
				newSrcsetArray = append(newSrcsetArray, srcsetElement)
			}
		}
		img.SetAttr("srcset", strings.Join(newSrcsetArray, ","))
	})
	doc.Find("a").Each(func(_ int, a *goquery.Selection) {
		href, ok := a.Attr("href")
		if !ok {
			return
		}
		parsedHref, err := url.Parse(href)
		if err != nil {
			return
		}
		if parsedHref.Host != "" && parsedHref.Scheme == "" {
			parsedHref.Scheme = "https"
			a.SetAttr("href", parsedHref.String())
		}
	})
	// Transform quotes into figures.
	doc.Find(".quotebox").Each(func(_ int, quotebox *goquery.Selection) {
		blockquote := quotebox.Find("blockquote")
		cite := quotebox.Find("cite")
		if blockquote.Length() > 0 {
			if cite.Length() > 0 {
				cite = cite.WrapAllHtml("<figcaption></figcaption>").Parent()
			}
			quotebox.ReplaceWithSelection(blockquote.AddSelection(cite).WrapAllHtml("<figure></figure>").Parent())
		}
	})
	doc.Find("blockquote.templatequote").Each(func(_ int, blockquote *goquery.Selection) {
		cite := blockquote.Find(".templatequotecite")
		if cite.Length() > 0 {
			cite = cite.WrapAllHtml("<figcaption></figcaption>").Contents().Unwrap().Parent()
		}
		blockquote.AddSelection(cite).WrapAllHtml("<figure></figure>")
	})
	doc.Find("div.block-indent").Each(func(_ int, block *goquery.Selection) {
		block.WrapAllHtml("<blockquote></blockquote>").Contents().Unwrap()
	})
	// Transform thumbimages into figures.
	doc.Find(".thumbimage").Each(func(_ int, thumbimage *goquery.Selection) {
		thumbcaption := thumbimage.SiblingsFiltered(".thumbcaption")
		if thumbcaption.Length() > 0 {
			thumbcaption = thumbcaption.WrapAllHtml("<figcaption></figcaption>").Contents().Unwrap().Parent()
		}
		figure := thumbimage.Clone().Contents().Unwrap().AddSelection(thumbcaption).WrapAllHtml("<figure></figure>").Parent()
		thumbimage.ReplaceWithSelection(figure)
		for figure.Parent().Not("body, section").Length() > 0 {
			figure = figure.Unwrap().Parent()
		}
	})
	// Transform presentations into figures.
	doc.Find("[role='presentation']").Each(func(_ int, table *goquery.Selection) {
		haudios := table.Find(".haudio")
		appendAfter := table
		haudios.Each(func(_ int, haudio *goquery.Selection) {
			header := haudio.Prev().Not("hr").Clone()
			children := haudio.Children()
			title := children.Eq(0).Clone()
			media := children.Eq(1).Clone()
			description := children.Eq(2).Clone()
			caption := header.AddSelection(title).AddSelection(description).WrapAllHtml("<figcaption></figcaption>").Parent()
			figure := media.AddSelection(caption).WrapAllHtml("<figure></figure>").Parent()
			appendAfter.AfterSelection(figure)
			appendAfter = figure
		})
		if haudios.Length() > 0 {
			table.Remove()
		}
	})
	// Remove help links.
	doc.Find(".audiolinkinfo").Each(func(_ int, audiolinkinfo *goquery.Selection) {
		// Remove help cursor.
		audiolinkinfo.RemoveAttr("style")
		// Remove first link and the dot following it.
		link := audiolinkinfo.Find("[rel='mw:WikiLink']").Eq(0)
		link.AddNodes(link.Nodes[0].NextSibling).Remove()
	})
	// Remove some sections.
	// TODO: Extract to annotations and metadata.
	doc.Find("section").Each(func(_ int, section *goquery.Selection) {
	LEVEL:
		for _, level := range []string{"h1", "h2", "h3", "h4", "h5", "h6"} {
			heading := section.ChildrenFiltered(level).Text()
			for _, h := range []string{"See also", "Online sources", "External links", "References", "Footnotes", "Notes", "Further reading"} {
				if strings.Contains(heading, h) {
					section.Remove()
					break LEVEL
				}
			}
		}
	})
	// TODO: Sanitize using bluemonday.
	doc.Find("*").RemoveAttr("data-mw")
	output, err := doc.Find("body").Html()
	if err != nil {
		return "", errors.WithStack(err)
	}
	return output, nil
}
