package s3

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/alist-org/alist/v3/internal/errs"
	"github.com/alist-org/alist/v3/internal/model"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/s3"
)

type archiveRequest struct {
	StorageClass string `json:"storage_class"`
}

type thawRequest struct {
	Days int64  `json:"days"`
	Tier string `json:"tier"`
}

type objectDescriptor struct {
	Path   string `json:"path"`
	Bucket string `json:"bucket"`
	Key    string `json:"key"`
}

type archiveResponse struct {
	Action       string           `json:"action"`
	Object       objectDescriptor `json:"object"`
	StorageClass string           `json:"storage_class"`
	RequestID    string           `json:"request_id,omitempty"`
	VersionID    string           `json:"version_id,omitempty"`
	ETag         string           `json:"etag,omitempty"`
	LastModified string           `json:"last_modified,omitempty"`
}

type thawResponse struct {
	Action    string           `json:"action"`
	Object    objectDescriptor `json:"object"`
	RequestID string           `json:"request_id,omitempty"`
	Status    *restoreStatus   `json:"status,omitempty"`
}

type restoreStatus struct {
	Ongoing bool   `json:"ongoing"`
	Expiry  string `json:"expiry,omitempty"`
	Raw     string `json:"raw"`
}

func (d *S3) Other(ctx context.Context, args model.OtherArgs) (interface{}, error) {
	if args.Obj == nil {
		return nil, fmt.Errorf("missing object reference")
	}
	if args.Obj.IsDir() {
		return nil, errs.NotSupport
	}

	switch strings.ToLower(strings.TrimSpace(args.Method)) {
	case "archive":
		return d.archive(ctx, args)
	case "archive_status":
		return d.archiveStatus(ctx, args)
	case "thaw":
		return d.thaw(ctx, args)
	case "thaw_status":
		return d.thawStatus(ctx, args)
	default:
		return nil, errs.NotSupport
	}
}

func (d *S3) archive(ctx context.Context, args model.OtherArgs) (interface{}, error) {
	key := getKey(args.Obj.GetPath(), false)
	payload := archiveRequest{}
	if err := decodeOtherArgs(args.Data, &payload); err != nil {
		return nil, fmt.Errorf("parse archive request: %w", err)
	}
	if payload.StorageClass == "" {
		return nil, fmt.Errorf("storage_class is required")
	}
	storageClass := normalizeStorageClass(payload.StorageClass)
	input := &s3.CopyObjectInput{
		Bucket:            &d.Bucket,
		Key:               &key,
		CopySource:        aws.String(url.PathEscape(d.Bucket + "/" + key)),
		MetadataDirective: aws.String(s3.MetadataDirectiveCopy),
		StorageClass:      aws.String(storageClass),
	}
	copyReq, output := d.client.CopyObjectRequest(input)
	copyReq.SetContext(ctx)
	if err := copyReq.Send(); err != nil {
		return nil, err
	}

	resp := archiveResponse{
		Action:       "archive",
		Object:       d.describeObject(args.Obj, key),
		StorageClass: storageClass,
		RequestID:    copyReq.RequestID,
	}
	if output.VersionId != nil {
		resp.VersionID = aws.StringValue(output.VersionId)
	}
	if result := output.CopyObjectResult; result != nil {
		resp.ETag = aws.StringValue(result.ETag)
		if result.LastModified != nil {
			resp.LastModified = result.LastModified.UTC().Format(time.RFC3339)
		}
	}
	if status, err := d.describeObjectStatus(ctx, key); err == nil {
		if status.StorageClass != "" {
			resp.StorageClass = status.StorageClass
		}
	}
	return resp, nil
}

func (d *S3) archiveStatus(ctx context.Context, args model.OtherArgs) (interface{}, error) {
	key := getKey(args.Obj.GetPath(), false)
	status, err := d.describeObjectStatus(ctx, key)
	if err != nil {
		return nil, err
	}
	return archiveResponse{
		Action:       "archive_status",
		Object:       d.describeObject(args.Obj, key),
		StorageClass: status.StorageClass,
	}, nil
}

