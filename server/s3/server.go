// Credits: https://pkg.go.dev/github.com/rclone/rclone@v1.65.2/cmd/serve/s3
// Package s3 implements a fake s3 server for alist
package s3

import (
	"bytes"
	"context"
	"math/rand"
	"net/http"

	"github.com/alist-org/gofakes3"
)

// Make a new S3 Server to serve the remote
func NewServer(ctx context.Context) (h http.Handler, err error) {
	var newLogger logger
	faker := gofakes3.New(
		newBackend(),
		// gofakes3.WithHostBucket(!opt.pathBucketMode),
		gofakes3.WithLogger(newLogger),
		gofakes3.WithRequestID(rand.Uint64()),
		gofakes3.WithoutVersioning(),
		gofakes3.WithV4Auth(authlistResolver()),
		gofakes3.WithIntegrityCheck(true), // Check Content-MD5 if supplied
	)

	return multipartInitXML(faker.Server()), nil
}

// gofakes3 v0.0.7 uses the Go type name as the XML root for this response.
func multipartInitXML(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || !r.URL.Query().Has("uploads") {
			next.ServeHTTP(w, r)
			return
		}
		resp := &multipartInitResponse{ResponseWriter: w}
		next.ServeHTTP(resp, r)
		body := resp.body.Bytes()
		if resp.status == http.StatusOK && bytes.Contains(body, []byte("<InitiateMultipartUpload>")) {
			body = bytes.Replace(body, []byte("<InitiateMultipartUpload>"), []byte("<InitiateMultipartUploadResult>"), 1)
			body = bytes.Replace(body, []byte("</InitiateMultipartUpload>"), []byte("</InitiateMultipartUploadResult>"), 1)
			w.Header().Del("Content-Length")
		}
		if resp.status != 0 {
			w.WriteHeader(resp.status)
		}
		if len(body) != 0 {
			_, _ = w.Write(body)
		}
	})
}

type multipartInitResponse struct {
	http.ResponseWriter
	body   bytes.Buffer
	status int
}

func (w *multipartInitResponse) WriteHeader(status int) {
	if w.status == 0 {
		w.status = status
	}
}

func (w *multipartInitResponse) Write(p []byte) (int, error) {
	w.WriteHeader(http.StatusOK)
	return w.body.Write(p)
}
