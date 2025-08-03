package _123Open

import (
	"fmt"
	"net/http"
)

func (d *Open123) getFiles(parentFileId int64, limit int, lastFileId int64) (*FileListResp, error) {
	var result FileListResp
	url := fmt.Sprintf("%s?parentFileId=%d&limit=%d&lastFileId=%d", ApiFileList, parentFileId, limit, lastFileId)

	_, err := d.Request(url, http.MethodGet, nil, &result)
	if err != nil {
		return nil, err
	}
	if result.Code != 0 {
		return nil, fmt.Errorf("list error: %s", result.Message)
	}
	return &result, nil
}
