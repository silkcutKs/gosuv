package gosuv

import (
	"container/list"
	"fmt"
	"sync"
)

type ProcessState int
type ProcessAction int

//
const (
	StateStopped ProcessState = iota
	StateFatal                // 除了完毕状态不一样之外，语义相同：都表示停止了
	StateRetryWait
	StateStopping
	StateRunning

	ActionNone ProcessAction = iota
	ActionStop
	ActionStart
)

//
//                                |---> ActionStop --> Stopped
//   StateStopped --> Start --> StateRunning --> StateFatal(StateStopped)
//                                |
//                                |--->  StateRetryWait ---->  StateFatal
//                                                       |-->  StateStop --> Running
//                                                       |-->  StateRunning
//
//

type StateMachine struct {
	list  *list.List
	State ProcessState
	mu    sync.Mutex
}

func NewStateMachine() *StateMachine {
	result := &StateMachine{
		list: list.New(),
	}
	return result
}

func (m *StateMachine) LastAction() ProcessAction {
	if m.list.Len() > 0 {
		val, _ := m.list.Back().Value.(ProcessAction)
		return val
	} else {
		return ActionNone
	}
}


func (m *StateMachine) PopNextAction() ProcessAction {
	if m.list.Len() > 0 {
		val, _ := m.list.Remove(m.list.Front()).(ProcessAction)
		return val
	} else {
		return ActionNone
	}
}

func (m *StateMachine) PrintStateActions() {
	fmt.Printf("State[%s]: ", m.State)
	// Iterate through list and print its contents.
	for e := m.list.Front(); e != nil; e = e.Next() {
		val, _ := e.Value.(ProcessAction)
		fmt.Printf(" --> %s", val)
	}
	fmt.Println()
}

// 添加状态
func (m *StateMachine) AddAction(action ProcessAction) {
	switch m.State {
	case StateStopping:
		fallthrough
	case StateStopped:
		fallthrough
	case StateFatal:

		if action == ActionStop {
			// [start] --> [start, stop] --> []
			if m.LastAction() == ActionStart {
				if m.list.Len() >= 1 {
					// start, stop, start, stop
					m.list.Remove(m.list.Back())
				}
			}
		} else {
			// 添加: ActionStart
			// [] --> start
			// [start stop] --> [start stop start]
			if m.LastAction() != ActionStart {
				// [start stop] --> [start stop start] --> [start]
				// Remove Back
				if m.list.Len() >= 2 {
					m.list.Remove(m.list.Back())
				} else {
					m.list.PushBack(ActionStart)
				}
			}
		}
	case StateRetryWait:
		fallthrough
	case StateRunning:
		// 只接受Stop的指令
		if action == ActionStart {
			// [stop] --> [stop start]
			if m.LastAction() == ActionStop {
				m.list.PushBack(ActionStart)
			}
		} else {
			// [] --> [stop]
			// [stop start] -> [stop start stop] --> [stop]
			if m.list.Len() >= 2 {
				m.list.Remove(m.list.Back())
			} else if m.LastAction() != ActionStop {
				m.list.PushBack(ActionStop)
			}
		}
	}
}

func (a ProcessAction) String() string {
	switch a {
	case ActionStop:
		return "stop"
	case ActionStart:
		return "start"
	}
	return "none"
}

func (s ProcessState) String() string {
	switch s {
	case StateStopped:
		return "stopped"
	case StateFatal:
		return "fatal"
	case StateRetryWait:
		return "retry wait"
	case StateStopping:
		return "stopping"
	case StateRunning:
		return "running"
	}
	return "unknown"
}
