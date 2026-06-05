# 设计文档：Strm 一键生成（带进度、可重新生成）

日期：2026-06-05
状态：已与用户评审通过，待写实现 plan
范围：alist 后端（`drivers/strm` + 任务 + 接口）+ alist-web 前端（两处按钮 + 内联进度）。

## 背景

Strm 驱动把其它存储的路径映射出来，把媒体文件以 `.strm` 暴露，并可把生成的 strm 写到本地磁盘（`SaveStrmToLocal` / `SaveStrmLocalPath` / `SaveLocalMode` ∈ insert/update/sync）。

现状：
- 生成是**惰性**的——浏览某目录时 `List` 调 `syncLocalDir` 才写该目录的 strm。
- 有一个**全树递归**生成（`rotateAllLocal`→`walkAndSync`→`syncLocalDirWithMode`），但目前只在保存存储时由 `RotateSignNow` 标志触发，fire-and-forget goroutine，**无进度、无任务、不能当按钮用**。

目标：把全树/子树生成做成**显式、可追踪进度、可重复运行**的动作，前端加按钮触发。

## 决策（已与用户确认）
- **范围**：整库（挂载根）+ 任意子目录，两者都要。
- **按钮位置**：文件浏览页工具栏（当前目录）+ 存储管理页（整库），两处都加。
- **进度**：复用 alist 任务系统（tache）驱动；前端在按钮处**内联**展示该任务进度条（任务也出现在「任务」面板）。
- **覆盖模式**：用存储已配置的 `SaveLocalMode`（不在按钮处选）。
- **权限**：接口与按钮**仅管理员**（写服务器本地磁盘）。
- **前置**：`SaveStrmLocalPath` 必须已配置；即使 `SaveStrmToLocal` 开关关闭，手动生成仍执行（显式动作）。

## 后端（alist）

### 1. 驱动改造（`drivers/strm`，DRY）
把现有 `walkAndSync` 重构为可复用、可带进度、可指定起点与 mode 的两阶段实现：

- 新增导出方法（驱动内，供任务调用）：
  ```go
  // GenerateLocal walks the given alist virtual path (must be inside this Strm
  // mount) and writes .strm/local files, reporting progress via up.
  func (d *Strm) GenerateLocal(ctx context.Context, virtualPath string, up func(percent float64)) error
  ```
- 内部两阶段：
  - **阶段1 扫描**：从 `virtualPath` 递归 `fs.List`，把所有应生成的条目（经 `mapListedObjects` 得到的媒体 `.strm` / 下载类文件，连同其 virtualDir）收集进切片 `tasks`，得到总数 N。期间 `up` 可报告不确定进度（如固定 0 或缓慢增长）。
  - **阶段2 生成**：遍历 `tasks`，对每个目录调用一次/逐文件 `writeLocal(localPath, localPayload(...), d.normalizedMode)`，每完成一个 `up(done/N*100)`。
- `rotateAllLocal`（`RotateSignNow` 路径）改为复用同一 walk（mode 仍用 update，可传参），消除重复逻辑。
- 校验：`SaveStrmLocalPath` 为空时返回明确错误。

> 说明：扫描与生成共用同一次 `fs.List` 结果（扫描阶段把条目缓存进切片），不重复列目录。

### 2. 任务（`internal/fs/strm_generate.go` 新文件）
```go
type StrmGenerateTask struct {
    task.TaskExtension
    StorageMountPath string `json:"storage_mount_path"`
    Path             string `json:"path"`   // alist virtual path inside the Strm mount
    status           string
}
func (t *StrmGenerateTask) GetName() string  // e.g. "generate strm [mount][path]"
func (t *StrmGenerateTask) GetStatus() string
func (t *StrmGenerateTask) Run() error       // resolve Strm driver by mount, call GenerateLocal with t.SetProgress
var StrmGenerateTaskManager *tache.Manager[*StrmGenerateTask]
```
- 为**避免 import 环**（`internal/fs` 不应 import 具体驱动包 `drivers/strm`），在能被双方引用的低层包（`internal/driver` 或 `internal/model`）定义接口：
  ```go
  type StrmGenerator interface {
      GenerateLocal(ctx context.Context, virtualPath string, up func(percent float64)) error
  }
  ```
  Strm 驱动实现该接口。`Run()` 用 `op.GetStorageByMountPath(StorageMountPath)` 拿驱动，类型断言 `driver.(StrmGenerator)`（否则报错「not a strm storage」），调用之。这样任务可放 `internal/fs`、不引入驱动包、无环。
