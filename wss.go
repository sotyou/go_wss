package go_wss

import (
    "context"
    "fmt"
    "github.com/gorilla/websocket"
    "github.com/pkg/errors"
    logger "github.com/sotyou/go_logger"
    "net/http"
    "net/http/httputil"
    "sync"
    "sync/atomic"
    "time"
)

func New(opts ...WssClientOpt) *WssClient {
    ctx, cancel := context.WithCancel(context.Background())
    ws := &WssClient{
        mode:   EventChannel,
        ctx:    ctx,
        conn:   nil,
        dialer: websocket.DefaultDialer,
        cancel: cancel,
        reconnectCount: defaultRetry,

        channels: make(map[string]struct{}, defaultSubCount),

        MsgFunc:            defaultMsgFunc,
        UncompressedFunc:   defaultUncompressedFunc,
        SystemErrFunc:      defaultSystemErrorFunc,
        AfterConnectedFunc: defaultAfterConnectedFunc,
        SubFunc:            defaultSubFunc,
        UnsubFunc:          defaultUnsubFunc,
        connected:          NotConnected,
    }
    for _, fn := range opts {
        fn(ws)
    }
    if ws.mode == EventChannel && ws.channel == nil {
        ws.channel = make(chan []byte)
    }
    return ws
}

func (w *WssClient) Channel() (chan []byte, error) {
    if w.channel == nil {
        return nil, ErrMessageChannelEmpty
    }
    return w.channel, nil
}

func (w *WssClient) Config(opts ...WssClientOpt) *WssClient {
    for _, fn := range opts {
        fn(w)
    }
    return w
}

type WssClient struct {
    sync.Mutex
    header             http.Header
    channels           map[string]struct{}
    channel            chan []byte
    readDeadLineTime   time.Duration
    reconnectInterval  time.Duration
    wssUrl             string

    SubFunc            func(ch string) string
    UnsubFunc          func(ch string) string
    MsgFunc            func(msg []byte) error
    UncompressedFunc   func(msg []byte) ([]byte, error)
    SystemErrFunc      func(err error)
    AfterConnectedFunc func() error

    ctx                context.Context
    conn               *websocket.Conn
    dialer             *websocket.Dialer
    cancel             context.CancelFunc

    mode               uint32
    connected          uint32
    isDump             bool
    isAutoReconnect    bool
    reconnectCount     int
}

func (w *WssClient) Conn() *websocket.Conn {
    return w.conn
}

func DumpResponse(resp *http.Response, body bool) {
    if resp == nil {
        return
    }
    d, _ := httputil.DumpResponse(resp, body)
    logger.Info("[ws] response:", string(d))
}

func (w *WssClient) Connect() error {
    conn, resp, err := w.dialer.Dial(w.wssUrl, w.header)
    defer w.dump(resp, true)
    if err != nil {
        fmt.Println(err)
        return err
    }
    w.conn = conn
    w.connected = Connected

    err = w.AfterConnectedFunc()
    if err != nil {
        return err
    }

    for key, _ := range w.channels {
        err = w.conn.WriteMessage(websocket.TextMessage, []byte(w.SubFunc(key)))
        if err != nil {
            fmt.Println("err", err)
            w.SystemErrFunc(err)
        }
    }
    return nil
}

func (w *WssClient) Close() {
    err := w.conn.Close()
    if err != nil {
        w.SystemErrFunc(errors.Wrapf(err, "[ws] [%s] Close websocket error", w.wssUrl))
    } else {
        logger.Info("[ws] Close websocket success.", nil)
    }
    time.Sleep(time.Second)
    w.cancel()
}

func (w *WssClient) SubAll() error {
    if w.connected == NotConnected {
        return ErrWssNotConnected
    }
    var err error
    for key, _ := range w.channels {
        err = w.conn.WriteMessage(websocket.TextMessage, []byte(w.SubFunc(key)))
        if err != nil {
            w.SystemErrFunc(err)
        }
    }
    return nil
}

func (w *WssClient) UnSubAll() error {
    if w.connected == NotConnected {
        return ErrWssNotConnected
    }
    var err error
    for key, _ := range w.channels {
        err = w.conn.WriteMessage(websocket.TextMessage, []byte(w.UnsubFunc(key)))
        if err != nil {
            w.SystemErrFunc(err)
        }
    }
    w.channels = make(map[string]struct{}, defaultSubCount)
    return nil
}

func (w *WssClient) Subs(chs []string) {
    w.Lock()
    defer w.Unlock()
    for _, v := range chs {
        w.channels[v] = struct{}{}
    }
}

