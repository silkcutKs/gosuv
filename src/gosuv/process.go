package gosuv

import (
	"fmt"
	"github.com/codeskyblue/kexec"
	log "github.com/wfxiang08/cyutils/utils/rolling_log"
	"io"
	"os"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"
)

var ProcessWg sync.WaitGroup

type Process struct {
	*FSM        `json:"-"`                           // Process内部状态
	ProcessName string                    `json:"process_name"`
	Program     *ProgramEx                `json:"program"`
	Index       int                       `json:"index"`
	cmd         *kexec.KCommand                      // 运行的命令
	Output      *QuickLossBroadcastWriter `json:"-"` // 输出？
	stopC       chan syscall.Signal
	retryLeft   int
	Status      string `json:"status"`
	mu          sync.Mutex

	stopWg      sync.WaitGroup
}

// Process --> KCommand(但是还没有运行起来)
func (p *Process) buildCommand() *kexec.KCommand {
	// 1. 运行命令行
	commandStr := p.Program.Command

	var tmpEnvs []string
	commandStr, tmpEnvs = PreprocessCommand(commandStr)

	// Hack: 只对php进程添加额外的id参数
	if strings.Contains(commandStr, ".php") {
		commandStr = fmt.Sprintf("%s --id=%d", commandStr, p.Index)
	}
	cmd := kexec.CommandString(commandStr)

	// cmd将输出同时写到3个文件中
	// 标准输出/err最终都输出到 Output中
	// 如何处理日志:
	// 所有的输出日志都写入
	// fout: 文件
	// Output: 可以用于实时数据反馈给Client
	// Stdout/Stderr 似乎没有太多的作用
	//log.Printf("buildCommand: %v", p.Program.Merger)
	//log.Printf("buildCommand: %v", p.Program.Merger.NewWriter(p.Index))
	cmd.Stdout = io.MultiWriter(p.Output, p.Program.Merger.NewWriter(p.Index))
	cmd.Stderr = io.MultiWriter(p.Output, p.Program.Merger.NewWriter(p.Index))

	// config environ
	cmd.Env = os.Environ() // inherit current vars
	environ := map[string]string{}
	if p.Program.User != "" {
		if !IsRoot() {
			// 不是root时不能切换用户
			log.Warnf("detect not root, can not switch user")
		} else if err := cmd.SetUser(p.Program.User); err != nil {
			log.Warnf("[%s] chusr to %s failed, %v", p.Program.Name, p.Program.User, err)
		} else {
			var homeDir string
			switch runtime.GOOS {
			case "linux":
				homeDir = "/home/" + p.Program.User // FIXME(ssx): maybe there is a better way
			case "darwin":
				homeDir = "/Users/" + p.Program.User
			}
			cmd.Env = append(cmd.Env, "HOME=" + homeDir, "USER=" + p.Program.User)
			environ["HOME"] = homeDir
			environ["USER"] = p.Program.User
		}
	}

	// 来自程序参数的命令
	if len(tmpEnvs) > 0 {
		cmd.Env = append(cmd.Env, tmpEnvs...)
	}

	cmd.Env = append(cmd.Env, p.Program.Environ...)
	mapping := func(key string) string {
		val := os.Getenv(key)
		if val != "" {
			return val
		}
		return environ[key]
	}

	// 根据环境变量
	cmd.Dir = os.Expand(p.Program.Dir, mapping)
	if strings.HasPrefix(cmd.Dir, "~") {
		cmd.Dir = mapping("HOME") + cmd.Dir[1:]
	}
	log.Infof("Program: [%s], DIR: %s\n", p.Program.Name, cmd.Dir)
	return cmd
}

// 只运行在 startCommand内部的独立的go func中
func (p *Process) waitNextRetry() {

	// 彻底失败，标记为Fatal
	if p.retryLeft <= 0 {
		p.retryLeft = p.Program.StartRetries
		p.SetState(Fatal)
		ProcessWg.Done()
		p.stopWg.Done()
		return

	} else {
		p.SetState(RetryWait)

		// 等进程的状态稳定之后，在处理WaitGroup的通知；以免状态机不正常工作；例如: start时发现进程并没有处于stop的状态，然后就不会重启了
		ProcessWg.Done()
		p.stopWg.Done()
	}

	// 开始Command
	// 等待2s, 如果没有stop, 则重新开始
	p.retryLeft -= 1
	select {
	case <-time.After(2 * time.Second):
		p.startCommand()
	case <-p.stopC:

		p.stopCommand()
	}
}

