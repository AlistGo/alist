# Strm 一键生成（按钮 + 进度）实现计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 给 Strm 驱动加「一键生成 strm」能力：后端做成可追踪进度的后台任务 + admin 接口；前端在文件浏览页工具栏（当前目录）和存储管理页（整库）各加一个按钮，点了显示内联进度条，可重复运行。

**Architecture:** 把 Strm 的全树/子树生成重构为可复用、两阶段（扫描计数→生成报进度）的 `GenerateLocal`，通过 `internal/driver.StrmGenerator` 接口暴露（避免 `internal/fs` import 驱动包成环）。新增 `StrmGenerateTask` + 任务管理器（tache），admin 接口入队任务并返回 task id，前端轮询该任务进度渲染进度条。覆盖模式用存储配置的 `SaveLocalMode`。

**Tech Stack:** 后端 Go（tache 任务系统、gin）；前端 SolidJS + @hope-ui/solid + Vite + pnpm。

**分支：**
- 后端 alist：已在 `feat/strm-generate-button`（spec 已提交于此）。
- 前端 alist-web：从 origin/main 切 `feat/strm-generate-button`（见 Task 4 Step 1）。

---

## Task 1: Strm 驱动 `GenerateLocal` + `StrmGenerator` 接口（后端）

**Files:**
- Create: `internal/driver/strm.go`
- Modify: `drivers/strm/driver.go`（加 GenerateLocal/resolveStarts/collectUnits/generateUnits、断言实现接口）
- Modify: `drivers/strm/util.go`（把 `SaveStrmToLocal` 守卫从 `syncLocalDirWithMode` 移到 `syncLocalDir`）
- Test: `drivers/strm/generate_test.go`

- [ ] **Step 1: 定义接口 `internal/driver/strm.go`**

```go
package driver

import "context"

// StrmGenerator is implemented by the Strm driver to (re)generate local .strm
// files for a subtree, reporting progress (0-100) via up.
type StrmGenerator interface {
	// GenerateLocal walks virtualPath (relative to the storage root, e.g. "/" or
	// "/Movies") and writes local files, reporting progress in percent.
	GenerateLocal(ctx context.Context, virtualPath string, up func(percent float64)) error
}
```

- [ ] **Step 2: 写失败测试 `drivers/strm/generate_test.go`**

```go
package strm

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/alist-org/alist/v3/internal/model"
)

func TestResolveStarts(t *testing.T) {
	// flatten (single alias)
	d := &Strm{}
	d.aliases = map[string][]string{"movies": {"/local/movies"}}
	d.autoFlatten = true
	d.singleRootKey = "movies"
	starts := d.resolveStarts("/")
	if len(starts) != 1 || starts[0].virtualDir != "/" || starts[0].realDir != "/local/movies" {
		t.Fatalf("flatten root got %+v", starts)
	}

	// non-flatten (multi alias)
	d2 := &Strm{}
	d2.aliases = map[string][]string{"a": {"/ra"}, "b": {"/rb"}}
	d2.autoFlatten = false
	rootStarts := d2.resolveStarts("/")
	if len(rootStarts) != 2 {
		t.Fatalf("non-flatten root want 2 got %d", len(rootStarts))
	}
	sub := d2.resolveStarts("/a/sub")
	if len(sub) != 1 || sub[0].virtualDir != "/a/sub" || sub[0].realDir != "/ra/sub" {
		t.Fatalf("non-flatten sub got %+v", sub)
	}
}

func TestGenerateUnitsWritesAndProgress(t *testing.T) {
	tmp := t.TempDir()
	d := &Strm{}
	d.SaveStrmLocalPath = tmp
	d.EncodePath = true
	d.WithoutUrl = true // avoid needing a SiteUrl / network
	d.normalizedPrefix = "/d"

	units := []strmDirUnit{
		{
			virtualDir: "/Movies",
			objs: []model.Obj{
				&model.Object{ID: "strm", Path: "/real/Movies/m.mkv", Name: "m.strm"},
			},
		},
	}

	var last float64
	d.generateUnits(context.Background(), units, SaveLocalUpdateMode, func(p float64) { last = p })

	if last != 100 {
		t.Fatalf("progress want 100 got %v", last)
	}
	out := filepath.Join(tmp, "Movies", "m.strm")
	b, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("strm not written: %v", err)
	}
	if len(b) == 0 {
		t.Fatal("strm file empty")
	}
}
```

