package options

import (
	"bufio"
	"encoding/binary"
	"errors"
	"fmt"
	"minidhcp/base"
	"minidhcp/options/allocators"
	"minidhcp/options/allocators/bitmap"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/insomniacslk/dhcp/dhcpv4"
)

var log = base.GetLogger("options")

var roleName = []string{"staff", "guest", "boss"}

//Record holds an IP lease record
type Record struct {
	IP      net.IP
	expires time.Time
	role    string
}

type Options struct {
	// Rough lock for the whole plugin, we'll get better performance once we use leasestorage
	sync.Mutex
	// Recordsv4 holds a MAC -> IP address and lease time mapping
	Recordsv4  map[string]*Record
	leasefile  *os.File
	leaseTimes []time.Duration
	allocs     []allocators.Allocator

	conf    *base.Config
	subnets []base.Subnet
}

// TODO  serverid push 1st plugin
func New(conf *base.Config) *Options {
	subnets := []base.Subnet{conf.Staff, conf.Guest, conf.Boss}
	ops := Options{
		conf:    conf,
		subnets: subnets,
	}
	ops.Setup4(subnets)
	log.Infof("NewOptions subnets: %v", subnets)

	return &ops
}

func (o *Options) findSubnetIndex(req *dhcpv4.DHCPv4) int {
	mac := req.ClientHWAddr.String()
	record, ok := o.Recordsv4[mac]
	if !ok {
		// TODO rest client request controlcenter, GET /auth/mac=?
		return 0
	}
	for i, role := range roleName {
		if role == record.role {
			return i
		}
	}
	return 0
}

func (o *Options) Handle(req, resp *dhcpv4.DHCPv4) {
	idxSubnet := o.findSubnetIndex(req)
	o.Handler4(req, resp, idxSubnet)
	o.Handler4Other(req, resp, idxSubnet)
	o.handler4ServerId(req, resp)
}

// Handler4 handles DHCPv4 packets for the range plugin
func (o *Options) Handler4(req, resp *dhcpv4.DHCPv4, idxSubnet int) {
	o.Lock()
	defer o.Unlock()
	alloc, leasetime := o.allocs[idxSubnet], o.leaseTimes[idxSubnet]
	mac := req.ClientHWAddr.String()
	record, ok := o.Recordsv4[mac]
	if !ok {
		rec := o.createNewIP(alloc, mac, leasetime, roleName[idxSubnet])
		err := o.saveIPAddress(req.ClientHWAddr, rec)
		if rec == nil || err != nil {
			log.Errorf("SaveIPAddress for MAC %s failed: %v", mac, err)
			return
		}
		o.Recordsv4[mac] = rec
		record = rec
	} else {
		// Ensure we extend the existing lease at least past when the one we're giving expires
		if record.expires.Before(time.Now().Add(leasetime)) {
			record.expires = time.Now().Add(leasetime).Round(time.Second)
			err := o.saveIPAddress(req.ClientHWAddr, record)
			if err != nil {
				log.Errorf("Could not persist lease for MAC %s: %v", mac, err)
				return
			}
		}
	}
	resp.YourIPAddr = record.IP
	resp.Options.Update(dhcpv4.OptIPAddressLeaseTime(leasetime.Round(time.Second)))
	log.Printf("found IP address %s for MAC %s", record.IP, mac)
}

func (o *Options) Handler4Other(req, resp *dhcpv4.DHCPv4, idxSubnet int) {
	subnet := o.subnets[idxSubnet]
	resp.Options.Update(dhcpv4.OptSubnetMask(net.IPMask(net.ParseIP(subnet.Netmask).To4())))
	resp.Options.Update(dhcpv4.OptRouter(net.ParseIP(subnet.Router)))
	resp.Options.Update(dhcpv4.OptDNS(net.ParseIP(subnet.Dns)))
}

func (o *Options) handler4ServerId(req, resp *dhcpv4.DHCPv4) {
	if req.OpCode != dhcpv4.OpcodeBootRequest {
		log.Warningf("not a BootRequest, ignoring")
		return
	}
	srvid := net.ParseIP(o.conf.ServerId)
	if req.ServerIPAddr != nil &&
		!req.ServerIPAddr.Equal(net.IPv4zero) &&
		!req.ServerIPAddr.Equal(srvid) {
		// This request is not for us, drop it.
		log.Infof("requested server ID does not match this server's ID. Got %v, want %v", req.ServerIPAddr, srvid)
		return
	}
	resp.ServerIPAddr = make(net.IP, net.IPv4len)
	copy(resp.ServerIPAddr[:], srvid)
	resp.UpdateOption(dhcpv4.OptServerIdentifier(srvid))
}

