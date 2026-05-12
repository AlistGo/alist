package lark

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/alist-org/alist/v3/internal/model"
	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
	larkbitable "github.com/larksuite/oapi-sdk-go/v3/service/bitable/v1"
	larkdrive "github.com/larksuite/oapi-sdk-go/v3/service/drive/v1"
	larksheets "github.com/larksuite/oapi-sdk-go/v3/service/sheets/v3"
	"github.com/pkg/errors"
)

const (
	larkExportOptionsMethod = "lark_export_options"
	larkExportCreateMethod  = "lark_export_create"
	larkExportStatusMethod  = "lark_export_status"

	larkExportStatusPending    = "pending"
	larkExportStatusProcessing = "processing"
	larkExportStatusSuccess    = "success"
	larkExportStatusFailed     = "failed"
)

type larkExportCreateReq struct {
	Format string `json:"format"`
	SubID  string `json:"sub_id"`
}

type larkExportStatusReq struct {
	Ticket string `json:"ticket"`
}

type LarkExportCreateResp struct {
	Ticket string `json:"ticket"`
	Token  string `json:"token"`
	Type   string `json:"type"`
	Format string `json:"format"`
	SubID  string `json:"sub_id,omitempty"`
}

type LarkExportOption struct {
	Value         string `json:"value"`
	Label         string `json:"label"`
	RequiresSubID bool   `json:"requires_sub_id"`
}

type LarkExportSubResource struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Type string `json:"type"`
}

type LarkExportOptionsResp struct {
	Type             string                  `json:"type"`
	Formats          []LarkExportOption      `json:"formats"`
	SubResources     []LarkExportSubResource `json:"sub_resources,omitempty"`
	SubResourceError string                  `json:"sub_resource_error,omitempty"`
}

type LarkExportStatusResp struct {
	Status       string `json:"status"`
	FileToken    string `json:"file_token,omitempty"`
	FileSize     int    `json:"file_size,omitempty"`
	JobStatus    int    `json:"job_status,omitempty"`
	ErrorMessage string `json:"error_message,omitempty"`
	ErrorDetail  string `json:"error_detail,omitempty"`
}

type larkAPIErrorResp interface {
	Error() string
	ErrorResp() string
}

func (c *Lark) Other(ctx context.Context, args model.OtherArgs) (interface{}, error) {
	switch strings.ToLower(strings.TrimSpace(args.Method)) {
	case larkExportOptionsMethod:
		return c.getExportOptions(ctx, args.Obj)
	case larkExportCreateMethod:
		var req larkExportCreateReq
		if err := decodeOtherData(args.Data, &req); err != nil {
			return nil, err
		}
		return c.createExportTask(ctx, args.Obj, req)
	case larkExportStatusMethod:
		var req larkExportStatusReq
		if err := decodeOtherData(args.Data, &req); err != nil {
			return nil, err
		}
		return c.getExportTask(ctx, args.Obj, req)
	default:
		return nil, fmt.Errorf("unsupported lark method: %s", args.Method)
	}
}

func decodeOtherData(data interface{}, v interface{}) error {
	b, err := json.Marshal(data)
	if err != nil {
		return errors.WithMessage(err, "failed to encode request data")
	}
	if err = json.Unmarshal(b, v); err != nil {
		return errors.WithMessage(err, "failed to decode request data")
	}
	return nil
}

func (c *Lark) createExportTask(ctx context.Context, obj model.Obj, req larkExportCreateReq) (*LarkExportCreateResp, error) {
	token, ok := c.getObjToken(ctx, obj.GetPath())
	if !ok {
		return nil, errors.WithStack(errors.New("lark file token not found"))
	}
	docType, err := larkExportType(obj.GetName())
	if err != nil {
		return nil, err
	}
	format := strings.ToLower(strings.TrimSpace(req.Format))
	if !larkExportFormatAllowed(docType, format) {
		return nil, fmt.Errorf("unsupported export format %q for lark type %q", format, docType)
	}
	subID := strings.TrimSpace(req.SubID)
	if larkExportFormatRequiresSubID(docType, format) && subID == "" {
		return nil, fmt.Errorf("sub_id is required when exporting lark type %q as %q", docType, format)
	}

	builder := larkdrive.NewExportTaskBuilder().
		Token(token).
		Type(docType).
		FileExtension(format).
		FileName(larkExportBaseName(obj.GetName()))
	if subID != "" {
		builder.SubId(subID)
	}
	exportTask := builder.Build()
	resp, err := doDrive(ctx, c, func(opts ...larkcore.RequestOptionFunc) (*larkdrive.CreateExportTaskResp, error) {
		return c.client.Drive.V1.ExportTask.Create(ctx,
			larkdrive.NewCreateExportTaskReqBuilder().ExportTask(exportTask).Build(), opts...)
	})
	if err != nil {
		return nil, err
	}
	if !resp.Success() {
		return nil, larkAPIError(resp)
	}
	if resp.Data == nil || resp.Data.Ticket == nil || *resp.Data.Ticket == "" {
		return nil, errors.New("lark export task response missing ticket")
	}
	return &LarkExportCreateResp{
		Ticket: *resp.Data.Ticket,
		Token:  token,
		Type:   docType,
		Format: format,
		SubID:  subID,
	}, nil
}

