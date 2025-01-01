package github

import (
	"context"
	"errors"
	"fmt"
	"github.com/alist-org/alist/v3/internal/model"
	"github.com/alist-org/alist/v3/pkg/utils"
	"github.com/go-resty/resty/v2"
	"io"
	"math"
	"strings"
	"text/template"
)

type ReaderWithProgress struct {
	Reader   io.Reader
	Length   int64
	Progress func(percentage float64)
	offset   int64
}

func (r *ReaderWithProgress) Read(p []byte) (int, error) {
	n, err := r.Reader.Read(p)
	r.offset += int64(n)
	r.Progress(math.Min(100.0, float64(r.offset)/float64(r.Length)*100.0))
	return n, err
}

type MessageTemplateVars struct {
	UserName   string
	ObjName    string
	ObjPath    string
	ParentName string
	ParentPath string
	TargetName string
	TargetPath string
}

func getMessage(tmpl *template.Template, vars *MessageTemplateVars, defaultOpStr string) (string, error) {
	sb := strings.Builder{}
	if err := tmpl.Execute(&sb, vars); err != nil {
		return fmt.Sprintf("%s %s %s", vars.UserName, defaultOpStr, vars.ObjPath), err
	}
	return sb.String(), nil
}

func calculateBase64Length(inputLength int64) int64 {
	return 4 * ((inputLength + 2) / 3)
}

func toErr(res *resty.Response) error {
	var errMsg ErrResp
	if err := utils.Json.Unmarshal(res.Body(), &errMsg); err != nil {
		return errors.New(res.Status())
	} else {
		return fmt.Errorf("%s: %s", res.Status(), errMsg.Message)
	}
}

// Example input:
// a = /aaa/bbb/ccc
// b = /aaa/b11/ddd/ccc
//
// Output:
// ancestor = /aaa
// aChildName = bbb
// bChildName = b11
// aRest = bbb/ccc
// bRest = b11/ddd/ccc
func getPathCommonAncestor(a, b string) (ancestor, aChildName, bChildName, aRest, bRest string) {
	a = utils.FixAndCleanPath(a)
	b = utils.FixAndCleanPath(b)
	idx := 1
	for idx < len(a) && idx < len(b) {
		if a[idx] != b[idx] {
			break
		}
		idx++
	}
	aNextIdx := idx
	for aNextIdx < len(a) {
		if a[aNextIdx] == '/' {
			break
		}
		aNextIdx++
	}
	bNextIdx := idx
	for bNextIdx < len(b) {
		if b[bNextIdx] == '/' {
			break
		}
		bNextIdx++
	}
	for idx > 0 {
		if a[idx] == '/' {
			break
		}
		idx--
	}
	ancestor = utils.FixAndCleanPath(a[:idx])
	aChildName = a[idx+1 : aNextIdx]
	bChildName = b[idx+1 : bNextIdx]
	aRest = a[idx+1:]
	bRest = b[idx+1:]
	return ancestor, aChildName, bChildName, aRest, bRest
}

func getUsername(ctx context.Context) string {
	user, ok := ctx.Value("user").(*model.User)
	if !ok {
		return "<system>"
	}
	return user.Username
}
