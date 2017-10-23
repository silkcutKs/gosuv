package gosuv

import (
	"testing"
	"fmt"
)

// go test gosuv -v -run "TestLdapAuth"
func TestLdapAuth(t *testing.T) {

	cfgPath := "../conf/config.media1.yml"
	cfg, err := ReadConf(cfgPath)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	ok, user_info, groups := VerifyUserNamePassword("fei.wang", "", &cfg)
	fmt.Printf("ok: %t, user_info: %v, groups: %s\n", ok, user_info, groups)

}