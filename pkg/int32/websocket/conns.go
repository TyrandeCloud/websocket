package websocket

import (
	"context"
	"fmt"
	"github.com/Shanghai-Lunara/pkg/zaplogger"
	"github.com/TyrandeCloud/websocket/pkg/env"
	"github.com/TyrandeCloud/websocket/pkg/int32/websocket/handler"
	"github.com/gorilla/websocket"
	cmap "github.com/nevercase/concurrent-map"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

const (
	ConnectionTimeout = 10
)

var upGrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

func NewConnections(ctx context.Context) *Connections {
	cs := &Connections{
		autoIncr:      0,
		clients:       cmap.New(),
		dashboardChan: make(chan []byte, 1024),
		removedChan:   make(chan int32, 4096),
		printMode: &printMode{
			read:  env.GetWebsocketRead(),
			write: env.GetWebsocketWrite(),
		},
		keepAliveTimeoutInSec: int32(env.GetWebsocketKeepaliveTimeout(ConnectionTimeout)),
		ctx:                   ctx,
	}
	go cs.remove()
	return cs
}

type Connections struct {
	autoIncr              int32
	clients               cmap.ConcurrentMap
	dashboardChan         chan []byte
	removedChan           chan int32
	printMode             *printMode
	keepAliveTimeoutInSec int32
	ctx                   context.Context
}

type printMode struct {
	write bool
	read  bool
}

func (cs *Connections) remove() {
	for {
		select {
		case <-cs.ctx.Done():
			return
		case id, isClose := <-cs.removedChan:
			if !isClose {
				return
			}
			shardKey := shardKey(id)
			cs.clients.SetIfAbsent(shardKey, cmap.New())
			obj, _ := cs.clients.Get(shardKey)
			t := obj.(cmap.ConcurrentMap)
			t.Remove(shardKey)
		}
	}
}

func shardKey(id int32) string {
	return fmt.Sprintf("%d", id)
}

func (cs *Connections) Handler(w http.ResponseWriter, r *http.Request, handler handler.Interface) {
	client, err := cs.newConn(w, r, handler)
	if err != nil {
		zaplogger.Sugar().Error(err)
		return
	}
	shardKey := shardKey(client.id)
	cs.clients.SetIfAbsent(shardKey, cmap.New())
	obj, _ := cs.clients.Get(shardKey)
	t := obj.(cmap.ConcurrentMap)
	t.Set(shardKey, client)
}

func (cs *Connections) SendToAll(data []byte) {
	t := cs.clients.Items()
	var wg sync.WaitGroup
	wg.Add(len(t))
	for _, v := range t {
		go func(in interface{}) {
			obj := in.(cmap.ConcurrentMap)
			t2 := obj.Items()
			for _, v2 := range t2 {
				client := v2.(*conn)
				client.writeChan <- data
			}
			wg.Done()
		}(v)
	}
	wg.Wait()
}

func (cs *Connections) SendToOne(data []byte, id int32) bool {
	key := shardKey(id)
	cs.clients.SetIfAbsent(key, cmap.New())
	t, _ := cs.clients.Get(key)
	obj := t.(cmap.ConcurrentMap)
	if t2, ok := obj.Get(key); ok {
		client := t2.(*conn)
		client.writeChan <- data
		return true
	} else {
		return false
	}
}

func (cs *Connections) newConn(w http.ResponseWriter, r *http.Request, handler handler.Interface) (*conn, error) {
	client, err := upGrader.Upgrade(w, r, nil)
	if err != nil {
		zaplogger.Sugar().Error(err)
		return nil, err
	}
	ctx, cancel := context.WithCancel(cs.ctx)
	c := &conn{
		id:                    atomic.AddInt32(&cs.autoIncr, 1),
		conn:                  client,
		handler:               handler,
		writeChan:             make(chan []byte, 4096),
		lastPingTime:          time.Now(),
		keepAliveTimeoutInSec: cs.keepAliveTimeoutInSec,
		closeOnce:             sync.Once{},
		removedChan:           cs.removedChan,
		printMode:             cs.printMode,
		ctx:                   ctx,
		cancel:                cancel,
	}
	if handler.Id() == 0 {
		handler.RegisterId(c.id)
	} else {
		c.id = handler.Id()
	}
	handler.RegisterConnWriteChan(c.writeChan)
	handler.RegisterConnPing(c.ping)
	handler.RegisterConnClose(c.close)
	handler.Run()
	go c.keepAlive()
	go c.readPump()
	go c.writePump()
	return c, nil
}

type conn struct {
	id                    int32
	conn                  *websocket.Conn
	handler               handler.Interface
	writeChan             chan []byte
	lastPingTime          time.Time
	keepAliveTimeoutInSec int32
	closeOnce             sync.Once
	removedChan           chan<- int32
	printMode             *printMode
	ctx                   context.Context
	cancel                context.CancelFunc
}

func (c *conn) keepAlive() {
	defer c.close()
	tick := time.NewTicker(time.Second * time.Duration(c.keepAliveTimeoutInSec/2+1))
	defer tick.Stop()
	for {
		select {
		case <-tick.C:
			if time.Now().Sub(c.lastPingTime) > time.Second*time.Duration(c.keepAliveTimeoutInSec) {
				zaplogger.Sugar().Info("keepAlive timeout")
				return
			}
		case <-c.ctx.Done():
			return
		}
	}
}

func (c *conn) ping() {
	c.lastPingTime = time.Now()
}

func (c *conn) close() {
	c.closeOnce.Do(func() {
		go c.handler.Close()
		c.cancel()
		c.removedChan <- c.id
		if err := c.conn.Close(); err != nil {
			zaplogger.Sugar().Error(err)
		}
	})
}

func (c *conn) readPump() {
	defer c.close()
	for {
		_, data, err := c.conn.ReadMessage()
		//klog.V(3).Infof("messageType: %d message-string: %s\n", messageType, string(data))
		if err != nil {
			zaplogger.Sugar().Error(err)
			return
		}
		if c.printMode.read == env.Show {
			zaplogger.Sugar().Infow("conn read", "id", c.id, "data", string(data))
		}
		res, err := c.handler.Handler(data)
		if err != nil {
			zaplogger.Sugar().Error(err)
			return
		}
		if len(res) == 0 {
			zaplogger.Sugar().Debug("ws conn handler res len 0")
			continue
		}
		c.writeChan <- res
	}
}

func (c *conn) writePump() {
	defer c.close()
	for {
		select {
		case msg, isClose := <-c.writeChan:
			if !isClose {
				return
			}
			if c.printMode.write == env.Show {
				zaplogger.Sugar().Infow("conn write", "id", c.id, "data", string(msg))
			}
			if err := c.conn.WriteMessage(websocket.BinaryMessage, msg); err != nil {
				zaplogger.Sugar().Error(err)
				return
			}
		case <-c.ctx.Done():
			return
		}
	}
}