func (c *Lark) getExportOptions(ctx context.Context, obj model.Obj) (*LarkExportOptionsResp, error) {
	token, ok := c.getObjToken(ctx, obj.GetPath())
	if !ok {
		return nil, errors.WithStack(errors.New("lark file token not found"))
	}
	docType, err := larkExportType(obj.GetName())
	if err != nil {
		return nil, err
	}
	out := &LarkExportOptionsResp{
		Type:    docType,
		Formats: larkExportOptions(docType),
	}
	switch docType {
	case larkdrive.TypeSheet:
		out.SubResources, err = c.listSheetSubResources(ctx, token)
	case larkdrive.TypeBitable:
		out.SubResources, err = c.listBitableSubResources(ctx, token)
	}
	if err != nil {
		out.SubResourceError = err.Error()
	}
	return out, nil
}

func (c *Lark) getExportTask(ctx context.Context, obj model.Obj, req larkExportStatusReq) (*LarkExportStatusResp, error) {
	ticket := strings.TrimSpace(req.Ticket)
	if ticket == "" {
		return nil, errors.New("ticket is required")
	}
	token, ok := c.getObjToken(ctx, obj.GetPath())
	if !ok {
		return nil, errors.WithStack(errors.New("lark file token not found"))
	}
	resp, err := doDrive(ctx, c, func(opts ...larkcore.RequestOptionFunc) (*larkdrive.GetExportTaskResp, error) {
		return c.client.Drive.V1.ExportTask.Get(ctx,
			larkdrive.NewGetExportTaskReqBuilder().Ticket(ticket).Token(token).Build(), opts...)
	})
	if err != nil {
		return nil, err
	}
	if !resp.Success() {
		return nil, larkAPIError(resp)
	}
	if resp.Data == nil || resp.Data.Result == nil {
		return nil, errors.New("lark export task response missing result")
	}
	result := resp.Data.Result
	out := &LarkExportStatusResp{
		Status: larkExportJobStatus(result),
	}
	if result.FileToken != nil {
		out.FileToken = *result.FileToken
	}
	if result.FileSize != nil {
		out.FileSize = *result.FileSize
	}
	if result.JobStatus != nil {
		out.JobStatus = *result.JobStatus
	}
	if out.Status == larkExportStatusFailed {
		out.ErrorMessage = larkExportTaskErrorMessage(result)
		out.ErrorDetail = larkExportTaskErrorDetail(result)
	} else if result.JobErrorMsg != nil {
		out.ErrorMessage = strings.TrimSpace(*result.JobErrorMsg)
	}
	return out, nil
}

func (c *Lark) DownloadExportFile(ctx context.Context, fileToken string) (io.Reader, string, error) {
	fileToken = strings.TrimSpace(fileToken)
	if fileToken == "" {
		return nil, "", errors.New("file_token is required")
	}
	resp, err := doDrive(ctx, c, func(opts ...larkcore.RequestOptionFunc) (*larkdrive.DownloadExportTaskResp, error) {
		return c.client.Drive.V1.ExportTask.Download(ctx,
			larkdrive.NewDownloadExportTaskReqBuilder().FileToken(fileToken).Build(), opts...)
	})
	if err != nil {
		return nil, "", err
	}
	if !resp.Success() {
		return nil, "", larkAPIError(resp)
	}
	if resp.File == nil {
		return nil, "", errors.New("lark export download response missing file")
	}
	return resp.File, resp.FileName, nil
}

func larkExportType(name string) (string, error) {
	switch {
	case strings.HasSuffix(name, ".lark-doc"):
		return larkdrive.TypeDoc, nil
	case strings.HasSuffix(name, ".lark-docx"):
		return larkdrive.TypeDocx, nil
	case strings.HasSuffix(name, ".lark-sheet"):
		return larkdrive.TypeSheet, nil
	case strings.HasSuffix(name, ".lark-bitable"):
		return larkdrive.TypeBitable, nil
	default:
		return "", fmt.Errorf("unsupported lark export file type: %s", name)
	}
}

func larkExportFormatAllowed(docType, format string) bool {
	switch docType {
	case larkdrive.TypeDoc, larkdrive.TypeDocx:
		return format == larkdrive.FileExtensionPdf || format == larkdrive.FileExtensionDocx
	case larkdrive.TypeSheet, larkdrive.TypeBitable:
		return format == larkdrive.FileExtensionXlsx || format == larkdrive.FileExtensionCsv
	default:
		return false
	}
}

func larkExportFormatRequiresSubID(docType, format string) bool {
	return (docType == larkdrive.TypeSheet || docType == larkdrive.TypeBitable) && format == larkdrive.FileExtensionCsv
}