func (w *WssClient) UnSubs() {
    w.Lock()
    defer w.Unlock()
    w.channels = map[string]struct{}{}
}

// Sub add a channel to subscribe, Sub is concurrent safe
func (w *WssClient) Sub(ch string) error {
    w.Lock()
    defer w.Unlock()
    if _, ok := w.channels[ch]; ok {
        return ErrChannelDuplicateSubscribe
    }
    w.channels[ch] = struct{}{}
    logger.Info("[ws] add channel to subscription", ch)

    err := w.conn.WriteMessage(websocket.TextMessage, []byte(w.SubFunc(ch)))
    if err != nil {
        w.SystemErrFunc(err)
    }
    return err
}

// Unsub add a channel to unsubscribe, Unsub is concurrent safe
func (w *WssClient) Unsub(ch string) error {
    w.Lock()
    defer w.Unlock()
    delete(w.channels, ch)
    err := w.conn.WriteMessage(websocket.TextMessage, []byte(w.UnsubFunc(ch)))
    if err != nil {
        w.SystemErrFunc(err)
    }
    return err
}

func (w *WssClient) ReceiveMsg() {
    var err error
    var msg []byte
    var messageType int
    defer w.Close()
    for {
        messageType, msg, err = w.conn.ReadMessage()
        if err != nil {
            w.SystemErrFunc(err)
            err := w.reconnect()
            if err != nil {
                w.SystemErrFunc(errors.Wrap(err, "[ws] quit message loop."))
                return
            }
        }

        // extend time after msg received
        if w.readDeadLineTime > 0 {
            if err := w.conn.SetReadDeadline(time.Now().Add(w.readDeadLineTime)); err != nil {
                logger.Warn("set readDeadLine error", err)
            }
        }

        switch messageType {
        case websocket.TextMessage:
            w.handleMsg(msg, err)
            if err == nil {
                return
            }
            w.SystemErrFunc(errors.Wrap(err, "[ws] message handler error."))
        case websocket.BinaryMessage:
            msg, err := w.UncompressedFunc(msg)
            if err != nil {
                w.SystemErrFunc(errors.Wrap(err, "[ws] uncompress handler error."))
                return
            }
            w.handleMsg(msg, err)
            if err != nil {
                w.SystemErrFunc(errors.Wrap(err, "[ws] uncompress message handler error."))
            }
        case websocket.CloseAbnormalClosure:
            w.SystemErrFunc(errors.Wrap(fmt.Errorf("%s", string(msg)), "[ws] abnormal Close message"))
        case websocket.CloseMessage:
            w.SystemErrFunc(errors.Wrap(fmt.Errorf("%s", string(msg)), "[ws] Close message"))
        case websocket.CloseGoingAway:
            w.SystemErrFunc(errors.Wrap(fmt.Errorf("%s", string(msg)), "[ws] goaway message"))
        case websocket.PingMessage:
            logger.Info("[ws] receive ping", string(msg))
        case websocket.PongMessage:
            logger.Info("[ws] receive pong", string(msg))
        }
    }
}

func (w *WssClient) handleMsg(msg []byte, err error) {
    switch w.mode {
    case EventCallback:
        err = w.MsgFunc(msg)
    case EventChannel:
        w.channel <- msg
    }
}

func (w *WssClient) WriteMessage(messageType int, msg []byte) error {
    return w.conn.WriteMessage(messageType, msg)
}

func (w *WssClient) WriteControl(messageType int, msg []byte, deadline time.Time) error {
    return w.conn.WriteControl(messageType, msg, deadline)
}

func (w *WssClient) reconnect() (err error) {
    if w.conn != nil {
        err = w.conn.Close()
        if err != nil {
            w.SystemErrFunc(errors.Wrapf(err, "[ws] [%s] Close websocket error", w.wssUrl))
            w.conn = nil
        }
    }
    atomic.StoreUint32(&w.connected, NotConnected)

    ticker := time.NewTicker(w.reconnectInterval)
    defer ticker.Stop()

    for cnt := 1; cnt <= w.reconnectCount; cnt++ {
        select {
        case _ = <- ticker.C:
            err = w.Connect()
            if err != nil {
                w.SystemErrFunc(errors.Wrap(err, fmt.Sprintf("[ws] websocket reconnect fail, retry[%d]", cnt)))
                continue
            }
            logger.Info("[ws] retry", cnt)
            break
        }
    }
    return
}

func (w *WssClient) dump(resp *http.Response, body bool) {
    if w.isDump {
        DumpResponse(resp, body)
    }
}

