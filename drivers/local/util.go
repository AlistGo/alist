package local

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/alist-org/alist/v3/internal/conf"
	"github.com/alist-org/alist/v3/internal/model"
	"github.com/alist-org/alist/v3/pkg/utils"
	"github.com/disintegration/imaging"
	ffmpeg "github.com/u2takey/ffmpeg-go"
)

func isSymlinkDir(f fs.FileInfo, path string) bool {
	if f.Mode()&os.ModeSymlink == os.ModeSymlink {
		dst, err := os.Readlink(filepath.Join(path, f.Name()))
		if err != nil {
			return false
		}
		if !filepath.IsAbs(dst) {
			dst = filepath.Join(path, dst)
		}
		stat, err := os.Stat(dst)
		if err != nil {
			return false
		}
		return stat.IsDir()
	}
	return false
}

// Get the snapshot of the video
func (d *Local) GetSnapshot(videoPath string) (imgData *bytes.Buffer, err error) {
	// Run ffprobe to get the video duration
	jsonOutput, err := ffmpeg.Probe(videoPath)
	if err != nil {
		return nil, err
	}
	// get format.duration from the json string
	type probeFormat struct {
		Duration string `json:"duration"`
	}
	type probeData struct {
		Format probeFormat `json:"format"`
	}
	var probe probeData
	err = json.Unmarshal([]byte(jsonOutput), &probe)
	if err != nil {
		return nil, err
	}
	totalDuration, err := strconv.ParseFloat(probe.Format.Duration, 64)
	if err != nil {
		return nil, err
	}

	var ss string
	if d.videoThumbPosIsPercentage {
		ss = fmt.Sprintf("%f", totalDuration*d.videoThumbPos)
	} else {
		// If the value is greater than the total duration, use the total duration
		if d.videoThumbPos > totalDuration {
			ss = fmt.Sprintf("%f", totalDuration)
		} else {
			ss = fmt.Sprintf("%f", d.videoThumbPos)
		}
	}

	// Run ffmpeg to get the snapshot
	srcBuf := bytes.NewBuffer(nil)
	// If the remaining time from the seek point to the end of the video is less
	// than the duration of a single frame, ffmpeg cannot extract any frames
	// within the specified range and will exit with an error.
	// The "noaccurate_seek" option prevents this error and would also speed up
	// the seek process.
	stream := ffmpeg.Input(videoPath, ffmpeg.KwArgs{"ss": ss, "noaccurate_seek": ""}).
		Output("pipe:", ffmpeg.KwArgs{"vframes": 1, "format": "image2", "vcodec": "mjpeg"}).
		GlobalArgs("-loglevel", "error").Silent(true).
		WithOutput(srcBuf, os.Stdout)
	if err = stream.Run(); err != nil {
		return nil, err
	}
	return srcBuf, nil
}

func readDir(dirname string) ([]fs.FileInfo, error) {
	f, err := os.Open(dirname)
	if err != nil {
		return nil, err
	}
	list, err := f.Readdir(-1)
	f.Close()
	if err != nil {
		return nil, err
	}
	sort.Slice(list, func(i, j int) bool { return list[i].Name() < list[j].Name() })
	return list, nil
}

func (d *Local) getThumb(file model.Obj) (*bytes.Buffer, *string, error) {
	fullPath := file.GetPath()
	thumbPrefix := "alist_thumb_"
	thumbName := thumbPrefix + utils.GetMD5EncodeStr(fullPath) + ".png"
	if d.ThumbCacheFolder != "" {
		// skip if the file is a thumbnail
		if strings.HasPrefix(file.GetName(), thumbPrefix) {
			return nil, &fullPath, nil
		}
		thumbPath := filepath.Join(d.ThumbCacheFolder, thumbName)
		if utils.Exists(thumbPath) {
			return nil, &thumbPath, nil
		}
	}
	var srcBuf *bytes.Buffer
	if utils.GetFileType(file.GetName()) == conf.VIDEO {
		videoBuf, err := d.GetSnapshot(fullPath)
		if err != nil {
			return nil, nil, err
		}
		srcBuf = videoBuf
	} else {
		imgData, err := os.ReadFile(fullPath)
		if err != nil {
			return nil, nil, err
		}
		imgBuf := bytes.NewBuffer(imgData)
		srcBuf = imgBuf
	}

	image, err := imaging.Decode(srcBuf, imaging.AutoOrientation(true))
	if err != nil {
		return nil, nil, err
	}
	thumbImg := imaging.Resize(image, 144, 0, imaging.Lanczos)
	var buf bytes.Buffer
	err = imaging.Encode(&buf, thumbImg, imaging.PNG)
	if err != nil {
		return nil, nil, err
	}
	if d.ThumbCacheFolder != "" {
		err = os.WriteFile(filepath.Join(d.ThumbCacheFolder, thumbName), buf.Bytes(), 0666)
		if err != nil {
			return nil, nil, err
		}
	}
	return &buf, nil, nil
}
