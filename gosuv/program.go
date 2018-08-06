package gosuv

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/kennygrant/sanitize"
	log "github.com/wfxiang08/cyutils/utils/log"
	"io"
	"os"
	"path"
	"syscall"
	"time"
)

type Program struct {
	ID           uint     `yaml:"-" json:"-" gorm:"primary_key"`
	Host         string   `yaml:"_" json:"host" gorm:"size:100" gorm:"index:host_name"`    // 名字
	Name         string   `yaml:"name" json:"name" gorm:"size:100" gorm:"index:host_name"` // 名字
	Command      string   `yaml:"command" json:"command" gorm:"size:500"`                  // 命令
	Environ      []string `yaml:"environ" json:"environ" sql:"-"`                          // 环境变量
	EnvironDb    string   `yaml:"-" json:"-" gorm:"size:2000"`                             // 环境变量
	Dir          string   `yaml:"directory" json:"directory" gorm:"size:255"`              // 当前工作目录
	StartAuto    bool     `yaml:"start_auto" json:"start_auto"`                            // 是否自动重启
	StartRetries int      `yaml:"start_retries" json:"start_retries"`
	StartSeconds int      `yaml:"start_seconds,omitempty" json:"start_seconds"`
	StopTimeout  int      `yaml:"stop_timeout,omitempty" json:"stop_timeout"`
	User         string   `yaml:"user,omitempty" json:"user" gorm:"size:40"` // 运行用户
	ProcessNum   int      `yaml:"process_num,omitempty" json:"process_num"`  // 同时运行进程数

	// 脚本作者
	Author string `yaml:"author,omitempty" json:"author" gorm:"size:40"`
}

// 如何控制并发数呢?
// 序列化参考: http://ghodss.com/2014/the-right-way-to-handle-yaml-in-golang/
//
type ProgramEx struct {
	*Program
	Status     FSMState                  `yaml:"-" json:"status"`
	RunningNum int                       `yaml:"-" json:"running_num"`
	Processes  []*Process                `yaml:"-" json:"-"`
	Output     *QuickLossBroadcastWriter `yaml:"-" json:"-"`
	OutputFile io.Writer                 `yaml:"-" json:"-"` // 输出文件
	Merger     *MergeWriter              `yaml:"-" json:"-"`
}

func (p *Program) String() string {
	return fmt.Sprintf("Program: %s@%s, ProcessNum: %d, Author: %s", p.Name, p.Host, p.ProcessNum, p.Author)
}
func (p *Program) Decode() {
	var environ []string
	if err := json.Unmarshal([]byte(p.EnvironDb), &environ); err != nil {
		p.Environ = nil
	} else {
		p.Environ = environ
	}
}
func (p *Program) Encode() {
	environDb, _ := json.Marshal(p.Environ)
	p.EnvironDb = string(environDb)
}

func (p *ProgramEx) InitProgram(logDir string) {
	log.Printf("InitProgram: %s, log: %s", p.Program.String(), logDir)

	// 1. 创建日志输出
	if len(logDir) > 0 {
		if !IsDir(logDir) {
			os.MkdirAll(logDir, 0755)
		}
		outPath := path.Join(logDir, sanitize.Name(p.Name)+".log")

		var err error
		// 每天Rotate
		p.OutputFile, err = log.NewRollingFile(outPath, 3)

		if err != nil {
			log.WarnError(err, "Create stdout log failed:")
		}
	}

	// 2. 内存的Merger(合并多个Process的输出) 10k, 太大了也不好
	outputBufferSize := 10 * 1024
	p.Output = NewQuickLossBroadcastWriter(outputBufferSize)
	if p.OutputFile != nil {
		p.Merger = NewMergeWriter(io.MultiWriter(p.Output, p.OutputFile))
	} else {
		p.Merger = NewMergeWriter(p.Output)
	}

	// 3. 创建多个进程
	p.Processes = nil
	p.Processes = make([]*Process, 0, p.ProcessNum)
	for i := 0; i < p.ProcessNum; i++ {
		p.Processes = append(p.Processes, p.NewProcess(i))
		log.Printf("New Process at index: %d", i)

		// 如果是自动启动，则启动
		if p.StartAuto {
			p.Processes[i].Operate(StartEvent)
		}
	}
}

