package server

import (
	"fmt"
	"net"
	"sync"

	"golang.org/x/net/ipv4"

	"minidhcp/base"
	"minidhcp/options"

	"github.com/insomniacslk/dhcp/dhcpv4"
	"github.com/insomniacslk/dhcp/dhcpv4/server4"
)

const MaxDatagram = 1 << 16

var log = base.GetLogger("server")

// buffer pool for recv & send
var bufpool = sync.Pool{
	New: func() interface{} {
		r := make([]byte, MaxDatagram)
		return &r
	},
}

type Server struct {
	conn   *ipv4.PacketConn
	iface  *net.Interface
	opts   *options.Options // core for dhcp's options add/update
	errors chan error
}

// Wait waits until the end of the execution of the server.
func (s *Server) Wait() error {
	log.Debug("Waiting")
	err := <-s.errors

	s.conn.Close()

	return err
}

// server asynchronously start. See `Wait` to wait until the execution ends.
func Start(cfg *base.Config) (*Server, error) {
	log.Println("Starting DHCPv4 server")
	// init ops = load options prepare dhcp options recv send
	srv := &Server{}
	srv.opts = options.New(cfg)

	// init conn,iface = ipv4.PacketConn, multicast ip
	addr := cfg.Address()
	udpConn, err := server4.NewIPv4UDPConn(addr.Zone, &addr)
	if err != nil {
		return srv, err
	}
	srv.conn = ipv4.NewPacketConn(udpConn)
	srv.iface, err = net.InterfaceByName(addr.Zone)
	if err != nil {
		srv.conn.Close()
		return srv, fmt.Errorf("DHCPv4: Listen could not find interface %s: %v", addr.Zone, err)
	}

	if addr.IP.IsMulticast() {
		err = srv.conn.JoinGroup(srv.iface, &addr)
		if err != nil {
			srv.conn.Close()
			return srv, err
		}
	}

	srv.errors = make(chan error)

	go srv.listen()

	return srv, err
}

func (s *Server) reqFromRecv4() (*dhcpv4.DHCPv4, *ipv4.ControlMessage) {
	b := *bufpool.Get().(*[]byte)
	b = b[:MaxDatagram] //Reslice to max capacity in case the buffer in pool was resliced smaller

	log.Printf("ipv4.PacketConn.ReadFrom wating...")

	n, oob, _, err := s.conn.ReadFrom(b)
	log.Printf("ipv4.PacketConn.ReadFrom: %d", n)
	if err != nil {
		log.Errorf("Error reading from connection: %v", err)
		s.errors <- err
	}

	// new req from recv buf
	b = b[:n]
	req, err := dhcpv4.FromBytes(b)
	bufpool.Put(&b)
	if err != nil {
		log.Printf("Error parsing DHCPv4 request: %v", err)
		s.errors <- err
	}
	return req, oob
}

func (s *Server) listen() {
	log.Printf("Listen %s", s.conn.LocalAddr())
	for {
		req, oob := s.reqFromRecv4()
		log.Printf("reqFromRecv4: %v", req)
		go func() {
			// pretranslate req and oob
			resp, err := s.respFromReq4(req, oob)
			if err != nil {
				log.Println(err)
				return
			}

			s.opts.Handle(req, resp)

			s.sendResp(req, resp, oob)
		}()
	}
}

func (s *Server) respFromReq4(req *dhcpv4.DHCPv4, oob *ipv4.ControlMessage) (resp *dhcpv4.DHCPv4, err error) {
	// verify: constants that represent valid values for OpcodeType
	if req.OpCode != dhcpv4.OpcodeBootRequest {
		err = fmt.Errorf("RecvMsg4: unsupported opcode %d. Only support %d", req.OpCode, dhcpv4.OpcodeBootRequest)
		return
	}

	// add resp option, Discover=>Offer, Request=>Ack
	resp, err = dhcpv4.NewReplyFromRequest(req)
	if err != nil {
		err = fmt.Errorf("MainHandler4: failed to build reply: %v", err)
		return
	}

	optType := dhcpv4.MessageTypeNone
	switch mt := req.MessageType(); mt {
	case dhcpv4.MessageTypeDiscover:
		optType = dhcpv4.MessageTypeOffer
	case dhcpv4.MessageTypeRequest:
		optType = dhcpv4.MessageTypeAck
	default:
		err = fmt.Errorf("Unhandled message type: %v", mt)
		return
	}
	resp.UpdateOption(dhcpv4.OptMessageType(optType))

	if resp == nil || err != nil {
		err = fmt.Errorf("RecvMsg4: dropping request because response is nil")
	}

	return
}

// TODO hotfix reload
// func GetUpdate(pluginName string) plugins.UpdateFunc {
// }
