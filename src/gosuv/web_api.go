package gosuv

import (
	"encoding/json"
	"fmt"
	"github.com/flosch/pongo2"
	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
	"github.com/jinzhu/gorm"
	"github.com/wfxiang08/cyutils/utils/atomic2"
	log "github.com/wfxiang08/cyutils/utils/rolling_log"
	"gosuv/gops"
	"net/http"
	"os"
	"os/user"
	"path"
	"strconv"
	"time"
)

//
// 将普通的http 1.1协议升级为websocket协议
//
var upgrader = websocket.Upgrader{}

// 默认的配置文件所在目录
var DefaultConfigDir string
var Assets http.Dir
var Version string

var StaticHandler http.Handler

// 创建SupervisorHandler以及HttpServer
func NewSupervisorHandler(cfg *Configuration, logDir string) (suv *Supervisor, hdlr http.Handler, err error) {

	// log.Printf("Host Info: %s,%s, %s", cfg.Db.DbType, cfg.Db.DbDsn, cfg.Host)
	// 创建supervisor
	suv = &Supervisor{
		ConfigDir:    DefaultConfigDir,
		name2Program: make(map[string]*ProgramEx, 0),
		dbType:       cfg.Db.DbType,
		dbDSN:        cfg.Db.DbDsn,
		Host:         cfg.Host,
		cfg:          cfg,
		logDir:       logDir,
	}

	if false {
		// 用于测试Table的创建
		log.Printf("Before create Table succeed, %s, %s", suv.dbType, suv.dbDSN)
		var db *gorm.DB
		db, err = gorm.Open(suv.dbType, suv.dbDSN)
		if err != nil {
			log.ErrorErrorf(err, "failed to connect database")
			return
		}
		defer db.Close()

		log.Printf("Before create Table succeed: %v, %s, %s", db.HasTable(&Program{}), suv.dbType, suv.dbDSN)
		db.CreateTable(&Program{})
		log.Printf("Create Table succeed")
		os.Exit(1)
	}

	// log.Printf("Db Config: %s ==> %s", suv.dbType, suv.dbDSN)

	suv.namesMu.Lock()
	err = suv.LoadDBWithLock()
	suv.namesMu.Unlock()
	// 从DB加载
	if err != nil {
		return
	}

	// 顶一个各种API
	r := mux.NewRouter()

	// 静态资源的配置， 参考头部 init 函数
	r.HandleFunc("/", suv.hIndex)
	r.HandleFunc("/prefs/{name}", suv.hPref)
	r.HandleFunc("/prefs/{name}/{index}", suv.hPref)

	r.HandleFunc("/program/{name}/processes", suv.hProgram)

	r.HandleFunc("/api/status", suv.hStatus)
	r.HandleFunc("/api/reload", suv.hReload).Methods("POST")
	r.HandleFunc("/api/restart", suv.hRestartAll).Methods("POST")

	// 获取某个程序对应的所有的进程
	r.HandleFunc("/api/processes/{name}", suv.hProcesslist).Methods("GET")

	// 开始结束某个进程
	r.HandleFunc("/api/processes/{name}/{index}/start", suv.hStartProcess).Methods("POST")
	r.HandleFunc("/api/processes/{name}/{index}/stop", suv.hStopProcess).Methods("POST")

	r.HandleFunc("/api/programs", suv.hGetProgramList).Methods("GET")
	r.HandleFunc("/api/programs/{name}", suv.hGetProgram).Methods("GET")
	r.HandleFunc("/api/programs/{name}", suv.hDelProgram).Methods("DELETE")
	r.HandleFunc("/api/programs/{name}", suv.hUpdateProgram).Methods("PUT")
	r.HandleFunc("/api/programs", suv.hAddProgram).Methods("POST")
	r.HandleFunc("/api/programs/{name}/start", suv.hStartProgram).Methods("POST")
	r.HandleFunc("/api/programs/{name}/stop", suv.hStopProgram).Methods("POST")

	// 通知客户端有Events发生
	r.HandleFunc("/ws/events", suv.wsEvents)

	r.HandleFunc("/ws/logs/{name}", suv.wsLog)
	r.HandleFunc("/ws/logs/{name}/{index}", suv.wsLog)

	r.HandleFunc("/ws/perfs/{name}", suv.wsPerf)
	r.HandleFunc("/ws/perfs/{name}/{index}", suv.wsPerf)

	return suv, r, nil
}

//
// 1. 首页
//
func (s *Supervisor) hIndex(w http.ResponseWriter, r *http.Request) {
	s.renderHTML(w, r, "index/index.html", pongo2.Context{
		"Host": s.Host,
	})
}

