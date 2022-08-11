//go:build linux
// +build linux

package main

import (
	"context"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"minidhcp/api"
	"minidhcp/base"
	"minidhcp/server"

	"github.com/sirupsen/logrus"

	"github.com/insomniacslk/dhcp/dhcpv4"
	"github.com/insomniacslk/dhcp/dhcpv4/client4"
)

var log = base.GetLogger("main")

// var (
// 	flagTestClientRequest  = flag.Bool("isTestClientRequest", false, "开启定时测试请求dhcp4")
// 	flagTestOnlyRestServer = flag.Bool("isTestOnlyRestServer", false, "取消DHCP server，只有Rest Server 测试用")
// 	// flagLogLevel           = flag.String("loglevel", "L", "info", fmt.Sprintf("Log level. One of %v", getLogLevels()))
// )

func testRequset(ifname string) {
	timer2 := time.NewTimer(time.Second * 2)
	go func() {
		<-timer2.C
		c := client4.NewClient()
		c.LocalAddr = &net.UDPAddr{
			IP:   net.ParseIP("127.0.0.1"),
			Port: 68,
		}
		c.RemoteAddr = &net.UDPAddr{
			IP:   net.ParseIP("127.0.0.1"),
			Port: 67,
		}
		log.Printf("local/remote addr %+v", c)

		mac, err := net.ParseMAC("66:55:44:33:22:11")
		if err != nil {
			log.Fatal("ParseMAC", "66:55:44:33:22:11", err)
		}
		conv, err := c.Exchange(ifname, dhcpv4.WithHwAddr(mac))
		for _, p := range conv {
			log.Print(p.Summary())
		}
		if err != nil {
			log.Fatal("Exchange ", err)
		}
	}()
}

func main() {
	log.Logger.SetLevel(logrus.DebugLevel)
	base.WithFile(log, "minidhcp.log")
	// logger.WithNoStdOutErr(log)

	cfg := base.LoadConfig()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		stop()
		return
	}()

	// start rest api server
	go api.NewRestServer(cfg)

	srv, err := server.Start(cfg)
	if err != nil {
		log.Fatal(err)
	}

	// for test request
	// testRequset(cfg.Ifname)

	// run dhcp server
	if err := srv.Wait(); err != nil {
		log.Print(err)
	}
}
