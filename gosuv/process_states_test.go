package gosuv

import (
	"testing"
	"container/list"
)

// go test gosuv -v -run "TestProcessStateChange"
func TestProcessStateChange(t *testing.T) {

	m := NewStateMachine()
	m.State = StateRunning

	m.AddAction(ActionStart)
	m.PrintStateActions()
	m.AddAction(ActionStop)
	m.PrintStateActions()
	m.AddAction(ActionStop)
	m.PrintStateActions()
	m.AddAction(ActionStart)
	m.PrintStateActions()
	m.AddAction(ActionStart)
	m.PrintStateActions()

	m.State = StateStopped
	m.list = list.New()
	m.AddAction(ActionStart)
	m.PrintStateActions()
	m.AddAction(ActionStop)
	m.PrintStateActions()
	m.AddAction(ActionStop)
	m.PrintStateActions()
	m.AddAction(ActionStart)
	m.PrintStateActions()
	m.AddAction(ActionStart)
	m.PrintStateActions()
	m.AddAction(ActionStop)
	m.PrintStateActions()
}
