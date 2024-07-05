package main

import (
	"encoding/binary"
	"fmt"
	"log"
	"net"

	"flag"

	"errors"
	"net/netip"

	"github.com/pierrec/lz4/v4"
	"github.com/songgao/water"
	"github.com/vishvananda/netlink"
)

const MAX_MTU = 9000

var clients map[netip.Addr]net.Conn
var mtu uint
var server *net.TCPConn
var serv bool

func HandleTun(ifce *water.Interface) {
	packet := make([]byte, mtu)
	result := make([]byte, lz4.CompressBlockBound(int(mtu))+2)
	for {
		n, err := ifce.Read(packet)
		if err != nil {
			// log.Fatal(err)
			panic(err)
		}
		// log.Printf("Packet Received: %d bytes\n", n)
		zn, err := lz4.CompressBlock(packet[:n], result[2:], nil)
		if err != nil {
			// log.Fatal(err)
			panic(err)
		}
		if zn == 0 {
			log.Fatal("Could not compress!")

		}
		bs := make([]byte, 2)
		binary.LittleEndian.PutUint16(bs, uint16(zn))
		// log.Printf("Packet Compressed: %d bytes\n", zn)
		// log.Printf("Size: %d, CSize: %d", n, zn)
		copy(result, bs)
		if !serv {
			// server.Write(bs)
			server.Write(result[:zn+2])

		} else {
			// Extract dest IP from packet bytes
			dst_ip, _ := netip.AddrFromSlice(packet[16:20])

			if client, ok := clients[dst_ip]; ok {

				// client.Write(bs)

				client.Write(result[:zn+2])
			}
		}
	}
}
func HandleClient(c net.Conn, ifce *water.Interface, c_ip netip.Addr) {
	defer c.Close()
	log.Printf("Serving %s\n", c.RemoteAddr().String())
	packet := make([]byte, lz4.CompressBlockBound(int(mtu))+2)
	result := make([]byte, mtu)
	stream := make([]byte, 0)

	for {
		left, err := c.Read(packet)
		if err != nil {
			log.Printf("Disconnect %s\n", c.RemoteAddr().String())
			c.Close()
			delete(clients, c_ip)
			return
		}
		stream = append(stream, packet[:left]...)

		for len(stream) > 2 {
			bs := binary.LittleEndian.Uint16(stream[:2])
			if len(stream) < int(bs)+2 {
				// log.Printf("HERE %d %d", len(stream), int(bs)+2)
				break
			}
			n, err := lz4.UncompressBlock(stream[2:bs+2], result)
			// log.Printf("Size: %d, CSize: %d", n, bs)
			if err != nil {
				log.Print(err)
				panic(err)
			}
			ifce.Write(result[:n])
			stream = stream[2+bs:]
			// left -= (2 + int(bs))
		}
	}
}

func NewClient(s *net.TCPConn, ifce *water.Interface) {

	defer s.Close()
	log.Printf("Connected %s\n", s.RemoteAddr().String())
	packet := make([]byte, lz4.CompressBlockBound(int(mtu))+2)
	result := make([]byte, mtu)
	stream := make([]byte, 0)
	for {
		left, err := s.Read(packet)
		if err != nil {
			// log.Print(err)
			panic(err)
		}
		stream = append(stream, packet[:left]...)

		for len(stream) > 2 {
			bs := binary.LittleEndian.Uint16(stream[:2])
			if len(stream) < int(bs)+2 {
				// log.Printf("HERE %d %d", len(stream), int(bs)+2)
				break
			}
			n, err := lz4.UncompressBlock(stream[2:bs+2], result)
			// log.Printf("Size: %d, CSize: %d", n, bs)
			if err != nil {
				log.Print(err)
				panic(err)
			}
			ifce.Write(result[:n])
			stream = stream[2+bs:]
			// left -= (2 + int(bs))
		}
	}

}

func assign_ip(c net.Conn, start netip.Addr) (netip.Addr, error) {
	c_ip := start
	for c_ip.IsValid() {
		if _, ok := clients[c_ip]; ok {
			c_ip = c_ip.Next()
		} else {
			break
		}
	}
	if c_ip.IsValid() {
		clients[c_ip] = c
		return c_ip, nil
	}
	return c_ip, errors.New("out of IPs")
}

