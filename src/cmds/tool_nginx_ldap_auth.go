package main

import (
	"flag"
	"fmt"
	log "github.com/wfxiang08/cyutils/utils/rolling_log"
	"gosuv"
	"net/http"
)

type SimpleHandler struct {
}

func (h *SimpleHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(200)
}

var (
	host   = flag.String("host", "", "ip/host that bind to, default *")
	port   = flag.Int("port", 8091, "port that bind to, default 8080")
	config = flag.String("config", "conf/config.yml", "")
)

func main() {
	flag.Parse()

	if len(*config) == 0 {
		log.Errorf("Config not specified")
	}
	cfg, err := gosuv.ReadConf(*config)
	if err != nil {
		log.ErrorErrorf(err, "Config file read failed")
		return
	}
	handler := &SimpleHandler{}
	authHandler := gosuv.NewLdapAuth(handler, &cfg, false)
	http.Handle("/", authHandler)
	err = http.ListenAndServe(fmt.Sprintf("%s:%d", *host, *port), nil)

	if err != nil {
		log.ErrorErrorf(err, "ListenAndServe failed")
		return
	}
}
