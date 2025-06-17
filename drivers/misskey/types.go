package misskey

type Resp struct {
	Code int
	Raw  []byte
}

type Properties struct {
	Width  int `json:"width"`
	Height int `json:"height"`
}

type MFile struct {
	ID           string     `json:"id"`
	CreatedAt    string     `json:"createdAt"`
	Name         string     `json:"name"`
	Type         string     `json:"type"`
	MD5          string     `json:"md5"`
	Size         int64      `json:"size"`
	IsSensitive  bool       `json:"isSensitive"`
	Blurhash     string     `json:"blurhash"`
	Properties   Properties `json:"properties"`
	URL          string     `json:"url"`
	ThumbnailURL string     `json:"thumbnailUrl"`
	Comment      *string    `json:"comment"`
	FolderID     *string    `json:"folderId"`
	Folder       MFolder    `json:"folder"`
}

type MFolder struct {
	ID        string  `json:"id"`
	CreatedAt string  `json:"createdAt"`
	Name      string  `json:"name"`
	ParentID  *string `json:"parentId"`
}
