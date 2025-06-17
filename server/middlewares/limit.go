package middlewares

import (
	"github.com/alist-org/alist/v3/internal/stream"
	"github.com/gin-gonic/gin"
	"io"
)

func MaxAllowed(n int) gin.HandlerFunc {
	sem := make(chan struct{}, n)
	acquire := func() { sem <- struct{}{} }
	release := func() { <-sem }
	return func(c *gin.Context) {
		acquire()
		defer release()
		c.Next()
	}
}

func UploadRateLimiter(limiter stream.Limiter) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Request.Body = &stream.RateLimitReader{
			Reader:  c.Request.Body,
			Limiter: limiter,
			Ctx:     c,
		}
		c.Next()
	}
}

type ResponseWriterWrapper struct {
	gin.ResponseWriter
	WrapWriter io.Writer
}

func (w *ResponseWriterWrapper) Write(p []byte) (n int, err error) {
	return w.WrapWriter.Write(p)
}

func DownloadRateLimiter(limiter stream.Limiter) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Writer = &ResponseWriterWrapper{
			ResponseWriter: c.Writer,
			WrapWriter: &stream.RateLimitWriter{
				Writer:  c.Writer,
				Limiter: limiter,
				Ctx:     c,
			},
		}
		c.Next()
	}
}
