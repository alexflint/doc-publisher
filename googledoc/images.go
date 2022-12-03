package googledoc

import (
	"fmt"
	"regexp"
)

// regular expression for finding image references in HTML-exported google docs
var imageRegexp = regexp.MustCompile(`images\/image\d+\.(png|jpg)`)

// MatchOBjectIDsToImages creates a map from Google Doc object IDs to the URL of the corresponding image
func MatchObjectIDsToImages(d *Archive, imageURLs []string) (map[string]string, error) {
	if len(imageURLs) != len(d.Images) {
		return nil, fmt.Errorf("google doc contained %d images but %d URLs were passed in",
			len(d.Images), len(imageURLs))
	}

	// create a map from image filenames to their URLs
	imageURLByFilename := make(map[string]string)
	for i, img := range d.Images {
		imageURLByFilename[img.Filename] = imageURLs[i]
	}

	// create a list of image filenames *in the order they appear in the HTML*
	imageFilenames := imageRegexp.FindAll(d.HTML, -1)
	var inlineObjectIDs []string
	for _, elem := range d.Doc.Body.Content {
		if elem.Paragraph == nil {
			continue
		}
		for _, e := range elem.Paragraph.Elements {
			if e.InlineObjectElement == nil {
				continue
			}

			obj, ok := d.Doc.InlineObjects[e.InlineObjectElement.InlineObjectId]
			if !ok {
				continue
			}

			emb := obj.InlineObjectProperties.EmbeddedObject
			if emb.EmbeddedDrawingProperties == nil && emb.ImageProperties == nil {
				continue
			}

			inlineObjectIDs = append(inlineObjectIDs, e.InlineObjectElement.InlineObjectId)
		}
	}

	// check that the number of images in the HTML matches the number of images in the document
	if len(inlineObjectIDs) != len(imageFilenames) {
		return nil, fmt.Errorf("found %d images in the HTML but %d inline objects in the document",
			len(imageFilenames), len(inlineObjectIDs))
	}

	// make a map from InlineObjectID to URL
	imageURLByObjectID := make(map[string]string)
	for i := range inlineObjectIDs {
		imageURLByObjectID[inlineObjectIDs[i]] = imageURLByFilename[string(imageFilenames[i])]
	}

	return imageURLByObjectID, nil
}
