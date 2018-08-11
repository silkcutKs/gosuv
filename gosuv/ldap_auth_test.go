package gosuv

import (
	"testing"
	"fmt"
)

// go test github.com/wfxiang08/gosuv/gosuv -v -run "TestLdapAuth"
func TestLdapAuth(t *testing.T) {

	cfgPath := "../conf/config.yml"
	cfg, err := ReadConf(cfgPath)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	ok, user_info, groups := VerifyUserNamePassword("xxx", "xxxx", &cfg)
	fmt.Printf("ok: %t, user_info: %v, groups: %s\n", ok, user_info, groups)

}