package websocket

import (
	"github.com/Shanghai-Lunara/pkg/zaplogger"
	"time"
)

func NewClientWithReconnect(opt *Option) {
	defer opt.Cancel()
	for {
		switch opt.Status {
		case OptionClosed:
			return
		case OptionActive, OptionInActive:
			client, err := NewClient(opt)
			if err != nil {
				zaplogger.Sugar().Error(err)
				if ok := opt.Next(); !ok {
					return
				}
				retryWait(opt.RetryDuration)
				continue
			}
			opt.ChangeStatus(OptionActive)
			if err := opt.Prepare(); err != nil {
				return
			}
			select {
			case <-opt.Done():
				return
			case <-client.Context.Done():
				if ok := opt.Next(); !ok {
					return
				}
				retryWait(opt.RetryDuration)
			}
		}
	}
}

func retryWait(sleep int64) {
	if sleep == 0 {
		return
	}
	time.Sleep(time.Millisecond * time.Duration(sleep))
}