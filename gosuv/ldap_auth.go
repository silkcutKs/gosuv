package gosuv

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"github.com/jtblin/go-ldap-client"
	"github.com/wfxiang08/cyutils/utils/log"
	"net/http"
	"strings"
	"sync"
)

const LdapUserKey = "ldap_uid"
const LdapUserMail = "ldap_mail"
const LdapUserGroupsKey = "ldap_ugroups"

type LdapUserInfo struct {
	UserId string
	Mail   string
	Groups string
}

func (u *LdapUserInfo) UpdateHeaders(h http.Header) {
	h.Set(LdapUserKey, u.UserId)
	h.Set(LdapUserMail, u.Mail)
	h.Set(LdapUserGroupsKey, u.Groups)
}
func (u *LdapUserInfo) String() string {
	return fmt.Sprintf("%s:%s(%s)", u.UserId, u.Mail, u.Groups)
}

var auth2UserInfo map[string]*LdapUserInfo
var rwLock sync.RWMutex


func init() {
	auth2UserInfo = make(map[string]*LdapUserInfo)
}

func GetUserInfo(r *http.Request) *LdapUserInfo {
	rwLock.RLock()
	auth := r.Header.Get("Authorization")
	userInfo, ok := auth2UserInfo[auth]
	rwLock.RUnlock()
	if ok {
		return userInfo
	} else {
		return nil
	}
}

type LdapAuth struct {
	f        http.Handler
	cfg      *Configuration
	checkUrl bool
}

func NewLdapAuth(f http.Handler, cfg *Configuration, checkUrl bool) *LdapAuth {
	return &LdapAuth{
		f:        f,
		cfg:      cfg,
		checkUrl: checkUrl,
	}
}
func (l *LdapAuth) ServeHTTP(w http.ResponseWriter, r *http.Request) {

	// 静态资源直接返回
	if l.checkUrl {
		if strings.HasPrefix(r.URL.Path, "/res") {
			l.f.ServeHTTP(w, r)
			return
		} else if strings.HasPrefix(r.URL.Path, "/ws/") {
			// localhost 访问API, 直接放行
			l.f.ServeHTTP(w, r)
			return
		} else if strings.HasPrefix(r.RequestURI, "/api/restart") {
			// r.RequestURI
			//    /api/restart 从本机访问
			//    /worker1/api/restart 从nginx proxy
			//
			l.f.ServeHTTP(w, r)
			return
		}
	}

	basicAuthPrefix := "Basic "
	// Parse request header
	auth := r.Header.Get("Authorization")
	rwLock.RLock()
	userInfo, ok := auth2UserInfo[auth]
	rwLock.RUnlock()

	if ok {
		userInfo.UpdateHeaders(r.Header)
		l.f.ServeHTTP(w, r)
		return
	}

	if strings.HasPrefix(auth, basicAuthPrefix) {
		// Decoding authentication information.
		payload, err := base64.StdEncoding.DecodeString(
			auth[len(basicAuthPrefix):],
		)

		if err == nil {
			pair := bytes.SplitN(payload, []byte(":"), 2)
			// 账号密码OK, 就直接放行
			if len(pair) == 2 {
				ok, user, groups := VerifyUserNamePassword(string(pair[0]), string(pair[1]), l.cfg)
				// 如何成功，则继续
				if ok {
					userInfo := &LdapUserInfo{
						UserId: user["uid"],
						Mail:   user["mail"],
						Groups: strings.Join(groups, ","),
					}
					rwLock.Lock()
					auth2UserInfo[auth] = userInfo
					log.Printf("Ldap User: %s", userInfo.String())
					userInfo.UpdateHeaders(r.Header)
					rwLock.Unlock()
					l.f.ServeHTTP(w, r)
					return
				}

			}
		}
	}
	// Authorization fail, return 401 Unauthorized.
	w.Header().Set("WWW-Authenticate", `Basic realm="Restricted"`)
	w.WriteHeader(http.StatusUnauthorized)
}

func VerifyUserNamePassword(username, password string, cfg *Configuration) (bool, map[string]string, []string) {
	client := &ldap.LDAPClient{
		Base:         cfg.Server.Ldap.Base,
		Host:         cfg.Server.Ldap.Host,
		Port:         cfg.Server.Ldap.Port,
		UseSSL:       cfg.Server.Ldap.UseSSL,
		BindDN:       cfg.Server.Ldap.BindDN,
		BindPassword: cfg.Server.Ldap.BindPassword,
		UserFilter:   cfg.Server.Ldap.UserFilter,
		GroupFilter:  "(memberUid=%s)",
		Attributes:   cfg.Server.Ldap.Attributes,
	}
	// It is the responsibility of the caller to close the connection
	defer client.Close()

	ok, user, _ := client.Authenticate(username, password)

	var groups []string
	var err error
	if ok {
		groups, err = client.GetGroupsOfUser(username)
		if err != nil {
			log.ErrorErrorf(err, "GetGroupsOfUser of %s failed", username)
		}
	}
	return ok, user, groups
}