func (d *S3) thaw(ctx context.Context, args model.OtherArgs) (interface{}, error) {
	key := getKey(args.Obj.GetPath(), false)
	payload := thawRequest{Days: 1}
	if err := decodeOtherArgs(args.Data, &payload); err != nil {
		return nil, fmt.Errorf("parse thaw request: %w", err)
	}
	if payload.Days <= 0 {
		payload.Days = 1
	}
	restoreRequest := &s3.RestoreRequest{
		Days: aws.Int64(payload.Days),
	}
	if tier := normalizeRestoreTier(payload.Tier); tier != "" {
		restoreRequest.GlacierJobParameters = &s3.GlacierJobParameters{Tier: aws.String(tier)}
	}
	input := &s3.RestoreObjectInput{
		Bucket:         &d.Bucket,
		Key:            &key,
		RestoreRequest: restoreRequest,
	}
	restoreReq, _ := d.client.RestoreObjectRequest(input)
	restoreReq.SetContext(ctx)
	if err := restoreReq.Send(); err != nil {
		return nil, err
	}
	status, _ := d.describeObjectStatus(ctx, key)
	resp := thawResponse{
		Action:    "thaw",
		Object:    d.describeObject(args.Obj, key),
		RequestID: restoreReq.RequestID,
	}
	if status != nil {
		resp.Status = status.Restore
	}
	return resp, nil
}

func (d *S3) thawStatus(ctx context.Context, args model.OtherArgs) (interface{}, error) {
	key := getKey(args.Obj.GetPath(), false)
	status, err := d.describeObjectStatus(ctx, key)
	if err != nil {
		return nil, err
	}
	return thawResponse{
		Action: "thaw_status",
		Object: d.describeObject(args.Obj, key),
		Status: status.Restore,
	}, nil
}

func (d *S3) describeObject(obj model.Obj, key string) objectDescriptor {
	return objectDescriptor{
		Path:   obj.GetPath(),
		Bucket: d.Bucket,
		Key:    key,
	}
}

type objectStatus struct {
	StorageClass string
	Restore      *restoreStatus
}

func (d *S3) describeObjectStatus(ctx context.Context, key string) (*objectStatus, error) {
	head, err := d.client.HeadObjectWithContext(ctx, &s3.HeadObjectInput{Bucket: &d.Bucket, Key: &key})
	if err != nil {
		return nil, err
	}
	status := &objectStatus{
		StorageClass: aws.StringValue(head.StorageClass),
		Restore:      parseRestoreHeader(head.Restore),
	}
	return status, nil
}

func parseRestoreHeader(header *string) *restoreStatus {
	if header == nil {
		return nil
	}
	value := strings.TrimSpace(*header)
	if value == "" {
		return nil
	}
	status := &restoreStatus{Raw: value}
	parts := strings.Split(value, ",")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if strings.HasPrefix(part, "ongoing-request=") {
			status.Ongoing = strings.Contains(part, "\"true\"")
		}
		if strings.HasPrefix(part, "expiry-date=") {
			expiry := strings.Trim(part[len("expiry-date="):], "\"")
			if expiry != "" {
				if t, err := time.Parse(time.RFC1123, expiry); err == nil {
					status.Expiry = t.UTC().Format(time.RFC3339)
				} else {
					status.Expiry = expiry
				}
			}
		}
	}
	return status
}

func decodeOtherArgs(data interface{}, target interface{}) error {
	if data == nil {
		return nil
	}
	raw, err := json.Marshal(data)
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, target)
}

func normalizeStorageClass(value string) string {
	normalized := strings.ToLower(strings.TrimSpace(strings.ReplaceAll(value, "-", "_")))
	if normalized == "" {
		return value
	}
	if v, ok := storageClassLookup[normalized]; ok {
		return v
	}
	return value
}

func normalizeRestoreTier(value string) string {
	normalized := strings.ToLower(strings.TrimSpace(value))
	switch normalized {
	case "", "default":
		return ""
	case "bulk":
		return s3.TierBulk
	case "standard":
		return s3.TierStandard
	case "expedited":
		return s3.TierExpedited
	default:
		return value
	}
}