func (p *ProgramEx) UpdateState() {
	runningNum := 0
	for i := 0; i < len(p.Processes); i++ {
		if p.Processes[i] == nil {
			log.Printf("Process is nil at: %d", i)
			continue
		}
		if p.Processes[i] != nil && p.Processes[i].state == Running {
			runningNum++
		}
	}
	p.RunningNum = runningNum
	if runningNum > 0 {
		p.Status = Running
	} else {
		p.Status = Stopped
	}
}

func (p *ProgramEx) IndexName(index int) string {
	// 最多支持999个并发进程
	return fmt.Sprintf("%s_%03d", p.Name, index)
}

//
// 如何验证Program是否有效
//
func (p *Program) Check() error {
	if p.Name == "" {
		return errors.New("Program name empty")
	}
	if p.Command == "" {
		return errors.New("Program command empty")
	}

	return nil
}

func (p *ProgramEx) UpdateProgram(newProgram *Program) bool {
	// 除了进程数，其他参数暂不作为明显的区分标志
	p.Command = newProgram.Command
	p.Environ = newProgram.Environ
	p.Dir = newProgram.Dir

	p.StartAuto = newProgram.StartAuto
	p.StartRetries = newProgram.StartRetries
	p.StartSeconds = newProgram.StartSeconds

	// 运行用户
	// 所有者
	if len(newProgram.Author) > 0 {
		p.Author = newProgram.Author
	}
	if newProgram.StopTimeout >= 3 {
		p.StopTimeout = newProgram.StopTimeout
	}
	// 这个如何修改呢?
	p.User = newProgram.User

	log.Printf("UpdateProgram: %s, ProcessNum: %d --> %d", p.Name, p.ProcessNum, newProgram.ProcessNum)

	if p.ProcessNum == newProgram.ProcessNum {
		return false
	}

	// 广播update Event
	// s.broadcastEvent(newProgram.Name + " update")
	if p.ProcessNum <= newProgram.ProcessNum {
		// 添加新的进程
		// TODO: 如果command被修改了，则可能有一致性的问题
		for i := p.ProcessNum; i < newProgram.ProcessNum; i++ {
			newProc := p.NewProcess(i)
			p.Processes = append(p.Processes, newProc)

			// 如果是自动启动，则启动
			if p.StartAuto {
				newProc.Operate(StartEvent)
			}
		}
	} else {
		for i := p.ProcessNum - 1; i >= newProgram.ProcessNum; i-- {
			p.stopAndWait(p.Processes[i])

			p.Processes[i] = nil
			p.Processes = p.Processes[0:i] // 删除最后一个元素

		}
	}

	// 最终状态
	p.ProcessNum = newProgram.ProcessNum
	return true

}

func (p *ProgramEx) StopAndWaitAll() bool {
	if p.ProcessNum == 0 {
		return false
	}
	result := false
	for i := p.ProcessNum - 1; i >= 0; i-- {
		if p.stopAndWait(p.Processes[i]) {
			result = true
		}

		p.Processes[i] = nil
		p.Processes = p.Processes[0:i] // 删除最后一个元素
	}
	return result
}

/**
 *
 */
func (p *ProgramEx) stopAndWait(process *Process) bool {
	log.Printf("Stop process: %s ...", process.ProcessName)

	if !process.IsRunning() {
		return false
	}

	// 停止Process
	process.Operate(StopEvent)

	c := make(chan string, 0)
	name := gEventPub.addEventSub(c)
	for {
		select {
		case <-c:
			// 等待事件
			if !process.IsRunning() {
				// 如果不再允许，那么当前Writer的使命也完成了
				gEventPub.CloseWriter(name)
				return true
			}
		}
	}
}

func (p *ProgramEx) StartOne(index int) {
	if index < len(p.Processes) {
		if !p.Processes[index].IsRunning() {
			p.Processes[index].Operate(StartEvent)
		}
	}
}

func (p *ProgramEx) StartAll(opUser string) {
	log.Printf("操作: %s start all: %s --> %d", opUser, p.Name, len(p.Processes))
	p.Merger.WriteStrLine(fmt.Sprintf("操作: %s start all: %s --> %d\n", opUser, p.Name, len(p.Processes)))

	for index := 0; index < len(p.Processes); index++ {
		if !p.Processes[index].IsRunning() {
			p.Processes[index].Operate(StartEvent)
		}
	}
}

