package mediafire

/*
Package mediafire
Author: Da3zKi7<da3zki7@duck.com>
Date: 2025-09-11

D@' 3z K!7 - The King Of Cracking
*/

type MediafireRenewTokenResponse struct {
	Response struct {
		Action            string `json:"action"`
		SessionToken      string `json:"session_token"`
		Result            string `json:"result"`
		CurrentAPIVersion string `json:"current_api_version"`
	} `json:"response"`
}

type MediafireResponse struct {
	Response struct {
		Action        string `json:"action"`
		FolderContent struct {
			ChunkSize   string            `json:"chunk_size"`
			ContentType string            `json:"content_type"`
			ChunkNumber string            `json:"chunk_number"`
			FolderKey   string            `json:"folderkey"`
			Folders     []MediafireFolder `json:"folders,omitempty"`
			Files       []MediafireFile   `json:"files,omitempty"`
			MoreChunks  string            `json:"more_chunks"`
		} `json:"folder_content"`
		Result string `json:"result"`
	} `json:"response"`
}

type MediafireFolder struct {
	FolderKey  string `json:"folderkey"`
	Name       string `json:"name"`
	Created    string `json:"created"`
	CreatedUTC string `json:"created_utc"`
}

type MediafireFile struct {
	QuickKey   string `json:"quickkey"`
	Filename   string `json:"filename"`
	Size       string `json:"size"`
	Created    string `json:"created"`
	CreatedUTC string `json:"created_utc"`
	MimeType   string `json:"mimetype"`
}

type File struct {
	ID         string
	Name       string
	Size       int64
	CreatedUTC string
	IsFolder   bool
}

type FolderContentResponse struct {
	Folders    []MediafireFolder
	Files      []MediafireFile
	MoreChunks bool
}

type MediafireLinksResponse struct {
	Response struct {
		Action string `json:"action"`
		Links  []struct {
			QuickKey       string `json:"quickkey"`
			View           string `json:"view"`
			NormalDownload string `json:"normal_download"`
			OneTime        struct {
				Download string `json:"download"`
				View     string `json:"view"`
			} `json:"one_time"`
		} `json:"links"`
		OneTimeKeyRequestCount    string `json:"one_time_key_request_count"`
		OneTimeKeyRequestMaxCount string `json:"one_time_key_request_max_count"`
		Result                    string `json:"result"`
		CurrentAPIVersion         string `json:"current_api_version"`
	} `json:"response"`
}

type MediafireDirectDownloadResponse struct {
	Response struct {
		Action string `json:"action"`
		Links  []struct {
			QuickKey       string `json:"quickkey"`
			DirectDownload string `json:"direct_download"`
		} `json:"links"`
		DirectDownloadFreeBandwidth string `json:"direct_download_free_bandwidth"`
		Result                      string `json:"result"`
		CurrentAPIVersion           string `json:"current_api_version"`
	} `json:"response"`
}

type MediafireFolderCreateResponse struct {
	Response struct {
		Action            string `json:"action"`
		FolderKey         string `json:"folder_key"`
		UploadKey         string `json:"upload_key"`
		ParentFolderKey   string `json:"parent_folderkey"`
		Name              string `json:"name"`
		Description       string `json:"description"`
		Created           string `json:"created"`
		CreatedUTC        string `json:"created_utc"`
		Privacy           string `json:"privacy"`
		FileCount         string `json:"file_count"`
		FolderCount       string `json:"folder_count"`
		Revision          string `json:"revision"`
		DropboxEnabled    string `json:"dropbox_enabled"`
		Flag              string `json:"flag"`
		Result            string `json:"result"`
		CurrentAPIVersion string `json:"current_api_version"`
		NewDeviceRevision int    `json:"new_device_revision"`
	} `json:"response"`
}

type MediafireMoveResponse struct {
	Response struct {
		Action            string   `json:"action"`
		Asynchronous      string   `json:"asynchronous,omitempty"`
		NewNames          []string `json:"new_names"`
		Result            string   `json:"result"`
		CurrentAPIVersion string   `json:"current_api_version"`
		NewDeviceRevision int      `json:"new_device_revision"`
	} `json:"response"`
}

