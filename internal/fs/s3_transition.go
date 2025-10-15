package fs

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/alist-org/alist/v3/drivers/s3"
	"github.com/alist-org/alist/v3/internal/driver"
	"github.com/alist-org/alist/v3/internal/model"
	"github.com/alist-org/alist/v3/internal/op"
	"github.com/alist-org/alist/v3/internal/task"
	"github.com/pkg/errors"
	"github.com/xhofe/tache"
)

const s3TransitionPollInterval = 15 * time.Second

// S3TransitionTask represents an asynchronous S3 archive/thaw request that is
// tracked via the task manager so that clients can monitor the progress of the
// operation.
type S3TransitionTask struct {
	task.TaskExtension
	status string

	StorageMountPath string          `json:"storage_mount_path"`
	ObjectPath       string          `json:"object_path"`
	DisplayPath      string          `json:"display_path"`
	ObjectName       string          `json:"object_name"`
	Transition       string          `json:"transition"`
	Payload          json.RawMessage `json:"payload,omitempty"`

	TargetStorageClass string `json:"target_storage_class,omitempty"`
	RequestID          string `json:"request_id,omitempty"`
	VersionID          string `json:"version_id,omitempty"`

	storage driver.Driver `json:"-"`
}

// S3TransitionTaskManager holds asynchronous S3 archive/thaw tasks.
var S3TransitionTaskManager *tache.Manager[*S3TransitionTask]

var _ task.TaskExtensionInfo = (*S3TransitionTask)(nil)

func (t *S3TransitionTask) GetName() string {
	action := strings.ToLower(t.Transition)
	if action == "" {
		action = "transition"
	}
	display := t.DisplayPath
	if display == "" {
		display = t.ObjectPath
	}
	if display == "" {
		display = t.ObjectName
	}
	return fmt.Sprintf("s3 %s %s", action, display)
}

func (t *S3TransitionTask) GetStatus() string {
	return t.status
}

func (t *S3TransitionTask) Run() error {
	t.ReinitCtx()
	t.ClearEndTime()
	start := time.Now()
	t.SetStartTime(start)
	defer func() { t.SetEndTime(time.Now()) }()

	if err := t.ensureStorage(); err != nil {
		t.status = fmt.Sprintf("locate storage failed: %v", err)
		return err
	}

	payload, err := t.decodePayload()
	if err != nil {
		t.status = fmt.Sprintf("decode payload failed: %v", err)
		return err
	}

	method := strings.ToLower(strings.TrimSpace(t.Transition))
	switch method {
	case s3.OtherMethodArchive:
		t.status = "submitting archive request"
		t.SetProgress(0)
		resp, err := op.Other(t.Ctx(), t.storage, model.FsOtherArgs{
			Path:   t.ObjectPath,
			Method: s3.OtherMethodArchive,
			Data:   payload,
		})
		if err != nil {
			t.status = fmt.Sprintf("archive request failed: %v", err)
			return err
		}
		archiveResp, ok := toArchiveResponse(resp)
		if ok {
			if t.TargetStorageClass == "" {
				t.TargetStorageClass = archiveResp.StorageClass
			}
			t.RequestID = archiveResp.RequestID
			t.VersionID = archiveResp.VersionID
			if archiveResp.StorageClass != "" {
				t.status = fmt.Sprintf("archive requested, waiting for %s", archiveResp.StorageClass)
			} else {
				t.status = "archive requested"
			}
		} else if sc := t.extractTargetStorageClass(); sc != "" {
			t.TargetStorageClass = sc
			t.status = fmt.Sprintf("archive requested, waiting for %s", sc)
		} else {
			t.status = "archive requested"
		}
		if t.TargetStorageClass != "" {
			t.TargetStorageClass = s3.NormalizeStorageClass(t.TargetStorageClass)
		}
		t.SetProgress(25)
		return t.waitForArchive()
	case s3.OtherMethodThaw:
		t.status = "submitting thaw request"
		t.SetProgress(0)
		resp, err := op.Other(t.Ctx(), t.storage, model.FsOtherArgs{
			Path:   t.ObjectPath,
			Method: s3.OtherMethodThaw,
			Data:   payload,
		})
		if err != nil {
			t.status = fmt.Sprintf("thaw request failed: %v", err)
			return err
		}
		thawResp, ok := toThawResponse(resp)
		if ok {
			t.RequestID = thawResp.RequestID
			if thawResp.Status != nil && !thawResp.Status.Ongoing {
				t.SetProgress(100)
				t.status = thawCompletionMessage(thawResp.Status)
				return nil
			}
		}
		t.status = "thaw requested"
		t.SetProgress(25)
		return t.waitForThaw()
	default:
		return errors.Errorf("unsupported transition method: %s", t.Transition)
	}
}

