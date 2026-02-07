package ftps

import (
	"crypto/tls"
	"io"
	"net"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jlaffaye/ftp"
)

func (d *FTPS) login() error {
	if d.conn != nil {
		_, err := d.conn.CurrentDir()
		if err == nil {
			return nil
		}
	}

	host, _, err := net.SplitHostPort(d.Address)
	if err != nil {
		host = d.Address
	}

	tlsConfig := &tls.Config{
		ServerName:         host,
		InsecureSkipVerify: d.TLSInsecureSkipVerify,
	}

	opts := []ftp.DialOption{
		ftp.DialWithShutTimeout(10 * time.Second),
	}
	if d.TLSMode == "Implicit" {
		opts = append(opts, ftp.DialWithTLS(tlsConfig))
	} else {
		opts = append(opts, ftp.DialWithExplicitTLS(tlsConfig))
	}

	conn, err := ftp.Dial(d.Address, opts...)
	if err != nil {
		return err
	}
	err = conn.Login(d.Username, d.Password)
	if err != nil {
		_ = conn.Quit()
		return err
	}
	d.conn = conn
	return nil
}

type FileReader struct {
	conn         *ftp.ServerConn
	resp         *ftp.Response
	offset       atomic.Int64
	readAtOffset int64
	mu           sync.Mutex
	path         string
	size         int64
}

func NewFileReader(conn *ftp.ServerConn, path string, size int64) *FileReader {
	return &FileReader{
		conn: conn,
		path: path,
		size: size,
	}
}

func (r *FileReader) Read(buf []byte) (n int, err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	off := r.offset.Load()
	n, err = r.readAtLocked(buf, off)
	r.offset.Add(int64(n))
	return
}

func (r *FileReader) ReadAt(buf []byte, off int64) (n int, err error) {
	if off < 0 {
		return 0, os.ErrInvalid
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.readAtLocked(buf, off)
}

func (r *FileReader) readAtLocked(buf []byte, off int64) (n int, err error) {
	if r.resp != nil && off != r.readAtOffset {
		_ = r.resp.Close()
		r.resp = nil
	}

	if r.resp == nil {
		r.resp, err = r.conn.RetrFrom(r.path, uint64(off))
		r.readAtOffset = off
		if err != nil {
			return 0, err
		}
	}

	n, err = r.resp.Read(buf)
	r.readAtOffset += int64(n)
	return
}

func (r *FileReader) Seek(offset int64, whence int) (int64, error) {
	oldOffset := r.offset.Load()
	var newOffset int64
	switch whence {
	case io.SeekStart:
		newOffset = offset
	case io.SeekCurrent:
		newOffset = oldOffset + offset
	case io.SeekEnd:
		newOffset = r.size + offset
	default:
		return -1, os.ErrInvalid
	}

	if newOffset < 0 {
		return oldOffset, os.ErrInvalid
	}
	if newOffset == oldOffset {
		return oldOffset, nil
	}
	r.offset.Store(newOffset)
	return newOffset, nil
}

func (r *FileReader) Close() error {
	if r.resp != nil {
		return r.resp.Close()
	}
	return nil
}