- [ ] **Step 3: 运行，确认失败**

Run: `cd /Users/yuxiaofan/GolandProjects/alist && go test ./drivers/strm/ -run 'TestResolveStarts|TestGenerateUnits' -v`
Expected: 编译失败（`resolveStarts`/`strmDirUnit`/`generateUnits` 未定义）。

- [ ] **Step 4: 在 `drivers/strm/driver.go` 增加实现**

在文件末尾（`var _ driver.Driver = (*Strm)(nil)` 之前或之后）追加：

```go
type strmDirUnit struct {
	virtualDir string
	objs       []model.Obj
}

type strmDirStart struct {
	virtualDir string
	realDir    string
}

// resolveStarts maps a strm-internal virtual path to the (virtualDir, realDir)
// walk start points. virtualPath "/" expands to all aliases.
func (d *Strm) resolveStarts(virtualPath string) []strmDirStart {
	virtualPath = cleanPath(virtualPath)
	var starts []strmDirStart
	if virtualPath == "/" {
		for alias, roots := range d.aliases {
			vroot := "/"
			if !d.autoFlatten {
				vroot = "/" + alias
			}
			for _, r := range roots {
				starts = append(starts, strmDirStart{virtualDir: vroot, realDir: r})
			}
		}
		return starts
	}
	root, sub := d.splitVirtualPath(virtualPath)
	roots, ok := d.aliases[root]
	if !ok {
		return nil
	}
	for _, r := range roots {
		starts = append(starts, strmDirStart{virtualDir: virtualPath, realDir: stdpath.Join(r, sub)})
	}
	return starts
}

// collectUnits recursively lists realDir and gathers per-directory mapped objs.
func (d *Strm) collectUnits(ctx context.Context, virtualDir, realDir string, units *[]strmDirUnit) {
	objs, err := fs.List(ctx, realDir, &fs.ListArgs{NoLog: true, Refresh: true})
	if err != nil {
		log.Warnf("strm: generate list failed %s: %v", realDir, err)
		return
	}
	mapped := d.mapListedObjects(ctx, realDir, objs)
	*units = append(*units, strmDirUnit{virtualDir: virtualDir, objs: mapped})
	for _, obj := range objs {
		if obj.IsDir() {
			d.collectUnits(ctx, stdpath.Join(virtualDir, obj.GetName()), stdpath.Join(realDir, obj.GetName()), units)
		}
	}
}

// generateUnits writes local files for pre-collected units and reports progress.
func (d *Strm) generateUnits(ctx context.Context, units []strmDirUnit, mode string, up func(percent float64)) {
	total := 0
	for _, u := range units {
		for _, o := range u.objs {
			if !o.IsDir() {
				total++
			}
		}
	}
	if total == 0 {
		if up != nil {
			up(100)
		}
		return
	}
	done := 0
	for _, u := range units {
		d.syncLocalDirWithMode(ctx, u.virtualDir, u.objs, mode)
		for _, o := range u.objs {
			if !o.IsDir() {
				done++
			}
		}
		if up != nil {
			up(float64(done) / float64(total) * 100)
		}
	}
}

// GenerateLocal implements driver.StrmGenerator.
func (d *Strm) GenerateLocal(ctx context.Context, virtualPath string, up func(percent float64)) error {
	if strings.TrimSpace(d.SaveStrmLocalPath) == "" {
		return errors.New("SaveStrmLocalPath is required")
	}
	starts := d.resolveStarts(virtualPath)
	if len(starts) == 0 {
		return errs.ObjectNotFound
	}
	var units []strmDirUnit
	for _, s := range starts {
		d.collectUnits(ctx, s.virtualDir, s.realDir, &units)
	}
	d.generateUnits(ctx, units, d.normalizedMode, up)
	return nil
}

var _ driver.StrmGenerator = (*Strm)(nil)
```

(`stdpath`, `strings`, `errors`, `fs`, `errs`, `log`, `model`, `driver` 均已在 driver.go 的 import 中。)

- [ ] **Step 5: 把 `SaveStrmToLocal` 守卫移到惰性入口（`drivers/strm/util.go`）**

