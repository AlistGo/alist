package handles

import (
	"strings"

	"github.com/alist-org/alist/v3/internal/conf"
	"github.com/gin-gonic/gin"
)

func GetSharePage(c *gin.Context) {
	c.Header("Content-Type", "text/html")
	c.Status(200)
	_, _ = c.Writer.WriteString(strings.Replace(conf.IndexHtml, "<body>", "<body data-page=\"share\">", 1))
	c.Writer.Flush()
	c.Writer.WriteHeaderNow()
}