func (t *S3TransitionTask) ensureStorage() error {
	if t.storage != nil {
		return nil
	}
	storage, err := op.GetStorageByMountPath(t.StorageMountPath)
	if err != nil {
		return err
	}
	t.storage = storage
	return nil
}

func (t *S3TransitionTask) decodePayload() (interface{}, error) {
	if len(t.Payload) == 0 {
		return nil, nil
	}
	var payload interface{}
	if err := json.Unmarshal(t.Payload, &payload); err != nil {
		return nil, err
	}
	return payload, nil
}

func (t *S3TransitionTask) extractTargetStorageClass() string {
	if len(t.Payload) == 0 {
		return ""
	}
	var req s3.ArchiveRequest
	if err := json.Unmarshal(t.Payload, &req); err != nil {
		return ""
	}
	return s3.NormalizeStorageClass(req.StorageClass)
}

func (t *S3TransitionTask) waitForArchive() error {
	ticker := time.NewTicker(s3TransitionPollInterval)
	defer ticker.Stop()

	ctx := t.Ctx()
	for {
		select {
		case <-ctx.Done():
			t.status = "archive canceled"
			return ctx.Err()
		case <-ticker.C:
			resp, err := op.Other(ctx, t.storage, model.FsOtherArgs{
				Path:   t.ObjectPath,
				Method: s3.OtherMethodArchiveStatus,
			})
			if err != nil {
				t.status = fmt.Sprintf("archive status error: %v", err)
				return err
			}
			archiveResp, ok := toArchiveResponse(resp)
			if !ok {
				t.status = fmt.Sprintf("unexpected archive status response: %T", resp)
				return errors.Errorf("unexpected archive status response: %T", resp)
			}
			currentClass := strings.TrimSpace(archiveResp.StorageClass)
			target := strings.TrimSpace(t.TargetStorageClass)
			if target == "" {
				target = currentClass
				t.TargetStorageClass = currentClass
			}
			if currentClass == "" {
				t.status = "waiting for storage class update"
				t.SetProgress(50)
				continue
			}
			if strings.EqualFold(currentClass, target) {
				t.SetProgress(100)
				t.status = fmt.Sprintf("archive complete (%s)", currentClass)
				return nil
			}
			t.status = fmt.Sprintf("storage class %s (target %s)", currentClass, target)
			t.SetProgress(75)
		}
	}
}

func (t *S3TransitionTask) waitForThaw() error {
	ticker := time.NewTicker(s3TransitionPollInterval)
	defer ticker.Stop()

	ctx := t.Ctx()
	for {
		select {
		case <-ctx.Done():
			t.status = "thaw canceled"
			return ctx.Err()
		case <-ticker.C:
			resp, err := op.Other(ctx, t.storage, model.FsOtherArgs{
				Path:   t.ObjectPath,
				Method: s3.OtherMethodThawStatus,
			})
			if err != nil {
				t.status = fmt.Sprintf("thaw status error: %v", err)
				return err
			}
			thawResp, ok := toThawResponse(resp)
			if !ok {
				t.status = fmt.Sprintf("unexpected thaw status response: %T", resp)
				return errors.Errorf("unexpected thaw status response: %T", resp)
			}
			status := thawResp.Status
			if status == nil {
				t.status = "waiting for thaw status"
				t.SetProgress(50)
				continue
			}
			if status.Ongoing {
				t.status = fmt.Sprintf("thaw in progress (%s)", status.Raw)
				t.SetProgress(75)
				continue
			}
			t.SetProgress(100)
			t.status = thawCompletionMessage(status)
			return nil
		}
	}
}

func thawCompletionMessage(status *s3.RestoreStatus) string {
	if status == nil {
		return "thaw complete"
	}
	if status.Expiry != "" {
		return fmt.Sprintf("thaw complete, expires %s", status.Expiry)
	}
	return "thaw complete"
}

func toArchiveResponse(v interface{}) (s3.ArchiveResponse, bool) {
	switch resp := v.(type) {
	case s3.ArchiveResponse:
		return resp, true
	case *s3.ArchiveResponse:
		if resp != nil {
			return *resp, true
		}
	}
	return s3.ArchiveResponse{}, false
}

func toThawResponse(v interface{}) (s3.ThawResponse, bool) {
	switch resp := v.(type) {
	case s3.ThawResponse:
		return resp, true
	case *s3.ThawResponse:
		if resp != nil {
			return *resp, true
		}
	}
	return s3.ThawResponse{}, false
}

// Ensure compatibility with persistence when tasks are restored.
func (t *S3TransitionTask) OnRestore() {
	// The storage handle is not persisted intentionally; it will be lazily
	// re-fetched on the next Run invocation.
	t.storage = nil
}