当前 `syncLocalDirWithMode` 开头是：
```go
	if !d.SaveStrmToLocal || strings.TrimSpace(d.SaveStrmLocalPath) == "" {
		return
	}
```
改为只保留本地路径校验：
```go
	if strings.TrimSpace(d.SaveStrmLocalPath) == "" {
		return
	}
```
并把 `SaveStrmToLocal` 开关移动到惰性包装 `syncLocalDir`：
```go
func (d *Strm) syncLocalDir(ctx context.Context, virtualDir string, objs []model.Obj) {
	if !d.SaveStrmToLocal {
		return
	}
	d.syncLocalDirWithMode(ctx, virtualDir, objs, d.normalizedMode)
}
```
这样：浏览触发的 `List`→`syncLocalDir` 仍遵守开关；而手动 `GenerateLocal`/`rotateAllLocal` 直接调 `syncLocalDirWithMode`，只要本地路径有值就执行（手动是显式动作）。

- [ ] **Step 6: 运行测试，确认通过**

Run: `cd /Users/yuxiaofan/GolandProjects/alist && go test ./drivers/strm/ -run 'TestResolveStarts|TestGenerateUnits' -v`
Expected: 2 个用例 PASS。

- [ ] **Step 7: 构建 + vet + 提交**

```bash
cd /Users/yuxiaofan/GolandProjects/alist
go build ./... && go vet ./drivers/strm/ ./internal/driver/
git add internal/driver/strm.go drivers/strm/driver.go drivers/strm/util.go drivers/strm/generate_test.go
git commit -m "feat(strm): add GenerateLocal (two-phase, progress) + StrmGenerator interface"
```
不要加任何 Claude/AI 署名。

---

## Task 2: `StrmGenerateTask` + 管理器 + 路由（后端）

**Files:**
- Create: `internal/fs/strm_generate.go`
- Modify: `internal/bootstrap/task.go`
- Modify: `server/handles/task.go`（在 `SetupTaskRoute` 注册）

- [ ] **Step 1: 创建任务 `internal/fs/strm_generate.go`**

```go
package fs

import (
	"fmt"
	"time"

	"github.com/alist-org/alist/v3/internal/driver"
	"github.com/alist-org/alist/v3/internal/op"
	"github.com/alist-org/alist/v3/internal/task"
	"github.com/pkg/errors"
	"github.com/xhofe/tache"
)

type StrmGenerateTask struct {
	task.TaskExtension
	StorageMountPath string `json:"storage_mount_path"`
	Path             string `json:"path"` // actual path relative to the storage root
}

func (t *StrmGenerateTask) GetName() string {
	return fmt.Sprintf("generate strm [%s](%s)", t.StorageMountPath, t.Path)
}

func (t *StrmGenerateTask) GetStatus() string {
	return "generating strm"
}

func (t *StrmGenerateTask) Run() error {
	t.ClearEndTime()
	t.SetStartTime(time.Now())
	defer func() { t.SetEndTime(time.Now()) }()
	storage, err := op.GetStorageByMountPath(t.StorageMountPath)
	if err != nil {
		return errors.WithMessage(err, "failed get storage")
	}
	gen, ok := storage.(driver.StrmGenerator)
	if !ok {
		return errors.New("not a strm storage")
	}
	return gen.GenerateLocal(t.Ctx(), t.Path, t.SetProgress)
}

var StrmGenerateTaskManager *tache.Manager[*StrmGenerateTask]
```

- [ ] **Step 2: 注册管理器 `internal/bootstrap/task.go`**

在 `InitTaskManager()` 里，`fs.UploadTaskManager = ...` 那一行之后加：
```go
	fs.StrmGenerateTaskManager = tache.NewManager[*fs.StrmGenerateTask](tache.WithWorks(3), tache.WithMaxRetry(0)) // strm generate, not persisted
```
（`tache` 已在该文件 import。worker 固定 3，避免对源存储 List 压力过大。）

- [ ] **Step 3: 注册任务路由 `server/handles/task.go`**

在 `SetupTaskRoute` 里，`taskRoute(g.Group("/upload"), fs.UploadTaskManager)` 之后加一行：
```go
	taskRoute(g.Group("/strm_generate"), fs.StrmGenerateTaskManager)
```

- [ ] **Step 4: 构建 + 提交**

```bash
cd /Users/yuxiaofan/GolandProjects/alist
go build ./...
git add internal/fs/strm_generate.go internal/bootstrap/task.go server/handles/task.go
git commit -m "feat(strm): add StrmGenerateTask, manager and task route"
```
不要加 Claude/AI 署名。

---

## Task 3: admin 接口 `POST /admin/strm/generate`（后端）

**Files:**
- Create: `server/handles/strm.go`
- Modify: `server/router.go`

