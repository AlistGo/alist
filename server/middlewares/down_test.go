package middlewares

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestDownloadPathDecodedOnce(t *testing.T) {
	router := gin.New()
	router.GET("/d/*path", func(c *gin.Context) {
		c.String(http.StatusOK, parsePath(c.Param("path")))
	})
	for _, path := range []string{
		"/教学/九上/验证.txt",
		"/教学/单元 #1%/课程/验证.txt",
		"/教学/literal%2Fname.txt",
		"/教学/literal%252Fname.txt",
		"/教学/a+b.txt",
	} {
		t.Run(path, func(t *testing.T) {
			u := url.URL{Path: "/d" + path}
			response := httptest.NewRecorder()
			router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, u.String(), nil))
			if response.Code != http.StatusOK || response.Body.String() != path {
				t.Fatalf("download path: status=%d body=%q, want %q", response.Code, response.Body.String(), path)
			}
		})
	}
}
