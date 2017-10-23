package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/urfave/cli"
	log "github.com/wfxiang08/cyutils/utils/rolling_log"
	"gosuv"
	"io/ioutil"
	"net/http"
	"net/http/pprof"
	"net/url"
	"os"
	"os/signal"
	"path"
	"strconv"
	"syscall"
)

var (
	version string = "dev"
	cfg gosuv.Configuration
)

func catchExitSignal(suv *gosuv.Supervisor) {
	sigC := make(chan os.Signal, 1)
	signal.Notify(sigC, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)

	for sig := range sigC {
		if sig == syscall.SIGHUP {
			log.Println("Receive SIGHUP, just ignore")
			continue
		}
		log.Printf("Got signal: %v, stopping all running process\n", sig)
		break
	}

	// 关闭所有的进程
	suv.Close()

	// 等待所有的进程结束
	log.Printf("Waiting for processes to terminate")
	gosuv.ProcessWg.Wait()

	log.Printf("Supervisor process terminated")
}

// 如何启动Server呢?
func actionStartServer(c *cli.Context) error {

	logPath := c.String("L")
	logDir := "" // 其他程序的日志输出路径

	if len(logPath) > 0 {
		logDir = path.Dir(logPath)
		f, err := log.NewRollingFile(logPath, 3)
		if err != nil {
			log.PanicErrorf(err, "open rolling log file failed: %s", logPath)
		} else {
			defer f.Close()
			log.StdLog = log.New(f, "")
		}
	}

	log.Printf("-->-->-->-->-->-->-->-->-->-->-->-->-->-->-->-->-->-->-->-->-->-->-->-->-->")
	log.Printf("-->-->-->-->-->-->-->-->-->-->-->-->-->-->-->-->-->-->-->-->-->-->-->-->-->")
	suv, hdlr, err := gosuv.NewSupervisorHandler(&cfg, logDir)
	if err != nil {
		log.PanicError(err, "NewSupervisorHandler failed")
	}

	// 如何认证?
	// 可以接入Ldap认证?
	auth := cfg.Server.Ldap.Enabled
	if auth {
		// 添加Ldap认证
		hdlr = gosuv.NewLdapAuth(hdlr, &cfg, true)
	}

	if len(cfg.Db.DbDsn) == 0 || len(cfg.Db.DbType) == 0 {
		log.Panicf("Database should be configured")
	}

	apiPrefix := "/" + suv.Host + "/"
	http.Handle(apiPrefix, http.StripPrefix(apiPrefix[0:len(apiPrefix) - 1], hdlr))
	http.Handle("/", hdlr)

	func() {
		// 资源文件所在的目录
		resourcePrefix := "/" + suv.Host + "/res/"
		http.Handle(resourcePrefix, http.StripPrefix(resourcePrefix, gosuv.StaticHandler))
		http.Handle("/res/", http.StripPrefix("/res/", gosuv.StaticHandler))
	}()

	func() {

		http.Handle("/" + suv.Host + "/debug/pprof/", http.HandlerFunc(pprof.Index))
		http.Handle("/" + suv.Host + "/debug/pprof/cmdline", http.HandlerFunc(pprof.Cmdline))
		http.Handle("/" + suv.Host + "/debug/pprof/profile", http.HandlerFunc(pprof.Profile))
		http.Handle("/" + suv.Host + "/debug/pprof/symbol", http.HandlerFunc(pprof.Symbol))
		http.Handle("/" + suv.Host + "/debug/pprof/trace", http.HandlerFunc(pprof.Trace))
	}()

	addr := cfg.Server.Addr

	// 直接在命令行指定运行模式, 或者后台运行模式都走这条路
	//./gosuv start-server -f
	// 直接运行 ListenAndServe
	suv.AutoStartPrograms()

	log.Printf("server listen on %v", addr)
	go func() {
		err := http.ListenAndServe(addr, nil)
		if err != nil {
			log.ErrorErrorf(err, "ListenAndServe error")
		}
	}()

	catchExitSignal(suv)

	return nil
}

