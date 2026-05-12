package lark

import (
	"testing"

	larkdrive "github.com/larksuite/oapi-sdk-go/v3/service/drive/v1"
)

func TestLarkExportType(t *testing.T) {
	tests := []struct {
		name    string
		want    string
		wantErr bool
	}{
		{name: "doc.lark-doc", want: larkdrive.TypeDoc},
		{name: "doc.lark-docx", want: larkdrive.TypeDocx},
		{name: "sheet.lark-sheet", want: larkdrive.TypeSheet},
		{name: "base.lark-bitable", want: larkdrive.TypeBitable},
		{name: "file.pdf", wantErr: true},
	}
	for _, tt := range tests {
		got, err := larkExportType(tt.name)
		if tt.wantErr {
			if err == nil {
				t.Fatalf("larkExportType(%q) expected error", tt.name)
			}
			continue
		}
		if err != nil {
			t.Fatalf("larkExportType(%q) unexpected error: %v", tt.name, err)
		}
		if got != tt.want {
			t.Fatalf("larkExportType(%q) = %q, want %q", tt.name, got, tt.want)
		}
	}
}

func TestLarkExportFormatAllowed(t *testing.T) {
	tests := []struct {
		docType string
		format  string
		want    bool
	}{
		{docType: larkdrive.TypeDoc, format: larkdrive.FileExtensionPdf, want: true},
		{docType: larkdrive.TypeDocx, format: larkdrive.FileExtensionDocx, want: true},
		{docType: larkdrive.TypeSheet, format: larkdrive.FileExtensionXlsx, want: true},
		{docType: larkdrive.TypeBitable, format: larkdrive.FileExtensionXlsx, want: true},
		{docType: larkdrive.TypeSheet, format: larkdrive.FileExtensionCsv, want: true},
		{docType: larkdrive.TypeBitable, format: larkdrive.FileExtensionCsv, want: true},
		{docType: larkdrive.TypeBitable, format: larkdrive.FileExtensionPdf, want: false},
	}
	for _, tt := range tests {
		if got := larkExportFormatAllowed(tt.docType, tt.format); got != tt.want {
			t.Fatalf("larkExportFormatAllowed(%q, %q) = %v, want %v", tt.docType, tt.format, got, tt.want)
		}
	}
}

func TestLarkExportFormatRequiresSubID(t *testing.T) {
	tests := []struct {
		docType string
		format  string
		want    bool
	}{
		{docType: larkdrive.TypeSheet, format: larkdrive.FileExtensionCsv, want: true},
		{docType: larkdrive.TypeBitable, format: larkdrive.FileExtensionCsv, want: true},
		{docType: larkdrive.TypeSheet, format: larkdrive.FileExtensionXlsx, want: false},
		{docType: larkdrive.TypeDocx, format: larkdrive.FileExtensionDocx, want: false},
	}
	for _, tt := range tests {
		if got := larkExportFormatRequiresSubID(tt.docType, tt.format); got != tt.want {
			t.Fatalf("larkExportFormatRequiresSubID(%q, %q) = %v, want %v", tt.docType, tt.format, got, tt.want)
		}
	}
}

func TestLarkExportOptions(t *testing.T) {
	tests := []struct {
		docType string
		want    []LarkExportOption
	}{
		{docType: larkdrive.TypeDocx, want: []LarkExportOption{
			{Value: larkdrive.FileExtensionPdf, Label: "PDF"},
			{Value: larkdrive.FileExtensionDocx, Label: "DOCX"},
		}},
		{docType: larkdrive.TypeSheet, want: []LarkExportOption{
			{Value: larkdrive.FileExtensionXlsx, Label: "XLSX"},
			{Value: larkdrive.FileExtensionCsv, Label: "CSV", RequiresSubID: true},
		}},
	}
	for _, tt := range tests {
		got := larkExportOptions(tt.docType)
		if len(got) != len(tt.want) {
			t.Fatalf("larkExportOptions(%q) len = %d, want %d", tt.docType, len(got), len(tt.want))
		}
		for i := range got {
			if got[i] != tt.want[i] {
				t.Fatalf("larkExportOptions(%q)[%d] = %+v, want %+v", tt.docType, i, got[i], tt.want[i])
			}
		}
	}
}

func TestLarkExportBaseName(t *testing.T) {
	tests := map[string]string{
		"weekly.lark-docx":       "weekly",
		"weekly.report.lark-doc": "weekly.report",
		"table.lark-sheet":       "table",
		"plain.pdf":              "plain.pdf",
	}
	for name, want := range tests {
		if got := larkExportBaseName(name); got != want {
			t.Fatalf("larkExportBaseName(%q) = %q, want %q", name, got, want)
		}
	}
}

func TestLarkExportJobStatus(t *testing.T) {
	successToken := "box_token"
	successCode := 0
	failCode := 2
	failMsg := "failed"
	successMsg := "success"

	tests := []struct {
		name   string
		result *larkdrive.ExportTask
		want   string
	}{
		{name: "nil", result: nil, want: larkExportStatusProcessing},
		{name: "file token wins", result: &larkdrive.ExportTask{FileToken: &successToken, JobStatus: &failCode}, want: larkExportStatusSuccess},
		{name: "status failure", result: &larkdrive.ExportTask{JobStatus: &failCode}, want: larkExportStatusFailed},
		{name: "error message failure", result: &larkdrive.ExportTask{JobStatus: &successCode, JobErrorMsg: &failMsg}, want: larkExportStatusFailed},
		{name: "success message without token still processing", result: &larkdrive.ExportTask{JobStatus: &successCode, JobErrorMsg: &successMsg}, want: larkExportStatusProcessing},
	}
	for _, tt := range tests {
		if got := larkExportJobStatus(tt.result); got != tt.want {
			t.Fatalf("%s: larkExportJobStatus() = %q, want %q", tt.name, got, tt.want)
		}
	}
}

func TestLarkExportTaskErrorMessage(t *testing.T) {
	failCode := 2
	failMsg := "source document cannot be exported"
	successMsg := "success"

	tests := []struct {
		name   string
		result *larkdrive.ExportTask
		want   string
	}{
		{name: "nil", result: nil, want: ""},
		{name: "job error message", result: &larkdrive.ExportTask{JobStatus: &failCode, JobErrorMsg: &failMsg}, want: failMsg},
		{name: "status fallback", result: &larkdrive.ExportTask{JobStatus: &failCode, JobErrorMsg: &successMsg}, want: "job_status=2"},
	}
	for _, tt := range tests {
		if got := larkExportTaskErrorMessage(tt.result); got != tt.want {
			t.Fatalf("%s: larkExportTaskErrorMessage() = %q, want %q", tt.name, got, tt.want)
		}
	}
}

func TestLarkExportTaskErrorDetail(t *testing.T) {
	failCode := 2
	failMsg := "source document cannot be exported"

	got := larkExportTaskErrorDetail(&larkdrive.ExportTask{
		JobStatus:   &failCode,
		JobErrorMsg: &failMsg,
	})
	want := `{"job_error_msg":"source document cannot be exported","job_status":2}`
	if got != want {
		t.Fatalf("larkExportTaskErrorDetail() = %q, want %q", got, want)
	}
}
