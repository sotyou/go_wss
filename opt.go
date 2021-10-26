package go_wss

import (
    "net/http"
    "time"
)

type WssClientOpt func(c *WssClient)

func WithChannel(channel chan []byte) WssClientOpt {
    return func(o *WssClient) {
        o.channel = channel
    }
}

func WithWsUrl(url string) WssClientOpt {
    return func(o *WssClient) {
        o.wssUrl = url
    }
}

func WithHeader(header http.Header) WssClientOpt {
    return func(o *WssClient) {
        o.header = header
    }
}

func WithIsAutoReconnect(is bool) WssClientOpt {
    return func(o *WssClient) {
        o.isAutoReconnect = is
    }
}

func WithIsDump(is bool) WssClientOpt {
    return func(o *WssClient) {
        o.isDump = is
    }
}

func WithReconnectCount(c int) WssClientOpt {
    return func(o *WssClient) {
        o.reconnectCount = c
    }
}

func WithReconnectInterval(i time.Duration) WssClientOpt {
    return func(o *WssClient) {
        o.reconnectInterval = i
    }
}

func WithUncompressFunc(fn func([]byte) ([]byte, error)) WssClientOpt {
    return func(o *WssClient) {
        o.UncompressedFunc = fn
    }
}

func WithSystemErrorFunc(fn func(error)) WssClientOpt {
    return func(o *WssClient) {
        o.SystemErrFunc = fn
    }
}

func WithMessageFunc(fn func(msg []byte) error) WssClientOpt {
    return func(o *WssClient) {
        o.MsgFunc = fn
    }
}

func WithAfterConnectedFunc(fn func() error) WssClientOpt {
    return func(o *WssClient) {
        o.AfterConnectedFunc = fn
    }
}

func WithSubFnc(fn func(string) string) WssClientOpt {
    return func(o *WssClient) {
        o.SubFunc = fn
    }
}

func WithUnsubFnc(fn func(string) string) WssClientOpt {
    return func(o *WssClient) {
        o.UnsubFunc = fn
    }
}

func WithSubs(fn func() []string) WssClientOpt {
    return func(o *WssClient) {
        for _, v := range fn() {
            o.channels[v] = struct{}{}
        }
    }
}
