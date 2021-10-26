package go_wss

import (
    "bytes"
    "compress/gzip"
    "errors"
    "fmt"
    logger "github.com/sotyou/go_logger"
    "io/ioutil"
    "net/http"
    "time"
)

const (
    NotConnected = iota
    Connected
)

const (
    EventCallback = iota
    EventChannel
)

var (
    ErrWssNotConnected = errors.New("wss not connected")

    ErrChannelDuplicateSubscribe = errors.New("cannot subscribe to a same channel twice")

    ErrMessageChannelEmpty = errors.New("empty message channel")

    defaultSubCount = 10

    defaultBufferSize = 100

    defaultRetry = 1

    defaultUncompressedFunc = func(data []byte) ([]byte, error) {
        r, err := gzip.NewReader(bytes.NewReader(data))
        if err != nil {
            return nil, err
        }
        return ioutil.ReadAll(r)
    }

    defaultSystemErrorFunc = func(err error) {
        logger.Error("[ws] system error", err)
    }

    defaultMsgFunc = func(msg []byte) error {
        logger.Info("[ws] receive message", string(msg))
        return nil
    }

    defaultAfterConnectedFunc = func() error {
        logger.Info("[ws] connect success.", time.Now().Format("2006-01-02 15:04:05"))
        return nil
    }

    defaultSubFunc = func(ch string) string {
        return fmt.Sprintf(`%s`, ch)
    }

    defaultUnsubFunc = func(ch string) string {
        return fmt.Sprintf(`%s`, ch)
    }

    defaultWss = New()
)

func Run(wssUrl string, header http.Header) (msgChan chan []byte, err error) {
    channel := make(chan []byte, defaultBufferSize)
    err = defaultWss.Config(WithChannel(channel), WithWsUrl(wssUrl), WithHeader(header)).Connect()
    go defaultWss.ReceiveMsg()
    return channel, err
}

func Stop() {
    defaultWss.Close()
}

func RunNew(opts ...WssClientOpt) (wss *WssClient, msgChan chan []byte, err error) {
    wss = New(opts...)
    channel := make(chan []byte, defaultBufferSize)
    err = wss.Connect()
    go wss.ReceiveMsg()
    return wss, channel, err
}
