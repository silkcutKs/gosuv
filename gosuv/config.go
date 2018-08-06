package gosuv

import (
	"github.com/go-yaml/yaml"
	"io/ioutil"
)

type Configuration struct {
	Server struct {
		Ldap struct {
			Enabled      bool     `yaml:"enabled"`
			Host         string   `yaml:"host"`
			Base         string   `yaml:"base"`
			Port         int      `yaml:"port"`
			UseSSL       bool     `yaml:"use_ssl"`
			BindDN       string   `yaml:"bind_dn"`
			BindPassword string   `yaml:"bind_password"`
			UserFilter   string   `yaml:"user_filter"`
			Attributes   []string `yaml:"attributes"`
		} `yaml:"ldap"`
		Addr string `yaml:"addr"`
	} `yaml:"server"`

	Client struct {
		ServerURL string `yaml:"server_url"`
	} `yaml:"client"`
	Db struct {
		DbType string `yaml:"db_type"`
		DbDsn  string `yaml:"db_dsn"`
	} `yaml:"db"`
	Host        string   `yaml:"host"`
	DefaultUser string   `yaml:"default_user"`
	Admins      []string `yaml:"admins"`
}

func ReadConf(filename string) (c Configuration, err error) {
	// 初始默认值
	c.Server.Addr = ":11313"
	c.Client.ServerURL = "http://localhost:11313"

	// 读取配置文件
	data, err := ioutil.ReadFile(filename)
	if err != nil {
		data = []byte("")
	}
	err = yaml.Unmarshal(data, &c)
	if err != nil {
		return
	}

	//// 保存配置文件
	//cfgDir := filepath.Dir(filename)
	//if !IsDir(cfgDir) {
	//	os.MkdirAll(cfgDir, 0755)
	//}
	//data, _ = yaml.Marshal(c)
	//err = ioutil.WriteFile(filename, data, 0644)
	return
}
