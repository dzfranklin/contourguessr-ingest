package repos

type Exif []ExifTag

type ExifTag struct {
	Tag string `json:"tag"`
	Raw struct {
		Content string `json:"_content"`
	}
}

func (e Exif) GetRaw(tag string) (string, bool) {
	for _, t := range e {
		if t.Tag == tag {
			return t.Raw.Content, true
		}
	}
	return "", false
}