//
// 2. 设置页面
//
func (s *Supervisor) hPref(w http.ResponseWriter, r *http.Request) {

	name := mux.Vars(r)["name"]
	log.Printf("Access log for: %s\n", name)

	indexStr := mux.Vars(r)["index"]
	index := int64(-1)
	if len(indexStr) > 0 {
		index, _ = strconv.ParseInt(indexStr, 10, 64)
	}

	s.renderHTML(w, r, "setting/settings.html", pongo2.Context{
		"Name":  name,
		"Index": index,
		"Host":  s.Host,
	})
}

//
// 3. 程序详情页面
//
func (s *Supervisor) hProgram(w http.ResponseWriter, r *http.Request) {
	programName := mux.Vars(r)["name"]

	s.namesMu.Lock()
	defer s.namesMu.Unlock()

	program := s.name2Program[programName]
	program.UpdateState()

	programEnc, _ := json.Marshal(program)

	s.renderHTML(w, r, "program/program.html", pongo2.Context{
		"Host":       s.Host,
		"name":       programName,
		"program":    program,
		"programEnc": string(programEnc),
	})
}

//
// 获取Program对应的Processlist
//
func (s *Supervisor) hProcesslist(w http.ResponseWriter, r *http.Request) {
	// 获取进程数
	programName := mux.Vars(r)["name"]
	s.namesMu.Lock()
	defer s.namesMu.Unlock()

	program := s.name2Program[programName]
	processes := program.Processes
	WriteJSON(w, processes)
}

// 当前服务的状态: 活着
func (s *Supervisor) hStatus(w http.ResponseWriter, r *http.Request) {
	WriteJSON(w, map[string]interface{}{
		"status": 0,
		"value":  "server is running",
	})
}

//
// 重新加载配置文件
//
func (s *Supervisor) hReload(w http.ResponseWriter, r *http.Request) {
	s.namesMu.Lock()
	defer s.namesMu.Unlock()

	err := s.LoadDBWithLock()

	ldapUser := r.Header.Get(LdapUserKey)
	log.Printf("操作: %s Reload config file", ldapUser)

	if err == nil {
		WriteJSON(w, JSONResponse{
			Status: 0,
			Value:  "load config success",
		})
	} else {
		WriteJSON(w, JSONResponse{
			Status: 1,
			Value:  err.Error(),
		})
	}
}

func (s *Supervisor) RestartAll() {
	log.Println("操作: Restart All Program")
	for name, program := range s.name2Program {
		log.Printf("操作: Restart program: %s", name)
		program.RestartAll()
	}
}

//
// 重新加载配置文件
//
func (s *Supervisor) hRestartAll(w http.ResponseWriter, r *http.Request) {
	s.namesMu.Lock()
	defer s.namesMu.Unlock()

	s.RestartAll()
	WriteJSON(w, JSONResponse{
		Status: 0,
		Value:  "Restart All Success",
	})
}

// 获取Program列表信息
func (s *Supervisor) hGetProgramList(w http.ResponseWriter, r *http.Request) {
	s.namesMu.Lock()
	defer s.namesMu.Unlock()

	data := s.Programs()
	WriteJSON(w, data)
}

// 获取Program的基本信息？
func (s *Supervisor) hGetProgram(w http.ResponseWriter, r *http.Request) {
	s.namesMu.Lock()
	defer s.namesMu.Unlock()

	name := mux.Vars(r)["name"]
	proc, ok := s.name2Program[name]
	if !ok {
		WriteJSON(w, JSONResponse{
			Status: 1,
			Value:  "program not exists",
		})
		return
	} else {
		WriteJSON(w, JSONResponse{
			Status: 0,
			Value:  proc,
		})
	}
}

func (s *Supervisor) normalizeUser(userName string, r *http.Request) string {
	// 只有Root账号启动的supervisor才有选择账号的权利
	if IsRoot() {
		ldapUser := r.Header.Get(LdapUserKey)
		if !containsString(s.cfg.Admins, ldapUser) {
			// 如果不是管理员，则只能有两个选择:
			// 自己 或 默认用户
			if userName == ldapUser || (len(s.cfg.DefaultUser) > 0 && s.cfg.DefaultUser == userName) {
				if _, err := user.Lookup(userName); err == nil {
					return userName
				}
			}

		} else {
			if _, err := user.Lookup(userName); err == nil {
				return userName
			} else {
				return ""
			}
		}
		return ""
	} else {
		// 不管是什么用户，反正都没用
		return userName
	}

}