- [ ] **Step 1: 创建 handler `server/handles/strm.go`**

```go
package handles

import (
	"github.com/alist-org/alist/v3/internal/driver"
	"github.com/alist-org/alist/v3/internal/fs"
	"github.com/alist-org/alist/v3/internal/model"
	"github.com/alist-org/alist/v3/internal/op"
	"github.com/alist-org/alist/v3/internal/task"
	"github.com/alist-org/alist/v3/server/common"
	"github.com/gin-gonic/gin"
)

type GenerateStrmReq struct {
	Path string `json:"path"`
}

// GenerateStrm enqueues a strm generation task for the given path (which must be
// inside a Strm storage). Admin only.
func GenerateStrm(c *gin.Context) {
	var req GenerateStrmReq
	if err := c.ShouldBind(&req); err != nil {
		common.ErrorResp(c, err, 400)
		return
	}
	user := c.MustGet("user").(*model.User)
	reqPath, err := user.JoinPath(req.Path)
	if err != nil {
		common.ErrorResp(c, err, 403)
		return
	}
	storage, actualPath, err := op.GetStorageAndActualPath(reqPath)
	if err != nil {
		common.ErrorResp(c, err, 400)
		return
	}
	if _, ok := storage.(driver.StrmGenerator); !ok {
		common.ErrorStrResp(c, "not a strm storage", 400)
		return
	}
	t := &fs.StrmGenerateTask{
		TaskExtension:    task.TaskExtension{Creator: user},
		StorageMountPath: storage.GetStorage().MountPath,
		Path:             actualPath,
	}
	fs.StrmGenerateTaskManager.Add(t)
	common.SuccessResp(c, gin.H{"task": getTaskInfo(t)})
}
```

(`getTaskInfo` 在同包 `server/handles/task.go` 中，可直接用。)

- [ ] **Step 2: 注册路由 `server/router.go`**

在 admin 分组（`g.Group(...)` 下，与 `storage`/`task` 同级）添加。找到 `_task(g.Group("/task"))` 附近的 admin 路由区，加：
```go
	g.POST("/strm/generate", handles.GenerateStrm)
```
（`g` 是 `auth.Group("/admin", ...)` 这类已鉴权的 admin 组；确保放在 admin 组内，与 `storage.POST(...)` 同一鉴权层级。实现时按该文件已有 admin 组变量名放置。）

- [ ] **Step 3: 构建 + 提交**

```bash
cd /Users/yuxiaofan/GolandProjects/alist
go build ./...
git add server/handles/strm.go server/router.go
git commit -m "feat(strm): add admin POST /strm/generate endpoint"
```

- [ ] **Step 4: 后端整体验收**

Run: `cd /Users/yuxiaofan/GolandProjects/alist && go build ./... && go vet ./drivers/strm/ ./internal/fs/ ./server/handles/ && go test ./drivers/strm/`
Expected: 全部通过。

---

## Task 4: 前端 i18n + 分支（alist-web）

**Files:**
- Modify: `src/lang/en/global.json`（或合适的命名空间）

- [ ] **Step 1: 切分支**

```bash
cd /Users/yuxiaofan/WebstormProjects/alist-web
git checkout -b feat/strm-generate-button origin/main
```

- [ ] **Step 2: 加文案**

在 `src/lang/en/global.json` 顶层对象内加入以下键（放在任意合法位置，保持 JSON 合法）：
```json
  "generate_strm": "Generate Strm",
  "generate_strm_start": "Generation started",
  "generate_strm_progress": "Generating Strm...",
  "generate_strm_done": "Strm generated",
  "generate_strm_failed": "Generation failed",
```

- [ ] **Step 3: 校验 JSON + 提交**

Run: `cd /Users/yuxiaofan/WebstormProjects/alist-web && node -e "JSON.parse(require('fs').readFileSync('src/lang/en/global.json','utf8'));console.log('JSON OK')"`
Expected: `JSON OK`
```bash
git add src/lang/en/global.json
git commit -m "i18n: add strm generate strings"
```

---

## Task 5: 前端共享生成逻辑（enqueue + 轮询进度）（alist-web）

**Files:**
- Create: `src/pages/manage/storages/StrmGenerate.tsx`

- [ ] **Step 1: 创建组件（按钮 + 进度弹窗）**