func (p *ProgramEx) StopOne(index int) {
	if index < len(p.Processes) {
		p.Processes[index].Operate(StopEvent)
	}
}

func (p *ProgramEx) StopAll(opUser string) {
	// 记录操作日志：
	log.Printf("操作: %s stop all: %s --> %d", opUser, p.Name, len(p.Processes))
	p.Merger.WriteStrLine(fmt.Sprintf("操作: %s stop all: %s --> %d\n", opUser, p.Name, len(p.Processes)))

	for index := 0; index < len(p.Processes); index++ {
		p.Processes[index].Operate(StopEvent)
	}
}

func (p *ProgramEx) RestartAll() {

	log.Printf("操作: admin restart all: %s --> %d", p.Name, len(p.Processes))
	p.Merger.WriteStrLine(fmt.Sprintf("操作: admin restart all: %s --> %d\n", p.Name, len(p.Processes)))

	for index := 0; index < len(p.Processes); index++ {
		p.Processes[index].Operate(RestartEvent)
	}
}

//
// 从Program创建一个Process
//
func (p *ProgramEx) NewProcess(index int) *Process {
	outputBufferSize := 10 * 1024 // 10k
	pr := &Process{
		FSM:         NewFSM(Stopped),
		ProcessName: p.IndexName(index),
		Program:     p,
		Index:       index,
		stopC:       make(chan syscall.Signal),
		retryLeft:   p.StartRetries,
		Status:      string(Stopped),
		Output:      NewQuickLossBroadcastWriter(outputBufferSize),
	}
	pr.StateChange = func(oldState, newState FSMState) {
		// 1. 更新Process的状态
		pr.Status = string(newState)

		// 3. Post Event
		event := fmt.Sprintf("State change[%s] %s -> %s", pr.ProcessName, string(oldState), string(newState))
		gEventPub.PostEvent(event)

	}

	// 设置默认的参数
	if pr.Program.StartSeconds <= 0 {
		pr.Program.StartSeconds = 3
	}
	if pr.Program.StopTimeout <= 0 {
		pr.Program.StopTimeout = 5
	}

	pr.AddHandler(Stopped, StartEvent, func() {
		// 重新开始retry
		pr.retryLeft = pr.Program.StartRetries
		pr.startCommand()
	})
	pr.AddHandler(Fatal, StartEvent, pr.startCommand)

	pr.AddHandler(Running, StopEvent, func() {
		select {
		case pr.stopC <- syscall.SIGTERM:
		// 如果stopC有太多的积压，则直接放弃
		case <-time.After(200 * time.Millisecond):
		}

	}).AddHandler(Running, RestartEvent, func() {
		go func() {
			// 不要做异步操作，直接Block即可
			if pr.IsRunning() {
				pr.Operate(StopEvent)
				// 等待结束
				log.Printf("GOSUV: Wait for process: %s to stop", pr.ProcessName)

				p.Merger.WriteStrLine(fmt.Sprintf("GOSUV: Restart Process: %s waiting stopped\n", pr.ProcessName))

				// 等待程序结束
				// 1. stopWg 必须在Progress的状态设置之后再调用
				// 2. Stopped状态必须在stopWg.Done()之前设置；通过defer调用状态有时候会打乱这种关系
				// 3. WaitGroup也可以作为一个状态传递的工具
				// 4. 重要的状态必须打印日志
				pr.stopWg.Wait()
				p.Merger.WriteStrLine(fmt.Sprintf("GOSUV: Restart Process: %s stopped, State: %s\n",
					pr.ProcessName, pr.State()))

				// 重要状态必须要有Check, 报错机制
				if pr.State() != Stopped {
					p.Merger.WriteStrLine(fmt.Sprintf("GOSUV: WARNING Expected Stopped: %s, but get: %s\n",
						pr.ProcessName, pr.State()))
				}
				pr.Operate(StartEvent)
			}
		}()
	})
	return pr
}

type ProgramSlice []*ProgramEx

func (s ProgramSlice) Len() int {
	return len(s)
}
func (s ProgramSlice) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}
func (s ProgramSlice) Less(i, j int) bool {
	return s[i].Name < s[j].Name
}