// TODO，reentry
func (o *Options) Setup4(subnets []base.Subnet) (err error) {
	o.subnets = subnets
	// new allocs/leasetimes array
	for _, sub := range subnets {
		alloc, err := o.createAllocator(sub.IpStart, sub.IpStop)
		if err != nil {
			return fmt.Errorf("could not create an allocator: %w", err)
		}
		o.allocs = append(o.allocs, alloc)

		leaseTime, err := time.ParseDuration(sub.LeaseTime)
		if err != nil {
			return fmt.Errorf("invalid lease duration: %v", sub.LeaseTime)
		}
		o.leaseTimes = append(o.leaseTimes, leaseTime)
	}

	file, err := os.OpenFile("lease.txt", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("failed to open lease file %s: %w", "lease.txt", err)
	}
	o.leasefile = file

	r, err := o.loadRecords()
	if err != nil {
		return fmt.Errorf("could not load records from file: %v", err)
	}
	o.Recordsv4 = r

	log.Printf("Loaded %d DHCPv4 leases from lease.txt", len(o.Recordsv4))
	return
}

func (o *Options) createNewIP(allocator allocators.Allocator, mac string, leaseTime time.Duration, roleName string) *Record {
	// Allocating new address since there isn't one allocated
	log.Printf("MAC address %s is new, leasing new IPv4 address", mac)
	ip, err := allocator.Allocate(net.IPNet{})
	if err != nil {
		log.Errorf("Could not allocate IP for MAC %s: %v", mac, err)
		return nil
	}
	rec := Record{
		IP:      ip.IP.To4(),
		expires: time.Now().Add(leaseTime),
		role:    roleName,
	}
	return &rec
}

func (o *Options) createAllocator(ipstart, ipend string) (allocators.Allocator, error) {
	ipRangeStart := net.ParseIP(ipstart)
	if ipRangeStart.To4() == nil {
		return nil, fmt.Errorf("invalid IPv4 address: %v", ipstart)
	}
	ipRangeEnd := net.ParseIP(ipend)
	if ipRangeEnd.To4() == nil {
		return nil, fmt.Errorf("invalid IPv4 address: %v", ipend)
	}
	if binary.BigEndian.Uint32(ipRangeStart.To4()) >= binary.BigEndian.Uint32(ipRangeEnd.To4()) {
		return nil, errors.New("start of IP range has to be lower than the end of an IP range")
	}

	allocator, err := bitmap.NewIPv4Allocator(ipRangeStart, ipRangeEnd)

	return allocator, err
}

// TODO, role
func (o *Options) loadRecords() (map[string]*Record, error) {
	sc := bufio.NewScanner(o.leasefile)
	records := make(map[string]*Record)
	for sc.Scan() {
		line := sc.Text()
		if len(line) == 0 {
			continue
		}
		tokens := strings.Fields(line)
		if len(tokens) != 4 {
			return nil, fmt.Errorf("malformed line, want 4 fields, got %d: %s", len(tokens), line)
		}

		hwaddr, err := net.ParseMAC(tokens[0])
		if err != nil {
			return nil, fmt.Errorf("malformed hardware address: %s", tokens[0])
		}

		ipaddr := net.ParseIP(tokens[1])
		if ipaddr.To4() == nil {
			return nil, fmt.Errorf("expected an IPv4 address, got: %v", ipaddr)
		}

		expires, err := strconv.ParseInt(tokens[2], 10, 64)
		if err != nil {
			return nil, fmt.Errorf("expected time of exipry in unix timestamp int64 sec format, got: %v", tokens[2])
		}
		tm := time.Unix(expires, 0)

		role := tokens[3]

		records[hwaddr.String()] = &Record{IP: ipaddr, expires: tm, role: role}
	}
	return records, nil
}

// saveIPAddress writes out a lease to storage
func (o *Options) saveIPAddress(mac net.HardwareAddr, rec *Record) error {
	s := fmt.Sprintf("%s %s %d %s\n", mac.String(), rec.IP.String(), rec.expires.Unix(), rec.role)
	_, err := o.leasefile.WriteString(s)
	if err != nil {
		return fmt.Errorf("leasefile.WriteString() %s: %s", err, s)
	}
	err = o.leasefile.Sync()
	if err != nil {
		return fmt.Errorf("leasefile.Sync() %s", err)
	}
	return nil
}
