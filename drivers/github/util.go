package github

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"text/template"
	"time"

	"github.com/ProtonMail/go-crypto/openpgp"
	"github.com/ProtonMail/go-crypto/openpgp/armor"
	"github.com/alist-org/alist/v3/internal/model"
	"github.com/alist-org/alist/v3/pkg/utils"
	"github.com/go-resty/resty/v2"
)

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

func loadPrivateKey(key, passphrase string) (*openpgp.Entity, error) {
	entityList, err := openpgp.ReadArmoredKeyRing(strings.NewReader(key))
	if err != nil {
		return nil, err
	}
	if len(entityList) < 1 {
		return nil, fmt.Errorf("no keys found in key ring")
	}
	entity := entityList[0]

	pass := []byte(passphrase)
	if entity.PrivateKey != nil && entity.PrivateKey.Encrypted {
		if err = entity.PrivateKey.Decrypt(pass); err != nil {
			return nil, fmt.Errorf("password incorrect: %+v", err)
		}
	}
	for _, subKey := range entity.Subkeys {
		if subKey.PrivateKey != nil && subKey.PrivateKey.Encrypted {
			if err = subKey.PrivateKey.Decrypt(pass); err != nil {
				return nil, fmt.Errorf("password incorrect: %+v", err)
			}
		}
	}
	return entity, nil
}

func signCommit(m *map[string]interface{}, entity *openpgp.Entity) (string, error) {
	var commit strings.Builder
	commit.WriteString(fmt.Sprintf("tree %s\n", (*m)["tree"].(string)))
	parents := (*m)["parents"].([]string)
	for _, p := range parents {
		commit.WriteString(fmt.Sprintf("parent %s\n", p))
	}
	now := time.Now()
	_, offset := now.Zone()
	hour := offset / 3600
	author := (*m)["author"].(map[string]string)
	commit.WriteString(fmt.Sprintf("author %s <%s> %d %+03d00\n", author["name"], author["email"], now.Unix(), hour))
	author["date"] = now.Format(time.RFC3339)
	committer := (*m)["committer"].(map[string]string)
	commit.WriteString(fmt.Sprintf("committer %s <%s> %d %+03d00\n", committer["name"], committer["email"], now.Unix(), hour))
	committer["date"] = now.Format(time.RFC3339)
	commit.WriteString(fmt.Sprintf("\n%s", (*m)["message"].(string)))
	data := commit.String()

	var sigBuffer bytes.Buffer
	err := openpgp.DetachSign(&sigBuffer, entity, strings.NewReader(data), nil)
	if err != nil {
		return "", fmt.Errorf("signing failed: %v", err)
	}
	var armoredSig bytes.Buffer
	armorWriter, err := armor.Encode(&armoredSig, "PGP SIGNATURE", nil)
	if err != nil {
		return "", err
	}
	if _, err = utils.CopyWithBuffer(armorWriter, &sigBuffer); err != nil {
		return "", err
	}
	_ = armorWriter.Close()
	return armoredSig.String(), nil
}
