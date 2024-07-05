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
const HEADER = 2
const DST_IP_START = 16
const DST_IP_STOP = 20

var clients map[netip.Addr]net.Conn
var mtu uint
var server *net.TCPConn
var serv bool

func HandleTun(ifce *water.Interface) {
	packet := make([]byte, mtu)
	result := make([]byte, lz4.CompressBlockBound(int(mtu))+HEADER)
	for {
		n, err := ifce.Read(packet)
		if err != nil {
			panic(err)
		}
		zn, err := lz4.CompressBlock(packet[:n], result[HEADER:], nil)
		if err != nil {
			panic(err)
		}
		if zn == 0 {
			log.Fatal("Could not compress!")

		}
		bs := make([]byte, HEADER)
		binary.LittleEndian.PutUint16(bs, uint16(zn))
		copy(result, bs)
		if !serv {
			server.Write(result[:zn+HEADER])

		} else {
			// Extract dest IP from packet bytes
			dst_ip, _ := netip.AddrFromSlice(packet[DST_IP_START:DST_IP_STOP])
			if dst_ip.IsMulticast() {
				for _, client := range clients {
					client.Write(result[:zn+HEADER])
				}
			} else {

				if client, ok := clients[dst_ip]; ok {

					client.Write(result[:zn+HEADER])
				}
			}
		}
	}
}
func HandleRemote(c net.Conn, ifce *water.Interface) {
	defer c.Close()
	log.Printf("Serving %s\n", c.RemoteAddr().String())
	packet := make([]byte, lz4.CompressBlockBound(int(mtu))+HEADER)
	result := make([]byte, mtu)
	stream := make([]byte, 0)

	for {
		left, err := c.Read(packet)
		if err != nil {
			log.Printf("Disconnect %s\n", c.RemoteAddr().String())
			c.Close()
			return
		}
		stream = append(stream, packet[:left]...)

		for len(stream) > HEADER {
			bs := binary.LittleEndian.Uint16(stream[:HEADER])
			if len(stream) < int(bs)+HEADER {
				break
			}
			n, err := lz4.UncompressBlock(stream[HEADER:bs+HEADER], result)
			if err != nil {
				log.Print(err)
				panic(err)
			}
			ifce.Write(result[:n])
			stream = stream[HEADER+bs:]
		}
	}
}

func assign_ip(c net.Conn, start netip.Addr) (netip.Addr, error) {
	c_ip := start
	for c_ip.IsValid() {
		if client, ok := clients[c_ip]; ok {
			_, err := client.Write(make([]byte, 0))
			if err == nil {
				c_ip = c_ip.Next()
			} else {
				break
			}
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
func config_link(name string, nl_addr *netlink.Addr) {
	link, err := netlink.LinkByName(name)
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
}
func main() {

	clients = make(map[netip.Addr]net.Conn)
	sSub := flag.String("subnet", "172.168.13.1/24", "Subnet that will be used for network.")
	sServ := flag.String("server", "", "ztun Server to connect to.")
	nPort := flag.Uint("port", 2024, "Port to use.")
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
		if i == 999 {
			panic("Can't allocate tun interface!")
		}
	}

	ifce, err := water.New(config)
	if err != nil {
		panic(err)
	}

	go HandleTun(ifce)

	if len(*sServ) > 0 {
		serv = false
		tcpAddr, err := net.ResolveTCPAddr("tcp", fmt.Sprintf("%s:%d", *sServ, *nPort))
		if err != nil {
			panic(err)
		}

		s, err := net.DialTCP("tcp4", nil, tcpAddr)
		if err != nil {
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

		if err != nil {
			panic(err)
		}
		nl_addr, err := netlink.ParseAddr(c_pre.String())
		if err != nil {
			panic(err)
		}
		config_link(config.Name, nl_addr)

		HandleRemote(s, ifce)

	} else {

		serv = true
		tcpAddr, err := net.ResolveTCPAddr("tcp", fmt.Sprintf("0.0.0.0:%d", *nPort))
		if err != nil {
			panic(err)
		}
		l, err := net.ListenTCP("tcp4", tcpAddr)

		if err != nil {
			panic(err)
		}
		defer l.Close()

		nlip, err := netlink.ParseAddr(ipr.String())

		if err != nil {
			panic(err)
		}
		config_link(config.Name, nlip)
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
						clients[c_ip] = c
						go HandleRemote(c, ifce)

					}
				}
			} else {
				panic(err)
			}
		}

	}

}
