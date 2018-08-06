package gosuv

import (
	"encoding/json"
	"github.com/flosch/pongo2"
	"net/http"
	"os/user"
)

type JSONResponse struct {
	Status int         `json:"status"`
	Value  interface{} `json:"value"`
}

//
// 将result以JSON格式返回
//
func WriteJSON(w http.ResponseWriter, result interface{}) {
	data, err := json.Marshal(result)

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=UTF-8")
	w.Write(data)
}

// 添加ldap账号信息
// https://stackoverflow.com/questions/28384343/golang-accessing-a-map-using-its-reference
func (s *Supervisor) injectUserInfo(r *http.Request, data pongo2.Context) {
	ldapUser := r.Header.Get(LdapUserKey)
	isAdmin := containsString(s.cfg.Admins, ldapUser)
	// log.Printf("Ldap User: %s, is_admin: %v, admins: %s", ldapUser, isAdmin, strings.Join(s.cfg.Admins, ", "))
	// 管理员可以控制其他所有的脚本
	data["LdapUser"] = ldapUser
	data["IsAdmin"] = isAdmin
}

//
// 如何处理html模板呢?
//
func (s *Supervisor) renderHTML(w http.ResponseWriter, r *http.Request, filePath string, data pongo2.Context) {
	// 1. 加载模板
	if data == nil {
		// 默认展示运行当前服务的用户
		data = pongo2.Context{
			"Version": Version,
			"User":    "",
		}

		user1, err := user.Current()
		if err == nil {
			data["User"] = user1.Username
		}
	}
	s.injectUserInfo(r, data)

	// 默认是text/html格式
	w.Header().Set("Content-Type", "text/html")

	tpl, err1 := pongo2.FromFile(filePath)
	if err1 != nil {
		http.Error(w, err1.Error(), http.StatusInternalServerError)
		return
	}

	err := tpl.ExecuteWriter(data, w)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