- 在 `internal/bootstrap/task.go` 注册 `StrmGenerateTaskManager`（不持久化，仿 `UploadTaskManager`；worker 数默认 1～3）。

### 3. 接口（`server/handles` + `server/router.go`）
- `POST /api/admin/strm/generate`，body：
  ```json
  { "path": "/<strm-mount>/<sub/dir>" }
  ```
- 处理：
  1. `op.GetStorageAndActualPath(path)` 解析存储；断言驱动为 Strm，否则 400。
  2. 校验 `SaveStrmLocalPath` 非空，否则返回明确错误。
  3. 构造 `StrmGenerateTask{StorageMountPath, Path}`，`StrmGenerateTaskManager.Add(t)`。
  4. 返回 `{ "task": <task info> }`（含 task id，供前端轮询）。
- 路由放入 admin 组（仅管理员）。整库入口传挂载根 path；子目录入口传该目录 path。

## 前端（alist-web）

### A. 文件浏览页工具栏按钮（当前目录）
- 仅当：当前用户是管理员 **且** 当前所在存储驱动为 Strm（可由现有 obj/store 的 provider 字段判断；若前端无该信息，则按钮常显但后端非 Strm 返回 400 时提示）。
- 点击 → `POST /api/admin/strm/generate { path: 当前路径 }` → 拿 task id → 打开一个带**进度条**的弹窗，轮询任务进度接口（`/api/admin/task/<group>/info?tid=` 或现有任务查询）更新进度，完成/失败提示。

### B. 存储管理页按钮（整库）
- Strm 存储行/编辑页加「生成 Strm」按钮 → `POST .../generate { path: 挂载根 }` → 同样的内联进度弹窗。

### 进度展示
- 复用任务系统：前端用返回的 task id 轮询该任务的进度（progress 0–100）与状态，渲染内联进度条；同一任务也在「任务」面板可见。
- i18n：仅改 `src/lang/en`（中文走 Crowdin）。

## 错误处理
- `SaveStrmLocalPath` 未配置 / path 非 Strm 驱动 → 接口 4xx + 明确消息，前端 toast。
- 子目录 `fs.List` 失败 → 记日志跳过，不中断（与现有 `walkAndSync` 行为一致）。
- 任务失败 → 任务系统标记失败，前端进度弹窗显示错误。

## 测试
- 后端：
  - 单测 `GenerateLocal` 两阶段：用本地/内存源存储构造 Strm，验证扫描计数、按 mode 写文件、进度回调被调用到 100。
  - `go build ./...`、`go vet ./drivers/strm/ ./internal/fs/`。
- 前端：`pnpm build`；手动验证按钮出现条件、进度条更新、重新生成、错误提示。

## 明确不做（YAGNI）
- 不在按钮处做 mode 选择（用存储配置）。
- 不做非管理员入口。
- 不改 strm 内容格式 / 签名逻辑（沿用现有 `buildStrmLine`/`generateSign`）。
- 不为非 Strm 驱动提供该按钮。

## 实现注意
- 任务放 `internal/fs`，通过 `StrmGenerator` 接口（定义在 `internal/driver` 或 `internal/model`）+ 类型断言访问驱动，避免 import 环。
- worker 并发：默认小（1–3），避免对源存储 List 压力过大。