type MediafireRenameResponse struct {
	Response struct {
		Action            string `json:"action"`
		Asynchronous      string `json:"asynchronous,omitempty"`
		Result            string `json:"result"`
		CurrentAPIVersion string `json:"current_api_version"`
		NewDeviceRevision int    `json:"new_device_revision"`
	} `json:"response"`
}

type MediafireCopyResponse struct {
	Response struct {
		Action            string   `json:"action"`
		Asynchronous      string   `json:"asynchronous,omitempty"`
		NewQuickKeys      []string `json:"new_quickkeys,omitempty"`
		NewFolderKeys     []string `json:"new_folderkeys,omitempty"`
		SkippedCount      string   `json:"skipped_count,omitempty"`
		OtherCount        string   `json:"other_count,omitempty"`
		Result            string   `json:"result"`
		CurrentAPIVersion string   `json:"current_api_version"`
		NewDeviceRevision int      `json:"new_device_revision"`
	} `json:"response"`
}

type MediafireRemoveResponse struct {
	Response struct {
		Action            string `json:"action"`
		Asynchronous      string `json:"asynchronous,omitempty"`
		Result            string `json:"result"`
		CurrentAPIVersion string `json:"current_api_version"`
		NewDeviceRevision int    `json:"new_device_revision"`
	} `json:"response"`
}

type MediafireCheckResponse struct {
	Response struct {
		Action          string `json:"action"`
		HashExists      string `json:"hash_exists"`
		InAccount       string `json:"in_account"`
		InFolder        string `json:"in_folder"`
		FileExists      string `json:"file_exists"`
		ResumableUpload struct {
			AllUnitsReady string `json:"all_units_ready"`
			NumberOfUnits string `json:"number_of_units"`
			UnitSize      string `json:"unit_size"`
			Bitmap        struct {
				Count string   `json:"count"`
				Words []string `json:"words"`
			} `json:"bitmap"`
			UploadKey string `json:"upload_key"`
		} `json:"resumable_upload"`
		AvailableSpace       string `json:"available_space"`
		UsedStorageSize      string `json:"used_storage_size"`
		StorageLimit         string `json:"storage_limit"`
		StorageLimitExceeded string `json:"storage_limit_exceeded"`
		UploadURL            struct {
			Simple            string `json:"simple"`
			SimpleFallback    string `json:"simple_fallback"`
			Resumable         string `json:"resumable"`
			ResumableFallback string `json:"resumable_fallback"`
		} `json:"upload_url"`
		Result            string `json:"result"`
		CurrentAPIVersion string `json:"current_api_version"`
	} `json:"response"`
}
type MediafireActionTokenResponse struct {
	Response struct {
		Action            string `json:"action"`
		ActionToken       string `json:"action_token"`
		Result            string `json:"result"`
		CurrentAPIVersion string `json:"current_api_version"`
	} `json:"response"`
}

type MediafirePollResponse struct {
	Response struct {
		Action   string `json:"action"`
		Doupload struct {
			Result      string `json:"result"`
			Status      string `json:"status"`
			Description string `json:"description"`
			QuickKey    string `json:"quickkey"`
			Hash        string `json:"hash"`
			Filename    string `json:"filename"`
			Size        string `json:"size"`
			Created     string `json:"created"`
			CreatedUTC  string `json:"created_utc"`
			Revision    string `json:"revision"`
		} `json:"doupload"`
		Result            string `json:"result"`
		CurrentAPIVersion string `json:"current_api_version"`
	} `json:"response"`
}

type MediafireFileSearchResponse struct {
	Response struct {
		Action            string `json:"action"`
		FileInfo          []File `json:"file_info"`
		Result            string `json:"result"`
		CurrentAPIVersion string `json:"current_api_version"`
	} `json:"response"`
}
