package rpc

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/url"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
	log "github.com/sirupsen/logrus"
)

type caller interface {
	// Call sends a request of rpc to aria2 daemon
	Call(method string, params, reply interface{}) (err error)
	Close() error
}

type httpCaller struct {
	uri    string
	c      *http.Client
	cancel context.CancelFunc
	wg     *sync.WaitGroup
	once   sync.Once
}

func newHTTPCaller(ctx context.Context, u *url.URL, timeout time.Duration, notifier Notifier) *httpCaller {
	c := &http.Client{
		Transport: &http.Transport{
			MaxIdleConnsPerHost: 1,
			MaxConnsPerHost:     1,
			// TLSClientConfig:     tlsConfig,
			Dial: (&net.Dialer{
				Timeout:   timeout,
				KeepAlive: 60 * time.Second,
			}).Dial,
			TLSHandshakeTimeout:   3 * time.Second,
			ResponseHeaderTimeout: timeout,
		},
	}
	var wg sync.WaitGroup
	ctx, cancel := context.WithCancel(ctx)
	h := &httpCaller{uri: u.String(), c: c, cancel: cancel, wg: &wg}
	if notifier != nil {
		h.setNotifier(ctx, *u, notifier)
	}
	return h
}

func (h *httpCaller) Close() (err error) {
	h.once.Do(func() {
		h.cancel()
		h.wg.Wait()
	})
	return
}

func (h *httpCaller) setNotifier(ctx context.Context, u url.URL, notifier Notifier) (err error) {
	u.Scheme = "ws"
	conn, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		return
	}
	h.wg.Add(1)
	go func() {
		defer h.wg.Done()
		defer conn.Close()
		select {
		case <-ctx.Done():
			conn.SetWriteDeadline(time.Now().Add(time.Second))
			if err := conn.WriteMessage(websocket.CloseMessage,
				websocket.FormatCloseMessage(websocket.CloseNormalClosure, "")); err != nil {
				log.Printf("sending websocket close message: %v", err)
			}
			return
		}
	}()
	h.wg.Add(1)
	go func() {
		defer h.wg.Done()
		var request websocketResponse
		var err error
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}
			if err = conn.ReadJSON(&request); err != nil {
				select {
				case <-ctx.Done():
					return
				default:
				}
				log.Printf("conn.ReadJSON|err:%v", err.Error())
				return
			}
			switch request.Method {
			case "aria2.onDownloadStart":
				notifier.OnDownloadStart(request.Params)
			case "aria2.onDownloadPause":
				notifier.OnDownloadPause(request.Params)
			case "aria2.onDownloadStop":
				notifier.OnDownloadStop(request.Params)
			case "aria2.onDownloadComplete":
				notifier.OnDownloadComplete(request.Params)
			case "aria2.onDownloadError":
				notifier.OnDownloadError(request.Params)
			case "aria2.onBtDownloadComplete":
				notifier.OnBtDownloadComplete(request.Params)
			default:
				log.Printf("unexpected notification: %s", request.Method)
			}
		}
	}()
	return
}

func (h *httpCaller) Call(method string, params, reply interface{}) (err error) {
	payload, err := EncodeClientRequest(method, params)
	if err != nil {
		return
	}
	r, err := h.c.Post(h.uri, "application/json", payload)
	if err != nil {
		return
	}
	err = DecodeClientResponse(r.Body, &reply)
	r.Body.Close()
	return
}

type websocketCaller struct {
	conn     *websocket.Conn
	sendChan chan *sendRequest
	cancel   context.CancelFunc
	wg       *sync.WaitGroup
	once     sync.Once
	timeout  time.Duration
}

func newWebsocketCaller(ctx context.Context, uri string, timeout time.Duration, notifier Notifier) (*websocketCaller, error) {
	var header = http.Header{}
	conn, _, err := websocket.DefaultDialer.Dial(uri, header)
	if err != nil {
		return nil, err
	}

	sendChan := make(chan *sendRequest, 16)
	var wg sync.WaitGroup
	ctx, cancel := context.WithCancel(ctx)
	w := &websocketCaller{conn: conn, wg: &wg, cancel: cancel, sendChan: sendChan, timeout: timeout}
	processor := NewResponseProcessor()
	wg.Add(1)
	go func() { // routine:recv
		defer wg.Done()
		defer cancel()
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}
			var resp websocketResponse
			if err := conn.ReadJSON(&resp); err != nil {
				select {
				case <-ctx.Done():
					return
				default:
				}
				log.Printf("conn.ReadJSON|err:%v", err.Error())
				return
			}
			if resp.Id == nil { // RPC notifications
				if notifier != nil {
					switch resp.Method {
					case "aria2.onDownloadStart":
						notifier.OnDownloadStart(resp.Params)
					case "aria2.onDownloadPause":
						notifier.OnDownloadPause(resp.Params)
					case "aria2.onDownloadStop":
						notifier.OnDownloadStop(resp.Params)
					case "aria2.onDownloadComplete":
						notifier.OnDownloadComplete(resp.Params)
					case "aria2.onDownloadError":
						notifier.OnDownloadError(resp.Params)
					case "aria2.onBtDownloadComplete":
						notifier.OnBtDownloadComplete(resp.Params)
					default:
						log.Printf("unexpected notification: %s", resp.Method)
					}
				}
				continue
			}
			processor.Process(resp.clientResponse)
		}
	}()
	wg.Add(1)
	go func() { // routine:send
		defer wg.Done()
		defer cancel()
		defer w.conn.Close()

		for {
			select {
			case <-ctx.Done():
				if err := w.conn.WriteMessage(websocket.CloseMessage,
					websocket.FormatCloseMessage(websocket.CloseNormalClosure, "")); err != nil {
					log.Printf("sending websocket close message: %v", err)
				}
				return
			case req := <-sendChan:
				processor.Add(req.request.Id, func(resp clientResponse) error {
					err := resp.decode(req.reply)
					req.cancel()
					return err
				})
				w.conn.SetWriteDeadline(time.Now().Add(timeout))
				w.conn.WriteJSON(req.request)
			}
		}
	}()

	return w, nil
}

func (w *websocketCaller) Close() (err error) {
	w.once.Do(func() {
		w.cancel()
		w.wg.Wait()
	})
	return
}

func (w *websocketCaller) Call(method string, params, reply interface{}) (err error) {
	ctx, cancel := context.WithTimeout(context.Background(), w.timeout)
	defer cancel()
	select {
	case w.sendChan <- &sendRequest{cancel: cancel, request: &clientRequest{
		Version: "2.0",
		Method:  method,
		Params:  params,
		Id:      reqid(),
	}, reply: reply}:

	default:
		return errors.New("sending channel blocking")
	}

	select {
	case <-ctx.Done():
		if err := ctx.Err(); err == context.DeadlineExceeded {
			return err
		}
	}
	return
}

type sendRequest struct {
	cancel  context.CancelFunc
	request *clientRequest
	reply   interface{}
}

var reqid = func() func() uint64 {
	var id = uint64(time.Now().UnixNano())
	return func() uint64 {
		return atomic.AddUint64(&id, 1)
	}
}()