func (p *Process) stopCommand() {
	p.mu.Lock()
	defer p.mu.Unlock()
	// 不要使用 defer 了，业务逻辑需要 精确控制 State和WaitGroup的顺序关系
	// defer p.SetState(Stopped)

	if p.cmd == nil {
		p.SetState(Stopped)
		return
	}

	// 准备停止
	p.SetState(Stopping)
	if p.cmd.Process != nil {
		// 首先发送信号：SIGTERM
		// kill -15 pid
		io.WriteString(p.cmd.Stderr, fmt.Sprintf("GOSUV: Kill by SIGTERM: %s\n", p.ProcessName))
		p.cmd.Process.Signal(syscall.SIGTERM)
	}

	// 等待程序的正常退出：StopTimeout
	// 或者直接通过 kill -9 pid 直接杀死
	select {
	case <-GoFunc(p.cmd.Wait):
	// 等待正常返回
		log.Printf("Program quit normally: %s", p.ProcessName)
	case <-time.After(time.Duration(p.Program.StopTimeout) * time.Second):
	// 只要没有正式"开杀", StopTimeout还是可以调整的
	// 如果超过: StopTimeout, 则直接kill -9 杀死
	// StopTimeout 这个很重要， 对于某些耗时操作，这个需要等待
		log.Printf("Program terminate all: %s", p.ProcessName)
		io.WriteString(p.cmd.Stderr, fmt.Sprintf("GOSUV: Kill by SIGKILL: %s\n", p.ProcessName))
		p.cmd.Terminate(syscall.SIGKILL) // cleanup
	}

	// 等待结束
	err := p.cmd.Wait() // This is OK, because Signal KILL will definitely work

	// Stopped状态必须在stopWg.Done()之前设置
	p.SetState(Stopped)

	// 结束
	ProcessWg.Done()
	p.stopWg.Done()

	if err == nil {
		io.WriteString(p.cmd.Stderr, fmt.Sprintf("GOSUV: Exit success: %s\n", p.ProcessName))
	} else {
		io.WriteString(p.cmd.Stderr, fmt.Sprintf("GOSUV: exit %s, %v\n", p.ProcessName, err.Error()))
	}
	p.cmd = nil
}

func (p *Process) IsRunning() bool {
	return p.State() == Running || p.State() == RetryWait
}

func (p *Process) startCommand() {
	log.Printf("START %s --> %s", p.ProcessName, p.Program.Command)
	p.cmd = p.buildCommand()
	io.WriteString(p.cmd.Stderr, fmt.Sprintf("GOSUV: startCommand: %s\n", p.ProcessName))

	p.SetState(Running)

	// 启动程序（异步）
	if err := p.cmd.Start(); err != nil {
		// 如果启动报错，那就没有办法再尝试，直接Fatal
		log.Warnf("Program %s start failed: %v", p.ProcessName, err)
		p.SetState(Fatal)
		return
	}

	go func() {
		ProcessWg.Add(1)
		p.stopWg.Add(1)

		errC := GoFunc(p.cmd.Wait)
		startTime := time.Now()
		// 进程启动之后，我们就等待它结束
		// 1. 自动结束
		//    1.1 直接放弃(过早退出)
		//    1.2 重试N次（这个N最好大一点), 不鲁棒的代码最好设置一个较大的N
		// 2. 通知结束
		select {
		case err := <-errC:
		// 结束
			elapsed := time.Since(startTime)
			log.Printf("Program finished: %s, time used %v", p.ProcessName, elapsed)

			if elapsed < time.Duration(p.Program.StartSeconds) * time.Second {
				// 第一次很快就退出，则设置为Fatal
				if p.retryLeft == p.Program.StartRetries {
					p.SetState(Fatal)
					ProcessWg.Done()
					p.stopWg.Done()
					log.Printf("Program exit too quick: %s, status -> fatal", p.ProcessName)
					return
				}
			} else if elapsed >= 600 * time.Second {
				// 如果程序跑了10分钟以上，则不减少retry的次数
				p.retryLeft++
			}

		// 失败重试
			io.WriteString(p.cmd.Stderr, fmt.Sprintf("GOSUV: startCommand failed: %s, Retry: %d, Last Error: %v\n", p.ProcessName, p.retryLeft, err))
			p.cmd = nil

			p.waitNextRetry()
		case <-p.stopC:
			log.Printf("Recv stop command：%s", p.ProcessName)
			p.stopCommand()
		}
	}()
}
