package handles

import (
	"path"
	"strings"

	"github.com/alist-org/alist/v3/internal/errs"
	"github.com/alist-org/alist/v3/internal/model"
	"github.com/alist-org/alist/v3/internal/op"
	"github.com/alist-org/alist/v3/internal/search"
	"github.com/alist-org/alist/v3/pkg/utils"
	"github.com/alist-org/alist/v3/server/common"
	"github.com/gin-gonic/gin"
	"github.com/pkg/errors"
)

type SearchReq struct {
	model.SearchReq
	Password string `json:"password"`
}

type SearchResp struct {
	model.SearchNode
	Type int `json:"type"`
}

func Search(c *gin.Context) {
	var (
		req SearchReq
		err error
	)
	if err = c.ShouldBind(&req); err != nil {
		common.ErrorResp(c, err, 400)
		return
	}
	user := c.MustGet("user").(*model.User)
	req.Parent, err = user.JoinPath(req.Parent)
	if err != nil {
		common.ErrorResp(c, err, 400)
		return
	}
	if err := req.Validate(); err != nil {
		common.ErrorResp(c, err, 400)
		return
	}
	var (
		filteredNodes []model.SearchNode
	)
	for len(filteredNodes) < req.PerPage {
		nodes, _, err := search.Search(c, req.SearchReq)
		if err != nil {
			common.ErrorResp(c, err, 500)
			return
		}
		if len(nodes) == 0 {
			break
		}
		for _, node := range nodes {
			if !strings.HasPrefix(node.Parent, user.BasePath) {
				continue
			}
			meta, err := op.GetNearestMeta(node.Parent)
			if err != nil && !errors.Is(errors.Cause(err), errs.MetaNotFound) {
				continue
			}
			if !common.CanAccessWithRoles(user, meta, path.Join(node.Parent, node.Name), req.Password) {
				continue
			}
			filteredNodes = append(filteredNodes, node)
			if len(filteredNodes) >= req.PerPage {
				break
			}
		}
		req.Page++
	}
	common.SuccessResp(c, common.PageResp{
		Content: utils.MustSliceConvert(filteredNodes, nodeToSearchResp),
		Total:   int64(len(filteredNodes)),
	})
}

func nodeToSearchResp(node model.SearchNode) SearchResp {
	return SearchResp{
		SearchNode: node,
		Type:       utils.GetObjType(node.Name, node.IsDir),
	}
}