func larkExportOptions(docType string) []LarkExportOption {
	switch docType {
	case larkdrive.TypeDoc, larkdrive.TypeDocx:
		return []LarkExportOption{
			{Value: larkdrive.FileExtensionPdf, Label: "PDF"},
			{Value: larkdrive.FileExtensionDocx, Label: "DOCX"},
		}
	case larkdrive.TypeSheet, larkdrive.TypeBitable:
		return []LarkExportOption{
			{Value: larkdrive.FileExtensionXlsx, Label: "XLSX"},
			{Value: larkdrive.FileExtensionCsv, Label: "CSV", RequiresSubID: true},
		}
	default:
		return nil
	}
}

func larkExportBaseName(name string) string {
	return trimLarkDisplayExt(name)
}

func larkExportJobStatus(result *larkdrive.ExportTask) string {
	if result == nil {
		return larkExportStatusProcessing
	}
	if result.FileToken != nil && *result.FileToken != "" {
		return larkExportStatusSuccess
	}
	if result.JobStatus != nil && *result.JobStatus == 2 {
		return larkExportStatusFailed
	}
	if result.JobErrorMsg != nil && *result.JobErrorMsg != "" && !strings.EqualFold(*result.JobErrorMsg, "success") {
		return larkExportStatusFailed
	}
	return larkExportStatusProcessing
}

func larkAPIError(resp larkAPIErrorResp) error {
	msg := strings.TrimSpace(resp.ErrorResp())
	if msg == "" || msg == "{}" || msg == "null" {
		msg = strings.TrimSpace(resp.Error())
	}
	return errors.New(msg)
}

func larkExportTaskErrorMessage(result *larkdrive.ExportTask) string {
	if result == nil {
		return ""
	}
	if result.JobErrorMsg != nil {
		msg := strings.TrimSpace(*result.JobErrorMsg)
		if msg != "" && !strings.EqualFold(msg, "success") {
			return msg
		}
	}
	if result.JobStatus != nil {
		return fmt.Sprintf("job_status=%d", *result.JobStatus)
	}
	return ""
}

func larkExportTaskErrorDetail(result *larkdrive.ExportTask) string {
	if result == nil {
		return ""
	}
	detail := map[string]interface{}{}
	if result.JobStatus != nil {
		detail["job_status"] = *result.JobStatus
	}
	if result.JobErrorMsg != nil {
		detail["job_error_msg"] = strings.TrimSpace(*result.JobErrorMsg)
	}
	if len(detail) == 0 {
		return ""
	}
	b, err := json.Marshal(detail)
	if err != nil {
		return ""
	}
	return string(b)
}

func (c *Lark) listSheetSubResources(ctx context.Context, token string) ([]LarkExportSubResource, error) {
	resp, err := doDrive(ctx, c, func(opts ...larkcore.RequestOptionFunc) (*larksheets.QuerySpreadsheetSheetResp, error) {
		return c.client.Sheets.SpreadsheetSheet.Query(ctx,
			larksheets.NewQuerySpreadsheetSheetReqBuilder().SpreadsheetToken(token).Build(), opts...)
	})
	if err != nil {
		return nil, err
	}
	if !resp.Success() {
		return nil, larkAPIError(resp)
	}
	if resp.Data == nil {
		return nil, nil
	}
	var res []LarkExportSubResource
	for _, sheet := range resp.Data.Sheets {
		if sheet == nil || sheet.SheetId == nil || strings.TrimSpace(*sheet.SheetId) == "" {
			continue
		}
		name := strings.TrimSpace(larkString(sheet.Title))
		if name == "" {
			name = *sheet.SheetId
		}
		res = append(res, LarkExportSubResource{
			ID:   *sheet.SheetId,
			Name: name,
			Type: strings.TrimSpace(larkString(sheet.ResourceType)),
		})
	}
	return res, nil
}

func (c *Lark) listBitableSubResources(ctx context.Context, token string) ([]LarkExportSubResource, error) {
	var res []LarkExportSubResource
	pageToken := ""
	for {
		builder := larkbitable.NewListAppTableReqBuilder().AppToken(token).PageSize(100)
		if pageToken != "" {
			builder.PageToken(pageToken)
		}
		resp, err := doDrive(ctx, c, func(opts ...larkcore.RequestOptionFunc) (*larkbitable.ListAppTableResp, error) {
			return c.client.Bitable.AppTable.List(ctx, builder.Build(), opts...)
		})
		if err != nil {
			return nil, err
		}
		if !resp.Success() {
			return nil, larkAPIError(resp)
		}
		if resp.Data == nil {
			return res, nil
		}
		for _, table := range resp.Data.Items {
			if table == nil || table.TableId == nil || strings.TrimSpace(*table.TableId) == "" {
				continue
			}
			name := strings.TrimSpace(larkString(table.Name))
			if name == "" {
				name = *table.TableId
			}
			res = append(res, LarkExportSubResource{
				ID:   *table.TableId,
				Name: name,
				Type: "table",
			})
		}
		if resp.Data.HasMore == nil || !*resp.Data.HasMore || resp.Data.PageToken == nil || *resp.Data.PageToken == "" {
			return res, nil
		}
		pageToken = *resp.Data.PageToken
	}
}