func checkServerStatus() error {
	// 抓取status
	resp, err := http.Get(cfg.Client.ServerURL + "/api/status")
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	// json解析
	var ret gosuv.JSONResponse
	err = json.Unmarshal(body, &ret)
	if err != nil {
		return errors.New("json loads error: " + string(body))
	}
	if ret.Status != 0 {
		return fmt.Errorf("%v", ret.Value)
	}
	return nil
}

func actionStatus(c *cli.Context) error {
	err := checkServerStatus()
	if err != nil {
		log.ErrorErrorf(err, "checkServerStatus error")
		return err
	} else {
		log.Println("Server is running, OK.")
	}
	return nil
}

func postForm(pathname string, data url.Values) (r gosuv.JSONResponse, err error) {
	// 如何post请求呢?
	//      cfg.Client.ServerURL + pathname
	//
	url := cfg.Client.ServerURL + pathname
	log.Printf("Request Url: %s", url)
	resp, err := http.PostForm(url, data)
	if err != nil {
		return r, err
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return r, err
	}
	err = json.Unmarshal(body, &r)
	if err != nil {
		return r, fmt.Errorf("POST %v %v", strconv.Quote(pathname), string(body))
	}
	return r, nil
}

func actionRestart(c *cli.Context) error {
	// 内部访问Http接口(重启内部所有的程序)
	ret, err := postForm("/api/restart", nil)
	if err != nil {
		log.ErrorErrorf(err, "Shutdown failed")
		return err
	} else {
		log.Printf("Restart All programs: %v", ret.Value)
		return nil
	}

}

// 所有的操作都通过api来实现
func actionReload(c *cli.Context) error {
	ret, err := postForm("/api/reload", nil)
	if err != nil {
		log.ErrorErrorf(err, "reload failed")
		return err
	}
	fmt.Println(ret.Value)
	return nil
}

func actionConfigTest(c *cli.Context) error {
	if _, _, err := gosuv.NewSupervisorHandler(&cfg, ""); err != nil {
		log.ErrorErrorf(err, "config test failed")
		return err
	}
	log.Println("test is successful")
	return nil
}

func actionVersion(c *cli.Context) error {
	fmt.Printf("gosuv version %s\n", version)
	return nil
}

func main() {

	app := cli.NewApp()
	app.Name = "tool_gosuv"
	app.Version = version
	app.Usage = "golang port of python-supervisor"
	gosuv.Version = version

	app.Before = func(c *cli.Context) error {
		var err error
		cfgPath := c.GlobalString("conf")

		log.Printf("ConfigPath: %s", cfgPath)
		// 读取配置文件
		cfg, err = gosuv.ReadConf(cfgPath)
		if err != nil {
			log.ErrorErrorf(err, "config read failed")
			return err
		}
		return nil
	}
	// 当前app支持的Flags(主要是输入config)
	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name:  "conf, c",
			Usage: "config file",
			Value: "",
		},
	}

	// 当前app支持的Commands
	app.Commands = []cli.Command{
		{
			Name:  "start",
			Usage: "Start supervisor",
			Flags: []cli.Flag{
				cli.StringFlag{
					Name:  "conf, c",
					Usage: "config file",
					Value: "",
				},
				cli.StringFlag{
					Name:  "L",
					Usage: "log file",
					Value: "",
				},
			},
			Action: actionStartServer,
		},
		{
			Name:    "status",
			Aliases: []string{"st"},
			Usage:   "Show program status",
			Action:  actionStatus,
		},
		{
			Name:   "reload",
			Usage:  "Reload config file",
			Action: actionReload,
		},

		{
			//
			// 命令: /usr/local/gosuv/tool_gosuv -c /usr/local/gosuv/config.yml restart
			//      /usr/local/video/tool_gosuv restart
			Name:   "restart",
			Usage:  "Restart programs",
			Action: actionRestart,
		},
		{
			Name:    "conftest",
			Aliases: []string{"t"},
			Usage:   "Test if config file is valid",
			Action:  actionConfigTest,
		},

		{
			Name:    "version",
			Usage:   "Show version",
			Aliases: []string{"v"},
			Action:  actionVersion,
		},
	}

	// 运行
	if err := app.Run(os.Args); err != nil {
		log.Printf("Run App Error: %v", err)
		os.Exit(1)
	}
}
