package gosuv

import (
	"testing"
	"fmt"
	"strings"
)

// go test gosuv -v -run "TestCommandPreprocess"
func TestCommandPreprocess(t *testing.T) {

	cmd := "SENTRY_CONF=/etc/sentry /www/sentry/bin/sentry run cron"
	newCmd, kvs := PreprocessCommand(cmd)

	fmt.Printf("NewCommand: %s\n", newCmd)
	fmt.Printf("Envs: %s\n", strings.Join(kvs, ", "))


	cmd = "   SENTRY_CONF=/etc/sentry /www/sentry/bin/sentry run cron"
	newCmd, kvs = PreprocessCommand(cmd)

	fmt.Printf("NewCommand: %s\n", newCmd)
	fmt.Printf("Envs: %s\n", strings.Join(kvs, ", "))


	cmd = "/www/sentry/bin/sentry run cron"
	newCmd, kvs = PreprocessCommand(cmd)

	fmt.Printf("NewCommand: %s\n", newCmd)
	fmt.Printf("Envs: %s\n", strings.Join(kvs, ", "))
}