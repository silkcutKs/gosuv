package gosuv

import (
	log "github.com/wfxiang08/cyutils/utils/log"
	"os/user"
	"sync"
)

// 定义一个Process的有限状态机
type FSMState string
type FSMEvent string
type FSMHandler func()

type FSM struct {
	mu          sync.Mutex
	state       FSMState // 当前状态

                         // 这个是固定的， 不需要同步处理
	handlers    map[FSMState]map[FSMEvent]FSMHandler
	StateChange func(oldState, newState FSMState)
}

func (f *FSM) AddHandler(state FSMState, event FSMEvent, hdlr FSMHandler) *FSM {
	_, ok := f.handlers[state]
	if !ok {
		f.handlers[state] = make(map[FSMEvent]FSMHandler)
	}

	// 一个状态下一个event只能有一个处理方法
	if _, ok = f.handlers[state][event]; ok {
		log.Panicf("set twice for state(%s) event(%s)", state, event)
	}
	f.handlers[state][event] = hdlr
	return f
}

func (f *FSM) State() FSMState {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.state
}

func (f *FSM) SetState(newState FSMState) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.StateChange != nil {
		// 回调状态
		f.StateChange(f.state, newState)
	}
	// 修改状态
	f.state = newState
}

func (f *FSM) Operate(event FSMEvent) FSMState {
	eventMap := f.handlers[f.State()]
	if eventMap == nil {
		return f.State()
	}
	// 如果有callback, 则回调
	if fn, ok := eventMap[event]; ok {
		fn()
	} else {
	}
	// 返回当前的State
	return f.State()
}

func NewFSM(initState FSMState) *FSM {
	return &FSM{
		state:    initState,
		handlers: make(map[FSMState]map[FSMEvent]FSMHandler),
	}
}

var (
	Running = FSMState("running")
	Stopped = FSMState("stopped")
	Fatal = FSMState("fatal")
	RetryWait = FSMState("retry wait")
	Stopping = FSMState("stopping")

	StartEvent = FSMEvent("start")
	StopEvent = FSMEvent("stop")
	RestartEvent = FSMEvent("restart")
)

func IsRoot() bool {
	u, err := user.Current()
	return err == nil && u.Username == "root"
}