```tsx
import {
  Button,
  Modal,
  ModalBody,
  ModalContent,
  ModalFooter,
  ModalHeader,
  ModalOverlay,
  Progress,
  ProgressIndicator,
  Text,
  VStack,
} from "@hope-ui/solid"
import { createSignal, onCleanup, Show } from "solid-js"
import { useT } from "~/hooks"
import { r, notify, handleResp } from "~/utils"

export type StrmGenerateButtonProps = {
  path: string
  size?: "xs" | "sm" | "md" | "lg"
  variant?: string
  colorScheme?: string
}

type TaskInfo = { id: string; progress: number; state: number; error: string }

export const StrmGenerateButton = (props: StrmGenerateButtonProps) => {
  const t = useT()
  const [opened, setOpened] = createSignal(false)
  const [progress, setProgress] = createSignal(0)
  const [running, setRunning] = createSignal(false)
  const [err, setErr] = createSignal("")
  let timer: ReturnType<typeof setInterval> | undefined

  const stop = () => {
    if (timer) {
      clearInterval(timer)
      timer = undefined
    }
  }
  onCleanup(stop)

  const poll = (tid: string) => {
    stop()
    timer = setInterval(async () => {
      const resp = await r.post(`/admin/task/strm_generate/info?tid=${tid}`)
      handleResp(resp, (info: TaskInfo) => {
        if (info.error) {
          stop()
          setRunning(false)
          setErr(info.error)
          return
        }
        setProgress(Math.floor(info.progress))
        if (info.progress >= 100) {
          stop()
          setRunning(false)
          notify.success(t("global.generate_strm_done"))
        }
      })
    }, 1000)
  }

  const start = async () => {
    setOpened(true)
    setRunning(true)
    setProgress(0)
    setErr("")
    const resp = await r.post("/admin/strm/generate", { path: props.path })
    handleResp(resp, (data: { task: TaskInfo }) => {
      notify.info(t("global.generate_strm_start"))
      poll(data.task.id)
    })
    if ((resp as any).code !== 200) {
      setRunning(false)
    }
  }

  const close = () => {
    stop()
    setOpened(false)
  }

  return (
    <>
      <Button
        size={props.size ?? "sm"}
        colorScheme={(props.colorScheme as any) ?? "accent"}
        onClick={start}
      >
        {t("global.generate_strm")}
      </Button>
      <Modal opened={opened()} onClose={close} blockScrollOnMount={false}>
        <ModalOverlay />
        <ModalContent>
          <ModalHeader>{t("global.generate_strm")}</ModalHeader>
          <ModalBody>
            <VStack spacing="$3" alignItems="stretch" py="$2">
              <Progress value={progress()} trackColor="$neutral4">
                <ProgressIndicator color="$success9" />
              </Progress>
              <Text>
                <Show
                  when={!err()}
                  fallback={t("global.generate_strm_failed") + ": " + err()}
                >
                  {running()
                    ? t("global.generate_strm_progress") + " " + progress() + "%"
                    : t("global.generate_strm_done")}
                </Show>
              </Text>
            </VStack>
          </ModalBody>
          <ModalFooter>
            <Button colorScheme="neutral" onClick={close}>
              {t("global.close")}
            </Button>
          </ModalFooter>
        </ModalContent>
      </Modal>
    </>
  )
}
```

(`r`、`notify`、`handleResp` 均从 `~/utils` 导出，见现有用法；`global.close` 已存在于 i18n。`Progress`/`ProgressIndicator` 为 @hope-ui/solid 组件。)

- [ ] **Step 2: 构建**

Run: `cd /Users/yuxiaofan/WebstormProjects/alist-web && pnpm build 2>&1 | tail -12`
Expected: 成功（组件暂未被引用也应编译通过）。若 `Progress`/`ProgressIndicator`/`handleResp` 导入名与本版本不符，按编译错误修正为本仓库实际导出（不要改行为）。

- [ ] **Step 3: 提交**

```bash
cd /Users/yuxiaofan/WebstormProjects/alist-web
git add src/pages/manage/storages/StrmGenerate.tsx
git commit -m "feat(strm): add StrmGenerate button + progress component"
```

---

## Task 6: 文件浏览页工具栏按钮（当前目录，Strm + admin）（alist-web）

**Files:**
- Create: `src/pages/home/toolbar/StrmGenerate.tsx`
- Modify: `src/pages/home/toolbar/Right.tsx`

- [ ] **Step 1: 工具栏包装组件 `src/pages/home/toolbar/StrmGenerate.tsx`**

