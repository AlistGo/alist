package quark

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"testing"

	"github.com/alist-org/alist/v3/internal/conf"
	"github.com/alist-org/alist/v3/internal/db"
	"github.com/alist-org/alist/v3/internal/model"
	"github.com/go-resty/resty/v2"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestMain(m *testing.M) {
	// 初始化内存数据库，保证 op.MustSaveDriverStorage 在测试中安全执行
	conf.Conf = conf.DefaultConfig()
	d, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		panic(err)
	}
	db.Init(d)
	os.Exit(m.Run())
}

var testDriverSeq int

func newTestDriver(srvURL string) *QuarkOrUC {
	testDriverSeq++
	d := &QuarkOrUC{Storage: model.Storage{MountPath: fmt.Sprintf("test-%d", testDriverSeq)}}
	d.conf = Conf{
		ua:      "test-ua",
		referer: "https://pan.quark.cn",
		api:     srvURL + "/1/clouddrive",
		pr:      "ucpro",
	}
	d.client = resty.New()
	return d
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
