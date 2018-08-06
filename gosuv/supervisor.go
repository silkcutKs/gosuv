package gosuv

import (
	"fmt"
	_ "github.com/go-sql-driver/mysql"
	"github.com/jinzhu/gorm"
	"github.com/wfxiang08/cyutils/utils/errors"
	log "github.com/wfxiang08/cyutils/utils/log"
	"path/filepath"
	"sort"
	"sync"
)

// 全局的Event Pub/Scribe
var gEventPub *WriteBroadcaster

func init() {
	gEventPub = NewWriteBroadcaster(4 * 1024)
}

// 两个概念:
//          Program
//          Process, 如何实现Process的多进程控制呢?
type Supervisor struct {
	ConfigDir string
	// Programs的管理
	name2Program map[string]*ProgramEx
	namesMu      sync.Mutex // 只用在Api, 或者初始化脚本中；内部函数不使用

	dbType string
	dbDSN  string
	Host   string

	cfg    *Configuration
	logDir string
}

func (s *Supervisor) Programs() []*ProgramEx {
	// 按照names的顺序返回programs
	pgs := make([]*ProgramEx, 0, len(s.name2Program))
	for _, program := range s.name2Program {
		program.UpdateState()
		pgs = append(pgs, program)
	}

	// 按照name做排序
	sort.Sort(ProgramSlice(pgs))
	return pgs
}

// 获取配置文件的路径
func (s *Supervisor) programPath() string {
	return filepath.Join(s.ConfigDir, "programs.yml")
}

// 需要WLock
func (s *Supervisor) addOrUpdateProgram(newProg *Program, saveDb bool) error {
	// 验证Program是否有效
	if err := newProg.Check(); err != nil {
		return err
	}

	oldProg, ok := s.name2Program[newProg.Name]

	if ok {
		// 更新已有的Program
		oldProg.UpdateProgram(newProg)

		if saveDb {
			s.dbUpdateProgram(oldProg.Program)
		}

		gEventPub.PostEvent(fmt.Sprintf("Program %s Updated", newProg.Name))
	} else {
		// 添加新的Program
		prog := &ProgramEx{
			Program: newProg,
		}
		prog.InitProgram(s.logDir)
		s.name2Program[newProg.Name] = prog
		if saveDb {
			s.dbInsertProgram(prog.Program)
		}

		gEventPub.PostEvent(fmt.Sprintf("Program %s Inserted", newProg.Name))
	}

	return nil
}

// Check
// - Yaml format
// - Duplicated program
func (s *Supervisor) readConfigFromDB() (pgs []*Program, err error) {
	log.Printf("readConfigFromDB ...")

	log.Printf("DB: %s, %s", s.dbType, s.dbDSN)
	// 创建数据库连接
	// http://jinzhu.me/gorm/database.html#connecting-to-a-database
	db, err := gorm.Open(s.dbType, s.dbDSN)
	if err != nil {
		return nil, errors.New("Failed to open database")
	}

	defer db.Close()

	// 读取Programs
	var programs []Program
	db.Where("host = ?", s.Host).Find(&programs)
	pgs = make([]*Program, 0)

	visited := map[string]bool{}
	for index, _ := range programs {
		// 不能有重复的元素
		if visited[programs[index].Name] {
			log.Errorf("Duplicated program name: %s", programs[index].Name)
			continue
		}
		pgs = append(pgs, &programs[index])
		visited[programs[index].Name] = true
	}

	for _, pg := range pgs {
		log.Printf("Loaded Program: %s", pg.String())
	}
	log.Printf("Programs Load Num: %d", len(pgs))

	return
}

func (s *Supervisor) LoadDBWithLock() error {
	// 加载配置文件
	pgs, err := s.readConfigFromDB()
	if err != nil {
		return err
	}
	// add or update program
	visited := map[string]bool{}
	for _, pg := range pgs {
		visited[pg.Name] = true
		s.addOrUpdateProgram(pg, false)
	}

	// delete not exists program
	for _, pg := range s.name2Program {
		if visited[pg.Name] {
			continue
		}
		s.removeProgram(pg.Name)
	}
	return nil
}

func (s *Supervisor) dbRemoveProgram(program *Program) {

	// 创建数据库连接
	// http://jinzhu.me/gorm/database.html#connecting-to-a-database
	db, err := gorm.Open(s.dbType, s.dbDSN)
	if err != nil {
		panic("failed to connect database")
	}
	defer db.Close()

	// 从数据库删除
	db.Delete(program)

}

func (s *Supervisor) dbUpdateProgram(program *Program) {

	// 创建数据库连接
	// http://jinzhu.me/gorm/database.html#connecting-to-a-database
	db, err := gorm.Open(s.dbType, s.dbDSN)
	if err != nil {
		log.ErrorErrorf(err, "failed to connect database")
		return
	}
	defer db.Close()

	program.Host = s.Host
	program.Encode()
	db.Save(program)
	log.Printf("Update program: %s", program.String())

}

func (s *Supervisor) dbInsertProgram(program *Program) {

	// 创建数据库连接
	// http://jinzhu.me/gorm/database.html#connecting-to-a-database
	db, err := gorm.Open(s.dbType, s.dbDSN)
	if err != nil {
		log.ErrorErrorf(err, "failed to connect database")
		return
	}
	defer db.Close()

	program.Encode()
	program.Host = s.Host

	if db.Create(program).RowsAffected == 0 {
		var oldProgram Program
		if !db.First(&oldProgram, "host = ? and name = ?", program.Host, program.Name).RecordNotFound() {
			program.ID = oldProgram.ID
			db.Save(program).Update()
		} else {
			log.Printf("Error insert record: %s", program.String())
		}
	}

	log.Printf("Add new program: %s", program.String())
}

func (s *Supervisor) removeProgram(name string) bool {

	// 删除program
	if program, ok := s.name2Program[name]; ok {
		log.Printf("RemoveProgram: %s", name)

		s.dbRemoveProgram(program.Program)
		delete(s.name2Program, name)

		// 关闭所有的Process
		program.StopAndWaitAll()
		gEventPub.PostEvent(program.Name + " deleted")

		return true
	} else {
		return false
	}
}

func (s *Supervisor) Close() {
	for _, program := range s.name2Program {
		program.StopAll("admin")
	}
}

func (s *Supervisor) AutoStartPrograms() {
	// 自动运行的Program, 直接启动
	for _, program := range s.name2Program {
		if program.StartAuto {
			log.Printf("Auto Start Programs: %s", program.Name)
			program.StartAll("admin")
		}
	}
}
