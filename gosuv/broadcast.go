package gosuv

import (
	"errors"
	"fmt"
	"github.com/glycerine/rbuf"
	"github.com/wfxiang08/cyutils/utils/atomic2"
	"github.com/wfxiang08/cyutils/utils/log"
	"sync"
	"time"
)

// The new broadcast
type streamWriter struct {
	wc     *chanStrWriter
	stream string
}

type WriteBroadcaster struct {
	sync.Mutex
	buf     *rbuf.FixedSizeRingBuf
	writers map[*streamWriter]bool
	closed  atomic2.Bool
}

func NewWriteBroadcaster(size int) *WriteBroadcaster {
	if size <= 0 {
		size = 10 * 1024
	}
	bc := &WriteBroadcaster{
		writers: make(map[*streamWriter]bool),
		buf:     rbuf.NewFixedSizeRingBuf(size),
	}
	bc.closed.Set(false)

	return bc
}

// 订阅消息到"chan string"中
func (w *WriteBroadcaster) addEventSub(c chan string) string {

	// sChan 什么时候关闭呢?
	// 在往Client写数据时，如果出现错误，那么sChan就会关闭
	name := fmt.Sprintf("%d", time.Now().UnixNano())
	sChan := w.NewChanString(name)

	go func() {
		for msg := range sChan {
			c <- msg
		}
	}()

	return name

}

func (w *WriteBroadcaster) Closed() bool {
	return w.closed.Get()
}

//
// 创建一个 ChanString, 能通过 chan 将 WriteBroadcaster 中的数据导出
//
func (w *WriteBroadcaster) NewChanString(name string) chan string {

	// 1. 创建一个: ChanStrWriter
	wr := NewChanStrWriter(name)
	if w.closed.Get() {
		wr.Close()
		return nil
	}

	// 2. 注册到 writers中
	w.Lock()
	sw := &streamWriter{wc: wr, stream: name}
	w.writers[sw] = true
	w.Unlock()

	// 3. 第一件事情就是写入历史消息
	wr.Write(w.buf.Bytes())
	return wr.C
}

func (w *WriteBroadcaster) Bytes() []byte {
	return w.buf.Bytes()
}

func (w *WriteBroadcaster) PostEvent(event string) (n int, err error) {
	return w.Write([]byte(event))
}

func (w *WriteBroadcaster) Write(p []byte) (n int, err error) {
	// 不同的线程都会访问Write函数？
	w.buf.WriteAndMaybeOverwriteOldestData(p)

	w.Lock()
	for sw, _ := range w.writers {
		// 同步写数据
		if _, err := sw.wc.Write(p); err != nil {
			log.Warnf("Broadcase write error: %s, %s", sw.stream, err.Error())

			// 出错了，就直接关闭Writer
			sw.wc.Close()
			delete(w.writers, sw)
		}
	}
	w.Unlock()
	return len(p), nil
}

func (w *WriteBroadcaster) CloseWriter(name string) {
	w.Lock()
	defer w.Unlock()

	for sw := range w.writers {
		if sw.stream == name {
			// 关闭了，是否需要删除呢?
			sw.wc.Close()
			delete(w.writers, sw)
			break
		}
	}
}

func (w *WriteBroadcaster) CloseWriters() error {
	w.Lock()
	defer w.Unlock()

	// 关闭所有的Writers
	for sw := range w.writers {
		sw.wc.Close()
	}

	// Reset状态
	w.writers = make(map[*streamWriter]bool)
	w.closed.Set(false)
	return nil
}

// chan string writer
type chanStrWriter struct {
	C      chan string
	closed atomic2.Bool
	start  time.Time
	name   string
}

func (c *chanStrWriter) Write(data []byte) (n int, err error) {

	// 如果关闭，则不在可写
	if c.closed.Get() {
		return 0, errors.New("chan writer closed")
	}

	// 将数据写入chan
	// 可能会阻塞?
	select {
	case c.C <- string(data):
	case <-time.After(100 * time.Millisecond):
		log.Printf("chanStrWriter is full, connection failed")
		return 0, errors.New("Chan Write failed")
	}

	return len(data), nil
}

func (c *chanStrWriter) Close() error {

	// 首次设置为true时，关闭 channel
	// https://golang.org/pkg/sync/atomic/#CompareAndSwapInt64
	if c.closed.CompareAndSwap(false, true) {
		// log.Printf("<---- CLOSE ChanStrWriter: %s, Alive time: %ds ", c.name, int(time.Now().Sub(c.start)/time.Second))
		close(c.C)
	}
	return nil
}

func NewChanStrWriter(name string) *chanStrWriter {
	writer := &chanStrWriter{
		C:     make(chan string, 100), // 正常情况下这个buffer足够了
		start: time.Now(),
		name:  name,
	}

	// log.Printf("----> OPEN ChanStrWriter: %s", writer.name)
	writer.closed.Set(false)
	return writer
}

// quick loss writer
type QuickLossBroadcastWriter struct {
	*WriteBroadcaster
	bufC   chan string
	closed atomic2.Bool
}

func (w *QuickLossBroadcastWriter) Write(buf []byte) (int, error) {
	// 最多缓存一定数量的消息，如果过多，则扔掉
	msg := string(buf)

	select {
	case w.bufC <- msg:
	default:
		log.Printf("BufC is full, drop messages")
	}
	return len(buf), nil
}

func (w *QuickLossBroadcastWriter) Close() error {
	if w.closed.CompareAndSwap(false, true) {
		// 关闭buffer chan
		close(w.bufC)
		// 关闭writers
		w.WriteBroadcaster.CloseWriters()
	}
	return nil
}

func (w *QuickLossBroadcastWriter) drain() {
	// 将 bufC中的数据写入 WriteBroadcaster
	for data := range w.bufC {
		w.WriteBroadcaster.Write([]byte(data))
	}
}

func NewQuickLossBroadcastWriter(size int) *QuickLossBroadcastWriter {
	qlw := &QuickLossBroadcastWriter{
		WriteBroadcaster: NewWriteBroadcaster(size),
		// bufC做一个buffer, 如果：WriteBroadcaster 写得慢，或者出错了，也可以缓解一个
		bufC: make(chan string, 200),
	}
	qlw.closed.Set(false)

	// 从buffer中读取数据，并写入Writer中
	go qlw.drain()
	return qlw
}
