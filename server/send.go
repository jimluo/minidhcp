package server

import (
	"fmt"
	"net"
	"syscall"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/insomniacslk/dhcp/dhcpv4"
	"golang.org/x/net/ipv4"
)

func (s *Server) makePeer(req, resp *dhcpv4.DHCPv4) (peer *net.UDPAddr, isSendPcap bool) {
	isSendPcap = false
	var ip net.IP
	var port int
	if !req.GatewayIPAddr.IsUnspecified() {
		// TODO: make RFC8357 compliant
		ip, port = req.GatewayIPAddr, dhcpv4.ServerPort
	} else if resp.MessageType() == dhcpv4.MessageTypeNak {
		ip, port = net.IPv4bcast, dhcpv4.ClientPort
	} else if !req.ClientIPAddr.IsUnspecified() {
		ip, port = req.ClientIPAddr, dhcpv4.ClientPort
	} else if req.IsBroadcast() {
		ip, port = net.IPv4bcast, dhcpv4.ClientPort
	} else {
		//sends a layer2 frame so that we can define the destination MAC address
		ip, port = resp.YourIPAddr, dhcpv4.ClientPort
		isSendPcap = true
	}
	peer = &net.UDPAddr{IP: ip, Port: port}

	return
}

func (s *Server) sendResp(req, resp *dhcpv4.DHCPv4, oob *ipv4.ControlMessage) {
	// Direct broadcasts, link-local and layer2 unicasts to the interface the request was received on.
	// Other packets should use the normal routing table in case of asymetric routing
	// if peer.IP.Equal(net.IPv4bcast) || peer.IP.IsLinkLocalUnicast() || useEthernet {
	var ifindex int
	if s.iface.Index != 0 {
		ifindex = s.iface.Index
	} else if oob != nil && oob.IfIndex != 0 {
		ifindex = oob.IfIndex
	} else {
		log.Println("HandleMsg4: Did not receive interface information")
	}

	peer, isSendPcap := s.makePeer(req, resp)

	if isSendPcap {
		intf, err := net.InterfaceByIndex(ifindex)
		if err != nil {
			log.Errorf("SendResp: Error get IfIndex %d %v", ifindex, err)
			return
		}
		log.Printf("InterfaceByIndex %v", ifindex, intf)
		err = s.sendEthernet(*intf, resp)
		if err != nil {
			log.Errorf("SendResp: Error send Ethernet packet: %v", err)
		}
	} else {
		woob := &ipv4.ControlMessage{IfIndex: ifindex} // ip4.conn 不使用cm oob
		n, err := s.conn.WriteTo(resp.ToBytes(), woob, peer)
		if err != nil {
			log.Errorf("SendResp: Error conn.Write %s bytes to %v failed: %v", n, peer, err)
		}
	}
}

//this function sends an unicast to the hardware address defined in resp.ClientHWAddr,
//the layer3 destination address is still the broadcast address;
//iface: the interface where the DHCP message should be sent;
//resp: DHCPv4 struct, which should be sent;
func (s *Server) sendEthernet(iface net.Interface, resp *dhcpv4.DHCPv4) error {

	eth := layers.Ethernet{
		EthernetType: layers.EthernetTypeIPv4,
		SrcMAC:       iface.HardwareAddr,
		DstMAC:       resp.ClientHWAddr,
	}
	if eth.SrcMAC == nil {
		eth.SrcMAC = net.HardwareAddr{0x00, 0x00, 0x00, 0x00, 0x00, 0x00}
		log.Println("eth.SrcMAC is nil [lo], set to lo mac 00:00:00:00:00:00", eth.SrcMAC)
	}

	ip := layers.IPv4{
		Version:  4,
		TTL:      64,
		SrcIP:    resp.ServerIPAddr,
		DstIP:    resp.YourIPAddr,
		Protocol: layers.IPProtocolUDP,
		Flags:    layers.IPv4DontFragment,
	}
	udp := layers.UDP{
		SrcPort: dhcpv4.ServerPort,
		DstPort: dhcpv4.ClientPort,
	}

	err := udp.SetNetworkLayerForChecksum(&ip)
	if err != nil {
		return fmt.Errorf("Send Ethernet: Couldn't set network layer: %v", err)
	}

	buf := gopacket.NewSerializeBuffer()
	opts := gopacket.SerializeOptions{
		ComputeChecksums: true,
		FixLengths:       true,
	}

	// Decode a packet
	packet := gopacket.NewPacket(resp.ToBytes(), layers.LayerTypeDHCPv4, gopacket.NoCopy)
	dhcpLayer := packet.Layer(layers.LayerTypeDHCPv4)
	dhcp, ok := dhcpLayer.(gopacket.SerializableLayer)
	if !ok {
		return fmt.Errorf("Layer %s is not serializable", dhcpLayer.LayerType().String())
	}
	err = gopacket.SerializeLayers(buf, opts, &eth, &ip, &udp, dhcp)
	if err != nil {
		return fmt.Errorf("Cannot serialize layer: %v, %v, %v, %v", err, eth, ip, udp)
	}
	data := buf.Bytes()

	// domain :=
	fd, err := syscall.Socket(syscall.AF_PACKET, syscall.SOCK_RAW, 0)
	if err != nil {
		return fmt.Errorf("Send Ethernet: Cannot open socket: %v", err)
	}
	defer func() {
		err = syscall.Close(fd)
		if err != nil {
			log.Errorf("Send Ethernet: Cannot close socket: %v", err)
		}
	}()

	err = syscall.SetsockoptInt(fd, syscall.SOL_SOCKET, syscall.SO_REUSEADDR, 1)
	if err != nil {
		log.Errorf("Send Ethernet: Cannot set option for socket: %v", err)
	}

	var hwAddr [8]byte
	copy(hwAddr[0:6], resp.ClientHWAddr[0:6])
	ethAddr := syscall.SockaddrLinklayer{
		Protocol: 0,
		Ifindex:  iface.Index,
		Halen:    6,
		Addr:     hwAddr, //not used
	}
	err = syscall.Sendto(fd, data, 0, &ethAddr)
	if err != nil {
		return fmt.Errorf("Cannot send frame via socket: %v", err)
	}
	return nil
}
