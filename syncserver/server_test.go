package syncserver

import "testing"

func TestValidServerAttachmentUsesAllowList(t *testing.T) {
	tests := []struct {
		name, mime string
		want       bool
	}{
		{"photo.png", "image/png", true},
		{"report.docx", "application/vnd.openxmlformats-officedocument.wordprocessingml.document", true},
		{"archive.zip", "application/zip", true},
		{"payload.exe", "application/octet-stream", false},
		{"page.svg", "image/svg+xml", false},
		{"unknown.bin", "application/octet-stream", false},
		{"fake.txt", "text/html", false},
	}
	for _, test := range tests {
		if got := validServerAttachment(test.name, test.mime); got != test.want {
			t.Errorf("validServerAttachment(%q, %q) = %v, want %v", test.name, test.mime, got, test.want)
		}
	}
}
