package utils

import (
	"fmt"
	"log"
	"syscall"

	current "github.com/containernetworking/cni/pkg/types/100"
	"github.com/vishvananda/netlink"
)

// This is overridden in the linker script
var BuildVersion = "version unknown"

const LinkNotFound = "Link not found"

func BuildString(pluginName string) string {
	return fmt.Sprintf("CNI %s plugin %s", pluginName, BuildVersion)
}

func GetOrCreateBridge(name, addr string) (netlink.Link, *current.Interface, error) {
	l, err := netlink.LinkByName(name)
	if err != nil && err.Error() == LinkNotFound {
		return CreateBridge(name, addr)
	}
	brIface := &current.Interface{
		Name: l.Attrs().Name,
		Mac:  l.Attrs().HardwareAddr.String(),
	}
	return l, brIface, nil
}

func CreateBridge(name, addr string) (netlink.Link, *current.Interface, error) {
	br := &netlink.Bridge{
		LinkAttrs: netlink.LinkAttrs{
			Name: name,
			MTU:  1500,
			//  Let kernel use default txqueuelen; leaving it unset
			//  means 0, and a zero-length TX queue messes up FIFO
			//  traffic shapers which use TX queue length as the
			//  default packet limit
			TxQLen: -1,
		},
	}
	err := netlink.LinkAdd(br)
	if err != nil && err != syscall.EEXIST {
		log.Printf("Error while adding bridge %s", err.Error())
		return nil, nil, err
		// log.Fatalf("Error while adding bridge %s", err.Error())
	}
	if err = netlink.LinkSetUp(br); err != nil {
		log.Printf("Error while setting link up 52 %s", err.Error())
		return nil, nil, err
		// log.Fatal("Error while setting interface up", err.Error())
	}
	l, _ := netlink.LinkByName(br.Name)
	// alocate an ip to the bridge
	brIP, err := netlink.ParseAddr(addr)
	if err != nil {
		log.Printf("Error here 60 %s", err.Error())
	}
	if err = netlink.AddrAdd(l, brIP); err != nil {
		return nil, nil, err
	}
	brIface := &current.Interface{
		Name: l.Attrs().Name,
		Mac:  l.Attrs().HardwareAddr.String(),
	}
	return l, brIface, nil
}

// l, _ := netlink.LinkByName(br.Name)
// // alocate an ip to the bridge
// brAddr := fmt.Sprintf("%s/%d", addResp.Gateway, addResp.NetMask)
// brIP, _ := netlink.ParseAddr(brAddr)
// if err = netlink.AddrAdd(br, brIP); err != nil {
// 	log.Fatal(err.Error())
// }
