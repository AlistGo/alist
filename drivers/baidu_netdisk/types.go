package baidu_netdisk

import (
	"path"
	"strconv"
	"time"

	"github.com/alist-org/alist/v3/internal/model"
	"github.com/alist-org/alist/v3/pkg/utils"
)

type TokenErrResp struct {
	ErrorDescription string `json:"error_description"`
	Error            string `json:"error"`
}

type File struct {
	//TkbindId     int    `json:"tkbind_id"`
	//OwnerType    int    `json:"owner_type"`
	Category int `json:"category"`
	//RealCategory string `json:"real_category"`
	FsId int64 `json:"fs_id"`
	//OperId      int   `json:"oper_id"`
	Thumbs struct {
		//Icon string `json:"icon"`
		Url3 string `json:"url3"`
		//Url2 string `json:"url2"`
		//Url1 string `json:"url1"`
	} `json:"thumbs"`
	//Wpfile         int    `json:"wpfile"`

	Size int64 `json:"size"`
	//ExtentTinyint7 int    `json:"extent_tinyint7"`
	Path string `json:"path"`
	//Share          int    `json:"share"`
	//Pl             int    `json:"pl"`
	ServerFilename string `json:"server_filename"`
	Md5            string `json:"md5"`
	//OwnerId        int    `json:"owner_id"`
	//Unlist int `json:"unlist"`
	Isdir int `json:"isdir"`

	// list resp
	ServerCtime int64 `json:"server_ctime"`
	ServerMtime int64 `json:"server_mtime"`
	LocalMtime  int64 `json:"local_mtime"`
	LocalCtime  int64 `json:"local_ctime"`
	//ServerAtime    int64    `json:"server_atime"` `

	// only create and precreate resp
	Ctime int64 `json:"ctime"`
	Mtime int64 `json:"mtime"`
}

func fileToObj(f File) *model.ObjThumb {
	if f.ServerFilename == "" {
		f.ServerFilename = path.Base(f.Path)
	}
	if f.ServerCtime == 0 {
		f.ServerCtime = f.Ctime
	}
	if f.ServerMtime == 0 {
		f.ServerMtime = f.Mtime
	}
	return &model.ObjThumb{
		Object: model.Object{
			ID:       strconv.FormatInt(f.FsId, 10),
			Path:     f.Path,
			Name:     f.ServerFilename,
			Size:     f.Size,
			Modified: time.Unix(f.ServerMtime, 0),
			Ctime:    time.Unix(f.ServerCtime, 0),
			IsFolder: f.Isdir == 1,

			// 直接获取的MD5是错误的
			HashInfo: utils.NewHashInfo(utils.MD5, DecryptMd5(f.Md5)),
		},
		Thumbnail: model.Thumbnail{Thumbnail: f.Thumbs.Url3},
	}
}

type ListResp struct {
	Errno     int    `json:"errno"`
	GuidInfo  string `json:"guid_info"`
	List      []File `json:"list"`
	RequestId int64  `json:"request_id"`
	Guid      int    `json:"guid"`
}

type DownloadResp struct {
	Errmsg string `json:"errmsg"`
	Errno  int    `json:"errno"`
	List   []struct {
		//Category    int    `json:"category"`
		//DateTaken   int    `json:"date_taken,omitempty"`
		Dlink string `json:"dlink"`
		//Filename    string `json:"filename"`
		//FsId        int64  `json:"fs_id"`
		//Height      int    `json:"height,omitempty"`
		//Isdir       int    `json:"isdir"`
		//Md5         string `json:"md5"`
		//OperId      int    `json:"oper_id"`
		//Path        string `json:"path"`
		//ServerCtime int    `json:"server_ctime"`
		//ServerMtime int    `json:"server_mtime"`
		//Size        int    `json:"size"`
		//Thumbs      struct {
		//	Icon string `json:"icon,omitempty"`
		//	Url1 string `json:"url1,omitempty"`
		//	Url2 string `json:"url2,omitempty"`
		//	Url3 string `json:"url3,omitempty"`
		//} `json:"thumbs"`
		//Width int `json:"width,omitempty"`
	} `json:"list"`
	//Names struct {
	//} `json:"names"`
	RequestId string `json:"request_id"`
}

type DownloadResp2 struct {
	Errno int `json:"errno"`
	Info  []struct {
		//ExtentTinyint4 int `json:"extent_tinyint4"`
		//ExtentTinyint1 int `json:"extent_tinyint1"`
		//Bitmap string `json:"bitmap"`
		//Category int `json:"category"`
		//Isdir int `json:"isdir"`
		//Videotag int `json:"videotag"`
		Dlink string `json:"dlink"`
		//OperID int64 `json:"oper_id"`
		//PathMd5 int `json:"path_md5"`
		//Wpfile int `json:"wpfile"`
		//LocalMtime int `json:"local_mtime"`
		/*Thumbs struct {
			Icon string `json:"icon"`
			URL3 string `json:"url3"`
			URL2 string `json:"url2"`
			URL1 string `json:"url1"`
		} `json:"thumbs"`*/
		//PlaySource int `json:"play_source"`
		//Share int `json:"share"`
		//FileKey string `json:"file_key"`
		//Errno int `json:"errno"`
		//LocalCtime int `json:"local_ctime"`
		//Rotate int `json:"rotate"`
		//Metadata time.Time `json:"metadata"`
		//Height int `json:"height"`
		//SampleRate int `json:"sample_rate"`
		//Width int `json:"width"`
		//OwnerType int `json:"owner_type"`
		//Privacy int `json:"privacy"`
		//ExtentInt3 int64 `json:"extent_int3"`
		//RealCategory string `json:"real_category"`
		//SrcLocation string `json:"src_location"`
		//MetaInfo string `json:"meta_info"`
		//ID string `json:"id"`
		//Duration int `json:"duration"`
		//FileSize string `json:"file_size"`
		//Channels int `json:"channels"`
		//UseSegment int `json:"use_segment"`
		//ServerCtime int `json:"server_ctime"`
		//Resolution string `json:"resolution"`
		//OwnerID int `json:"owner_id"`
		//ExtraInfo string `json:"extra_info"`
		//Size int `json:"size"`
		//FsID int64 `json:"fs_id"`
		//ExtentTinyint3 int `json:"extent_tinyint3"`
		//Md5 string `json:"md5"`
		//Path string `json:"path"`
		//FrameRate int `json:"frame_rate"`
		//ExtentTinyint2 int `json:"extent_tinyint2"`
		//ServerFilename string `json:"server_filename"`
		//ServerMtime int `json:"server_mtime"`
		//TkbindID int `json:"tkbind_id"`
	} `json:"info"`
	RequestID int64 `json:"request_id"`
}

type PrecreateResp struct {
	Errno      int   `json:"errno"`
	RequestId  int64 `json:"request_id"`
	ReturnType int   `json:"return_type"`

	// return_type=1
	Path      string `json:"path"`
	Uploadid  string `json:"uploadid"`
	BlockList []int  `json:"block_list"`

	// return_type=2
	File File `json:"info"`
}
