package api

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	restful "github.com/emicklei/go-restful/v3"
)

const (
	bodyCfg = `{"rdata":{"iface":"eth1","match":"ipmac"}}`
	bodyRg  = `{"rdata":[{"name":"n","vlan":"v","ipstart":"192.168.0.1","ipstop":"192.168.0.2","leasetime":"60s","mask":"","gateway":"","dns1":""}]}`
	bodySg  = `{"rdata":[{"ip":"192.168.0.1","mac":"00:1A:6D:38:15:FF","name":"60s"}]}`
)

var (
	server RestServer = RestServer{}
	host   string     = "/dhcp"
)

func newReq(cmd, body string) *http.Request {
	req, err := http.NewRequest("POST", host+cmd, strings.NewReader(body))
	if err != nil {
		fmt.Println("newReq-http.NewRequest: ", err)
	}
	req.Header.Set("Content-Type", "application/json")
	return req
}

func verifyResultSuccess(t *testing.T, rr *httptest.ResponseRecorder) {
	resp := rr.Result()
	if resp.StatusCode != http.StatusOK {
		t.Error("Response code is ", resp.StatusCode)
	}
	if !bytes.ContainsAny(rr.Body.Bytes(), "QS000000") {
		t.Error("Response code error", rr.Body.String())
	}
}

func TestMain(m *testing.M) {
	setHandler := func(cmd string, f func(req *restful.Request, resp *restful.Response)) {
		http.DefaultServeMux.HandleFunc("/dhcp"+cmd, func(w http.ResponseWriter, r *http.Request) {
			f(restful.NewRequest(r), restful.NewResponse(w))
		})
	}
	setHandler("/config", server.setConfig)
	setHandler("/range", server.setRange)
	setHandler("/staticroute", server.setStaticRoute)

	os.Exit(m.Run())
}

func TestSetConfig(t *testing.T) {
	req := newReq("/config", strings.ReplaceAll(bodyCfg, " ", ""))
	resp := httptest.NewRecorder()
	http.DefaultServeMux.ServeHTTP(resp, req)

	verifyResultSuccess(t, resp)
}

func TestSetRange(t *testing.T) {
	req := newReq("/range", strings.ReplaceAll(bodyRg, " ", ""))
	resp := httptest.NewRecorder()
	http.DefaultServeMux.ServeHTTP(resp, req)

	verifyResultSuccess(t, resp)
}

func TestSetStaticRoute(t *testing.T) {
	req := newReq("/staticroute", strings.ReplaceAll(bodySg, " ", ""))
	resp := httptest.NewRecorder()
	http.DefaultServeMux.ServeHTTP(resp, req)

	verifyResultSuccess(t, resp)
}