func main() {

	clients = make(map[netip.Addr]net.Conn)
	sSub := flag.String("subnet", "172.168.13.1/24", "Subnet that will be used for network.")
	sServ := flag.String("server", "", "ztun Server:port to connect to.")
	flag.UintVar(&mtu, "mtu", MAX_MTU, "Interface MTU. Larger MTU can help with compression.")
	flag.Parse()
	if mtu > MAX_MTU {
		log.Fatalf("MTU cannot exceed %d", MAX_MTU)
	}

	ipr := netip.MustParsePrefix(*sSub)

	config := water.Config{
		DeviceType: water.TUN,
	}
	for i := range 1000 {

		config.Name = fmt.Sprintf("ztun%d", i)
		_, err := netlink.LinkByName(config.Name)
		if err != nil {
			break
		}
	}

	ifce, err := water.New(config)
	if err != nil {
		// log.Fatal(err)
		panic(err)
	}

	go HandleTun(ifce)

	if len(*sServ) > 0 {
		serv = false
		tcpAddr, err := net.ResolveTCPAddr("tcp", *sServ)
		if err != nil {
			// log.Fatalf("ResolveTCPAddr failed: %s", err.Error())
			panic(err)
		}

		s, err := net.DialTCP("tcp4", nil, tcpAddr)
		if err != nil {
			// log.Fatalf("Dial failed: %s", err.Error())
			panic(err)
		}
		server = s
		c_addr := make([]byte, 5)
		n, err := s.Read(c_addr)
		if err != nil || n != 5 {
			log.Fatal("Faied to set IP!")
		}
		c_ip, _ := netip.AddrFromSlice(c_addr[:4])
		c_pre := netip.PrefixFrom(c_ip, int(c_addr[4]))
		link, err := netlink.LinkByName(config.Name)
		if err != nil {
			// log.Fatal(err)
			panic(err)
		}

		if err != nil {
			// log.Fatal(err)
			panic(err)
		}
		nl_addr, err := netlink.ParseAddr(c_pre.String())
		if err != nil {
			panic(err)
		}
		err = netlink.AddrAdd(link, nl_addr)
		if err != nil {
			panic(err)
		}
		err = netlink.LinkSetMTU(link, int(mtu))
		if err != nil {
			panic(err)
		}
		err = netlink.LinkSetUp(link)
		if err != nil {
			panic(err)
		}
		NewClient(s, ifce)

	} else {

		serv = true
		tcpAddr, err := net.ResolveTCPAddr("tcp", "0.0.0.0:2024")
		if err != nil {
			// log.Fatalf("ResolveTCPAddr failed: %s", err.Error())
			panic(err)
		}
		l, err := net.ListenTCP("tcp4", tcpAddr)

		if err != nil {
			// log.Fatal(err)
			panic(err)
		}
		defer l.Close()

		link, err := netlink.LinkByName(config.Name)
		if err != nil {
			// log.Fatal(err)
			panic(err)
		}
		nlip, err := netlink.ParseAddr(ipr.String())

		if err != nil {
			// log.Fatal(err)
			panic(err)
		}
		err = netlink.AddrAdd(link, nlip)
		if err != nil {
			panic(err)
		}
		err = netlink.LinkSetMTU(link, int(mtu))
		if err != nil {
			panic(err)
		}
		err = netlink.LinkSetUp(link)
		if err != nil {
			panic(err)
		}
		for {
			c, err := l.Accept()
			if err == nil {
				c_ip, err := assign_ip(c, ipr.Addr().Next())

				if err != nil {
					// Out of client IPs... reject
					c.Close()

				} else {
					c_pre := make([]byte, 5)
					c_addr := c_ip.AsSlice()
					copy(c_pre[:4], c_addr)
					c_pre[4] = byte(ipr.Bits())
					n, err := c.Write(c_pre)

					if err != nil || n != 5 {
						c.Close()
					} else {
						go HandleClient(c, ifce, c_ip)
						clients[c_ip] = c
					}
				}
			} else {
				panic(err)
			}
		}

	}

}
