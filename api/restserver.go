package api

import (
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"strings"
	"time"

	"minidhcp/base"

	restful "github.com/emicklei/go-restful/v3"
)

var log = base.GetLogger("restserver")

type (
	RestServer struct {
		cfg *base.Config
	}

	config struct {
		Iface string `json:"netInterface"`
		Match string `json:"allocate"`
	}
	reqConfig struct {
		Data config `json:"rdata"`
	}

	iprange struct {
		Id        int    `json:"id"`
		Name      string `json:"name"`
		Vlan      int    `json:"vlanId"`
		IpRange   string `json:"ipRange"`
		Leasetime string `json:"leaseTime"`
		Mask      string `json:"ipMask"`
		Gateway   string `json:"Gateway"`
		Dns1      string `json:"dns"`
	}
	reqRange struct {
		Data []iprange `json:"rdata"`
	}

	ipstatic struct {
		Name string `json:"name"`
		Mac  string `json:"mac"`
		Ip   string `json:"ip"`
	}
	reqStaticRoute struct {
		Data []ipstatic `json:"rdata"`
	}
)

func (r *RestServer) respSuccess(resp *restful.Response) {
	resp.WriteAsJson(`{"rcode":"QS000000","rmsg":"success","rdata":"success"}`)
}

func (r *RestServer) respError(resp *restful.Response, err error) {
	resp.WriteError(http.StatusInternalServerError, err)
	log.Info("respError", err.Error())
}

//设置DHCP服务配置 POST https://ip:port/dhcp/config
// "rdata": {
// 		"ifacce": "eth1"
// 		"match":  "ipmac" //[ipmac | authuser]
// }
// 出参：{"rcode":"QS000000","rmsg":"success","rdata":"success"}
func (r *RestServer) setConfig(req *restful.Request, resp *restful.Response) {
	cfg := new(reqConfig)
	err := req.ReadEntity(&cfg)
	if err != nil {
		r.respError(resp, err)
		return
	}

	r.cfg.Ifname = cfg.Data.Iface
	r.cfg.Marshal()

	r.respSuccess(resp)
}

//设置分配的动态IP段 POST
// https://ip:port/dhcp/range
//  "rdata": [
// 	{
// 		"name": ""
// 		"vlan": ""
// 		"ipstart": "192.168.0.1"
// 		"ipstop": "192.168.0.1"
// 		"leasetime": "60s"
// 		"mask": "255.255.255.0"
// 		"gateway": "192.168.1.1"
// 		"dns1": "192.168.1.100"
// 	},
// ]
func (r *RestServer) setRange(req *restful.Request, resp *restful.Response) {
	rrg := new(reqRange)
	err := req.ReadEntity(&rrg)
	if err != nil {
		r.respError(resp, err)
		return
	}

	for _, v := range rrg.Data {
		rg := r.cfg.Guest
		if v.Name == "staff" {
			rg = r.cfg.Staff
		} else if v.Name == "boss" {
			rg = r.cfg.Boss
		}

		ss := strings.Split(v.IpRange, "-")
		if len(ss) < 2 {
			r.respError(resp, errors.New("IpRange array length less than 2"))
			return
		}
		rg.Role = v.Name
		rg.IpStart = ss[0]
		rg.IpStop = ss[1]
		rg.Dns = v.Dns1
		rg.Router = v.Gateway
		rg.Netmask = v.Mask
		rg.LeaseTime = v.Leasetime
	}
	r.cfg.Marshal()

	r.respSuccess(resp)
}

func (r *RestServer) setStaticRoute(req *restful.Request, resp *restful.Response) {
	rsr := new(reqStaticRoute)
	err := req.ReadEntity(&rsr)
	if err != nil {
		r.respError(resp, err)
		return
	}

	if len(rsr.Data) < 1 {
		r.respError(resp, errors.New("static router array length less than 1"))
		return
	}
	l := rsr.Data[0]
	r.cfg.Staticrouter1 = l.Mac + " " + l.Ip
	r.cfg.Marshal()

	r.respSuccess(resp)
}

//获取租约分配记录 GET
// https://ip:port/dhcp/lease
func (r *RestServer) getAllocateLease(req *restful.Request, resp *restful.Response) {
	file, err := os.OpenFile("lease.txt", os.O_RDONLY, 0444)
	if err != nil {
		r.respError(resp, err)
	}
	defer file.Close()

	b, err := ioutil.ReadAll(file)
	if err != nil {
		log.Fatal(err)
	}
	s := `{"rcode":"QS000000","rmsg":"success","rdata":"` + string(b) + `"}`
	resp.WriteAsJson(s)
}

func NewRestServer(cfg *base.Config) {
	r := RestServer{cfg}

	ws := new(restful.WebService)
	ws.Filter(minidhcpLogging)
	ws.Path("/dhcp").
		Consumes(restful.MIME_JSON).
		Produces(restful.MIME_JSON) // you can specify this per route as well

	ws.Route(ws.POST("/config").To(r.setConfig))
	ws.Route(ws.POST("/range").To(r.setRange))
	ws.Route(ws.POST("/staticroute").To(r.setStaticRoute))
	ws.Route(ws.POST("/lease").To(r.getAllocateLease))

	restful.DefaultContainer.Add(ws)

	// log.Info("start listening on ", log.String("ipport", ipport))
	restport := ":" + cfg.RestPort
	fmt.Println("start listening on ", restport)
	go func() {
		err := http.ListenAndServe(restport, nil)
		fmt.Println("ListenAndServe", err)
		// log.Fatal("ListenAndServe", log.String("err", err.Error()))
	}()
}

// WebService Filter
func minidhcpLogging(req *restful.Request, resp *restful.Response, chain *restful.FilterChain) {
	now := time.Now()
	fmt.Println("reqest", time.Since(now), req.Request.Method+req.Request.URL.String())
	// log.Info("reqest",
	// 	log.String("timeat", time.Now().Sub(now).String()),
	// 	log.String("url", req.Request.Method+req.Request.URL.String()))
	chain.ProcessFilter(req, resp)
}