```tsx
import { Show } from "solid-js"
import { useRouter } from "~/hooks"
import { me } from "~/store"
import { objStore } from "~/store"
import { StrmGenerateButton } from "~/pages/manage/storages/StrmGenerate"

export const ToolbarStrmGenerate = () => {
  const { pathname } = useRouter()
  const isAdmin = () => (me().role || []).includes(2)
  const isStrm = () => objStore.provider === "Strm"
  return (
    <Show when={isAdmin() && isStrm()}>
      <StrmGenerateButton path={pathname()} />
    </Show>
  )
}
```

(`me` 与 `objStore` 从 `~/store` 导出；admin 判定与 `FolderTree.tsx` 一致：`me().role.includes(2)`。`objStore.provider` 是当前目录所属存储驱动名。)

- [ ] **Step 2: 接入 `Right.tsx`**

在 `src/pages/home/toolbar/Right.tsx` 顶部 import 区加：
```tsx
import { ToolbarStrmGenerate } from "./StrmGenerate"
```
在工具栏按钮区（例如 `<Show when={isFolder() && userCan("offline_download")}>...</Show>` 之后）插入：
```tsx
            <ToolbarStrmGenerate />
```
（放在现有按钮序列里即可；它自身已做 admin + Strm 条件渲染。）

- [ ] **Step 3: 构建 + 提交**

Run: `cd /Users/yuxiaofan/WebstormProjects/alist-web && pnpm build 2>&1 | tail -8`
Expected: 成功。
```bash
git add src/pages/home/toolbar/StrmGenerate.tsx src/pages/home/toolbar/Right.tsx
git commit -m "feat(strm): add generate button to file browser toolbar"
```

---

## Task 7: 存储管理页按钮（整库，Strm + admin）（alist-web）

**Files:**
- Modify: `src/pages/manage/storages/Storages.tsx`

- [ ] **Step 1: 在存储行操作区加按钮**

在 `src/pages/manage/storages/Storages.tsx` 顶部 import 区加：
```tsx
import { StrmGenerateButton } from "./StrmGenerate"
```
在每行「操作」单元格（编辑/删除按钮所在的 `HStack`/`Td`）内，加一个仅对 Strm 存储显示的按钮（`storage` 为该行对象，含 `driver` 和 `mount_path`）：
```tsx
                  <Show when={storage.driver === "Strm"}>
                    <StrmGenerateButton path={storage.mount_path} size="sm" />
                  </Show>
```
（`Show` 已在该文件 import；若未 import 则从 `solid-js` 引入。`storage.mount_path` 即整库根路径。）

- [ ] **Step 2: 构建 + 提交**

Run: `cd /Users/yuxiaofan/WebstormProjects/alist-web && pnpm build 2>&1 | tail -8`
Expected: 成功。
```bash
git add src/pages/manage/storages/Storages.tsx
git commit -m "feat(strm): add whole-storage generate button to storages page"
```

- [ ] **Step 3: 手动验收（人工）**

前置：alist 后端跑 `feat/strm-generate-button`，前端 `pnpm dev`，已配置一个 Strm 存储且填了 `SaveStrmLocalPath`。
1. 管理员进入该 Strm 挂载目录 → 工具栏出现「Generate Strm」；非 Strm 目录不出现；非管理员不出现。
2. 点击 → 弹窗进度条从 0 增长到 100% → 提示完成 → 本地 `SaveStrmLocalPath` 下出现对应 `.strm`。
3. 存储管理页该 Strm 行有「Generate Strm」按钮 → 点击整库生成、进度条到 100%。
4. 再点一次（重新生成）→ 按存储 `SaveLocalMode` 行为（insert 跳过已存在 / update 覆盖变化 / sync 还删多余）。
5. `SaveStrmLocalPath` 未配置时 → 任务失败、弹窗显示错误。

---

## 验收
- [ ] 后端：`cd /Users/yuxiaofan/GolandProjects/alist && go build ./... && go test ./drivers/strm/ && go vet ./drivers/strm/ ./internal/fs/ ./server/handles/`
- [ ] 前端：`cd /Users/yuxiaofan/WebstormProjects/alist-web && pnpm build`
- [ ] 手动 e2e（Task 7 Step 3 全过）。

## 范围外
- 不在按钮处选覆盖模式（用存储配置）。
- 非 Strm 驱动不提供按钮；非管理员不可见/不可用。
- 不改 strm 内容/签名格式。
