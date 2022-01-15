package websocket

func NewClientWithReconnect(opt *Option) {
	defer opt.Cancel()
	for {
		switch opt.Status {
		case OptionClosed:
			return
		case OptionActive, OptionInActive:
			client, err := NewClient(opt)
			if err != nil {
				if ok := opt.Next(); !ok {
					return
				}
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
			}
		}
	}
}
