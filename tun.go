package main

import (
	"fmt"
	"github.com/songgao/water"
	"github.com/vishvananda/netlink"
	"net"
	"os/exec"
)

func (client *Client) SetTunOnDevice() error {
	ifce, err := water.New(water.Config{DeviceType: water.TUN,
		PlatformSpecificParams: water.PlatformSpecificParams{Name: "tun0"},
	})
	if err != nil {
		return err
	}
	client.TunInterface = ifce
	return nil
}

//
//func (client *Client) StartVPNClient() error {
//	err := client.SetTunOnDevice()
//	if err != nil{
//		return fmt.Errorf("error whilst creating tun device: %v", err)
//	}
//	if err != nil {
//		return err
//	}
//	err = app.AssignIPToTun()
//	if err != nil {
//		return err
//	}
//
//	err = app.CreateRoutes()
//	if err != nil {
//		return err
//	}
//
//	packet := make([]byte, 65535)
//	clientConn, err := net.Dial("udp", app.Config.ServerAddress)
//
//	if err != nil {
//		return err
//	}
//	defer clientConn.Close()
//
//	//receive
//	go func() {
//		for {
//			packet := make([]byte, 65535)
//			n, err := clientConn.Read(packet)
//			if err != nil {
//				log.Println(err)
//				continue
//			}
//			_, err = vpnClient.TunInterface.Write(packet[:n])
//			if err != nil {
//				log.Println(err)
//				continue
//			}
//		}
//	}()
//
//	//send
//	for {
//		n, err := vpnClient.TunInterface.Read(packet)
//		if err != nil {
//			log.Println(err)
//			break
//		}
//
//		_, err = clientConn.Write(packet[:n])
//		if err != nil {
//			log.Println(err)
//			continue
//		}
//	}
//	return nil
//}
//
//func (app *App) StartVPNServer() error {
//	vpnServer := server.NewServer(app.Config)
//	err := vpnServer.SetTunOnDevice()
//	if err != nil {
//		return err
//	}
//	vpnServer.ConnMap = cmap.New[net.Addr]()
//	err = app.AssignIPToTun()
//	if err != nil {
//		return err
//	}
//
//	err = app.CreateRoutes()
//	if err != nil {
//		return err
//	}
//
//	localAddress, _ := strconv.Atoi(app.Config.LocalAddress)
//	serverConn, err := net.ListenUDP("udp", &net.UDPAddr{Port: localAddress})
//	if err != nil {
//		return err
//	}
//	defer serverConn.Close()
//	go func() {
//		for {
//			packet := make([]byte, 65535)
//
//			n, clientAddr, err := serverConn.ReadFrom(packet)
//			if err != nil {
//				log.Println(err)
//				continue
//			}
//			sourceIPAddress := utils.ResolveSourceIPAddressFromRawPacket(packet)
//			vpnServer.ConnMap.Set(sourceIPAddress, clientAddr)
//			_, err = vpnServer.TunInterface.Write(packet[:n])
//			if err != nil {
//				log.Println(err)
//				continue
//			}
//		}
//	}()
//
//	for {
//		packet := make([]byte, 1500)
//		n, err := vpnServer.TunInterface.Read(packet)
//		if err != nil {
//			log.Println(err)
//			break
//		}
//		destinationIPAddress := utils.ResolveDestinationIPAddressFromRawPacket(packet)
//		destinationUDPAddress, ok := vpnServer.ConnMap.Get(destinationIPAddress)
//		if ok {
//			_, err = serverConn.WriteToUDP(packet[:n], destinationUDPAddress.(*net.UDPAddr))
//			if err != nil {
//				log.Println(err)
//				continue
//			}
//			vpnServer.ConnMap.Remove(destinationIPAddress)
//		}
//	}
//	return nil
//}

func (client *Client) AssignIPToTun() error {
	tunLink, err := netlink.LinkByName("tun0")
	if err != nil {
		return fmt.Errorf("error when getting tun link by name: %w", err)
	}

	parsedTunIPAddress, err := netlink.ParseAddr(client.Config.ClientTunIP)
	if err != nil {
		return fmt.Errorf("error when parsing tun ip address: %w", err)
	}

	err = netlink.AddrAdd(tunLink, parsedTunIPAddress)
	if err != nil {
		return fmt.Errorf("error when adding tun ip to tun interface: %w", err)
	}

	err = netlink.LinkSetUp(tunLink)
	if err != nil {
		return fmt.Errorf("error when bringing tun device online: %w", err)
	}

	return nil
}

func (client *Client) CreateTunnelRoute() error {
	_, network, err := net.ParseCIDR(client.Config.ClientTunIP)
	if err != nil {
		return fmt.Errorf("error parsing tun cidr: %w", err)
	}
	routeTrafficToDestinationThroughTun := exec.Command("sudo", "ip", "route", "add", network.String(), "dev", "tun0")
	_, err = routeTrafficToDestinationThroughTun.Output()
	if err != nil {
		return fmt.Errorf("error establishing tunnel route: %w", err)
	}
	return nil
}