// 添加Program的信息
func (s *Supervisor) hAddProgram(w http.ResponseWriter, r *http.Request) {
	retries, err := strconv.Atoi(r.FormValue("retries"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}

	processNum, err := strconv.Atoi(r.FormValue("process_num"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}

	stopTimeout, err := strconv.Atoi(r.FormValue("stop_timeout"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}

	if stopTimeout < 5 {
		stopTimeout = 5
	}

	pg := &Program{
		Name:         r.FormValue("name"),
		Command:      r.FormValue("command"),
		Dir:          r.FormValue("dir"),
		User:         r.FormValue("user"),
		Author:       r.FormValue("author"),
		StopTimeout:  stopTimeout,
		ProcessNum:   processNum, // 进程数字
		StartAuto:    r.FormValue("autostart") == "on",
		StartRetries: retries,
	}
	if pg.Dir == "" {
		pg.Dir = "/"
	}
	if err := pg.Check(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	var data map[string]interface{}

	userName := s.normalizeUser(pg.User, r)
	if len(userName) == 0 {
		// 无效的用户名
		// 无效的用户名
		WriteJSON(w, map[string]interface{}{
			"status": 1,
			"error":  fmt.Sprintf("Invalid user: %s, contact system admin", pg.User),
		})
		return
	}

	// 如何添加Program呢?
	s.namesMu.Lock()
	defer s.namesMu.Unlock()

	if _, ok := s.name2Program[pg.Name]; ok {
		data = map[string]interface{}{
			"status": 1,
			"error":  fmt.Sprintf("Program %s already exists", strconv.Quote(pg.Name)),
		}
	} else {
		// 记录当前用户
		if len(pg.Author) == 0 {
			pg.Author = r.Header.Get(LdapUserKey)
		}

		ldapUser := r.Header.Get(LdapUserKey)
		log.Printf("操作: %s add program: %s, Cmd: %s", ldapUser, pg.Name, pg.Command)

		if err := s.addOrUpdateProgram(pg, true); err != nil {

			data = map[string]interface{}{
				"status": 1,
				"error":  err.Error(),
			}
		} else {
			data = map[string]interface{}{
				"status": 0,
			}
		}
	}
	WriteJSON(w, data)
}

func (s *Supervisor) hUpdateProgram(w http.ResponseWriter, r *http.Request) {
	pg := Program{}
	err := json.NewDecoder(r.Body).Decode(&pg)
	if err != nil {
		WriteJSON(w, map[string]interface{}{
			"status": 1,
			"error":  err.Error(),
		})
		return
	}

	// 可以更新数据
	if pg.StopTimeout < 5 {
		pg.StopTimeout = 5
	}

	userName := s.normalizeUser(pg.User, r)
	if len(userName) == 0 {
		// 无效的用户名
		WriteJSON(w, map[string]interface{}{
			"status": 1,
			"error":  fmt.Sprintf("Invalid user: %s, contact system admin", pg.User),
		})
		return
	}

	s.namesMu.Lock()
	// 记录当前用户
	if len(pg.Author) == 0 {
		pg.Author = r.Header.Get(LdapUserKey)
	}
	pg.Host = s.Host // 所有的操作都和本机的host相关
	err = s.addOrUpdateProgram(&pg, true)
	s.namesMu.Unlock()

	ldapUser := r.Header.Get(LdapUserKey)
	log.Printf("操作: %s update program: %s, Cmd: %s", ldapUser, pg.Name, pg.Command)
	if err != nil {
		WriteJSON(w, map[string]interface{}{
			"status": 2,
			"error":  err.Error(),
		})
		return
	} else {
		WriteJSON(w, map[string]interface{}{
			"status":      0,
			"description": "program updated",
		})
	}
}

//
// 删除Program
//
func (s *Supervisor) hDelProgram(w http.ResponseWriter, r *http.Request) {
	name := mux.Vars(r)["name"]

	var data map[string]interface{}

	s.namesMu.Lock()
	defer s.namesMu.Unlock()

	if _, ok := s.name2Program[name]; !ok {
		data = map[string]interface{}{
			"status": 1,
			"error":  fmt.Sprintf("Program %s not exists", strconv.Quote(name)),
		}
	} else {
		ldapUser := r.Header.Get(LdapUserKey)
		log.Printf("操作: %s delete program: %s, Cmd: %s", ldapUser, name, s.name2Program[name].Command)

		s.removeProgram(name)

		data = map[string]interface{}{
			"status": 0,
		}
	}
	WriteJSON(w, data)
}

func (s *Supervisor) hStartProgram(w http.ResponseWriter, r *http.Request) {
	name := mux.Vars(r)["name"]
	s.namesMu.Lock()
	defer s.namesMu.Unlock()
	program, ok := s.name2Program[name]

	var data map[string]interface{}
	if !ok {
		data = map[string]interface{}{
			"status": 1,
			"error":  fmt.Sprintf("Process %s not exists", strconv.Quote(name)),
		}
	} else {
		ldapUser := r.Header.Get(LdapUserKey)
		program.StartAll(ldapUser)

		// 记住之前的状态
		program.StartAuto = true
		s.dbUpdateProgram(program.Program)

		data = map[string]interface{}{
			"status": 0,
			"name":   name,
		}
	}
	gEventPub.PostEvent(fmt.Sprintf("Program %s Started", program.Name))
	WriteJSON(w, data)
}

func (s *Supervisor) hStopProgram(w http.ResponseWriter, r *http.Request) {
	name := mux.Vars(r)["name"]
	s.namesMu.Lock()
	defer s.namesMu.Unlock()
	program, ok := s.name2Program[name]

	var data map[string]interface{}
	if !ok {
		data = map[string]interface{}{
			"status": 1,
			"error":  fmt.Sprintf("Process %s not exists", strconv.Quote(name)),
		}
	} else {
		ldapUser := r.Header.Get(LdapUserKey)
		program.StopAll(ldapUser)

		// 记住之前的状态
		program.StartAuto = false
		s.dbUpdateProgram(program.Program)

		data = map[string]interface{}{
			"status": 0,
			"name":   name,
		}
	}
	gEventPub.PostEvent(fmt.Sprintf("Program %s Stopped", program.Name))
	WriteJSON(w, data)
}

func (s *Supervisor) hStartProcess(w http.ResponseWriter, r *http.Request) {

	name := mux.Vars(r)["name"]

	s.namesMu.Lock()
	defer s.namesMu.Unlock()
	program, ok := s.name2Program[name]

	indexStr := mux.Vars(r)["index"]
	index, _ := strconv.ParseInt(indexStr, 10, 64)

	var data map[string]interface{}
	if !ok {
		data = map[string]interface{}{
			"status": 1,
			"error":  fmt.Sprintf("Process %s not exists", strconv.Quote(name)),
		}
	} else {
		ldapUser := r.Header.Get(LdapUserKey)
		log.Printf("操作: %s start process: %s, index: %d", ldapUser, program.Name, index)
		program.Merger.WriteStrLine(fmt.Sprintf("操作: %s start process: %s, index: %d\n", ldapUser, program.Name, index))
		program.StartOne(int(index))

		data = map[string]interface{}{
			"status": 0, // 开始成功
			"name":   name,
		}
	}
	gEventPub.PostEvent(fmt.Sprintf("Process %s:%d started", program.Name, index))
	WriteJSON(w, data)
}

func (s *Supervisor) hStopProcess(w http.ResponseWriter, r *http.Request) {
	name := mux.Vars(r)["name"]

	s.namesMu.Lock()
	defer s.namesMu.Unlock()
	program, ok := s.name2Program[name]

	indexStr := mux.Vars(r)["index"]
	index, _ := strconv.ParseInt(indexStr, 10, 64)

	var data map[string]interface{}
	if !ok {
		data = map[string]interface{}{
			"status": 1,
			"error":  fmt.Sprintf("Process %s not exists", strconv.Quote(name)),
		}
	} else {
		ldapUser := r.Header.Get(LdapUserKey)
		log.Printf("操作: %s stop process: %s, index: %d", ldapUser, program.Name, index)
		program.Merger.WriteStrLine(fmt.Sprintf("操作: %s stop process: %s, index: %d\n", ldapUser, program.Name, index))

		program.StopOne(int(index))
		data = map[string]interface{}{
			"status": 0,
			"name":   name,
		}
	}
	gEventPub.PostEvent(fmt.Sprintf("Process %s:%d Stopped", program.Name, index))
	WriteJSON(w, data)
}

//
// 服务器脚本状态改变，通知client更新
//
func (s *Supervisor) wsEvents(w http.ResponseWriter, r *http.Request) {
	// 1. 升级http为websocket
	c, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.ErrorErrorf(err, "upgrade to websocket failed")
		return
	}

	s.handleWs(gEventPub, r, c, true)
}

func (s *Supervisor) handleWs(output *WriteBroadcaster, r *http.Request, c *websocket.Conn, skipFirst bool) {
	var closed atomic2.Bool
	closed.Set(false)

	name := r.RemoteAddr
	logChan := output.NewChanString(name)

	// 必须有写数据的一方来关闭
	logHb := make(chan string, 15)
	go func() {
		// 来自客户端的关闭通知
		for !closed.Get() {
			_, data, err := c.ReadMessage()
			if err != nil {
				log.Printf("Close Writer by client error: %s", err.Error())
				closed.Set(true)
				close(logHb)
				break
			} else {
				logHb <- string(data)
			}
		}
	}()

	for !closed.Get() {
		select {
		case data := <-logHb:
			err := c.WriteMessage(1, []byte(data))
			if err != nil {
				log.Printf("Close Writer By write error: %s", r.RemoteAddr)
				closed.Set(true)
			}
		// 服务器timeout, 也会关闭logChan
		case data, ok := <-logChan:
			if ok {
				// Event不关注历史消息
				if skipFirst {
					skipFirst = false
					continue
				}

				// 添加一个ChanString, 将proc的输出转移到: Conn上
				err := c.WriteMessage(1, []byte(data))
				if err != nil {
					log.Printf("Close Writer By write error: %s", r.RemoteAddr)
					closed.Set(true)
				}
			} else {
				log.Printf("Close Writer By logChan closed error: %s", r.RemoteAddr)
				closed.Set(true)
			}
		case <-time.After(100 * time.Millisecond):
			// DO NOTHING
		}
	}

	output.CloseWriter(name)
}

func (s *Supervisor) wsLog(w http.ResponseWriter, r *http.Request) {
	// 读取参数
	name := mux.Vars(r)["name"]
	log.Printf("Access log for: %s\n", name)

	indexStr := mux.Vars(r)["index"]
	index := int64(-1)
	if len(indexStr) > 0 {
		index, _ = strconv.ParseInt(indexStr, 10, 64)
	}

	// 必须立马解除Lock
	s.namesMu.Lock()
	program, ok := s.name2Program[name]
	s.namesMu.Unlock()

	if !ok {
		log.Println("No such process")
		// TODO: raise error here?
		return
	}

	// 协议升级
	// 从http 1.1升级到ws, 或者http2.0之类的
	c, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.ErrorErrorf(err, "upgrade to websocket failed")
		return
	}
	defer c.Close()

	var output *QuickLossBroadcastWriter
	if index >= 0 {
		process := program.Processes[index]
		output = process.Output
	} else {
		output = program.Output
	}

	s.handleWs(output.WriteBroadcaster, r, c, false)

}

// Performance
func (s *Supervisor) wsPerf(w http.ResponseWriter, r *http.Request) {
	c, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.ErrorErrorf(err, "upgrade to websocket failed")
		return
	}
	defer c.Close()

	name := mux.Vars(r)["name"]
	indexStr := mux.Vars(r)["index"]
	index := int64(-1)
	if len(indexStr) > 0 {
		index, _ = strconv.ParseInt(indexStr, 10, 64)
	}

	// 必须立马解除Lock
	s.namesMu.Lock()
	program, ok := s.name2Program[name]
	s.namesMu.Unlock()

	if ok {
		var processes []*Process
		if index >= 0 {
			processes = []*Process{
				program.Processes[index],
			}
		} else {
			processes = program.Processes
		}

		for {
			var totalPinfo gops.ProcInfo
			for _, process := range processes {
				// c.SetWriteDeadline(time.Now().Add(3 * time.Second))
				if process.cmd == nil || process.cmd.Process == nil {
					continue
				}

				pid := process.cmd.Process.Pid
				ps, err := gops.NewProcess(pid)
				if err != nil {
					continue
				}
				mainPinfo, err := ps.ProcInfo()
				if err != nil {
					continue
				}
				totalPinfo.Add(mainPinfo)
				pi := ps.ChildrenProcInfo(true)
				totalPinfo.Add(pi)
			}

			totalPinfo.Pids = removeDuplicates(totalPinfo.Pids)
			err = c.WriteJSON(totalPinfo)
			if err != nil {
				break
			}
			time.Sleep(700 * time.Millisecond)
		}
	}
}

func removeDuplicates(elements []int) []int {
	// Use map to record duplicates as we find them.
	encountered := map[int]bool{}
	result := []int{}

	for v := range elements {
		if encountered[elements[v]] == true {
			// Do not add duplicate.
		} else {
			// Record this element as an encountered element.
			encountered[elements[v]] = true
			// Append to result slice.
			result = append(result, elements[v])
		}
	}
	// Return the new slice.
	return result
}

func init() {

	Assets = http.Dir("res")

	StaticHandler = http.FileServer(Assets)
	StaticHandler = NewExpireHanlder(GzipHandler(StaticHandler))

	// 设置pongo2的模板路径
	pongo2.DefaultSet.SetBaseDirectory(path.Join(string(Assets), "../templates"))

	// 不限制Origin
	upgrader.CheckOrigin = func(r *http.Request) bool {
		return true
	}
}
