package main

import (
	"context"
	"fmt"
	"io/fs"
	"io/ioutil"
	"log"
	"net"
	"os"
	"runtime"

	"github.com/BENSARI-Fathi/cni/conf"
	"github.com/BENSARI-Fathi/cni/grpc"
	"github.com/BENSARI-Fathi/cni/utils"
	"github.com/BENSARI-Fathi/cni/v1/pb"
	"github.com/containernetworking/cni/pkg/skel"
	"github.com/containernetworking/cni/pkg/types"
	current "github.com/containernetworking/cni/pkg/types/100"
	"github.com/containernetworking/cni/pkg/version"
	"github.com/containernetworking/plugins/pkg/ip"
	"github.com/containernetworking/plugins/pkg/ns"
	k8s_utils "github.com/containernetworking/plugins/pkg/utils"
	"github.com/vishvananda/netlink"
)

func init() {
	// this ensures that main runs only on main thread (thread group leader).
	// since namespace ops (unshare, setns) are done for a single thread, we
	// must ensure that the goroutine does not jump from OS thread to thread
	runtime.LockOSThread()
}

func cmdAdd(args *skel.CmdArgs) error {
	config := conf.LoadNetConf(args.StdinData)
	err := ioutil.WriteFile("/var/log/mylog.log", args.StdinData, fs.FileMode(os.O_APPEND))
	if err != nil {
		return err
	}

	ioutil.WriteFile("/var/log/myCni.log", []byte(fmt.Sprintf("%v", args.ContainerID)), os.ModeAppend.Perm())
	// get the ip address from the ipam plugin

	grpClient := grpc.NewGrpcClient()
	addReq := &pb.AddRequest{
		Subnet:      config.Plugin.Subnet,
		Gateway:     config.Plugin.Gateway,
		ContainerId: args.ContainerID,
	}

	addResp, err := grpClient.Add(context.Background(), addReq)
	if err != nil {
		log.Fatalf("Error when calling Add RPC %s", err.Error())
	}
	// Create a bridge
	brIP := fmt.Sprintf("%s/%d", config.Plugin.Gateway, addResp.GetNetMask())
	br, brIface, err := utils.GetOrCreateBridge(config.Plugin.Bridge, brIP)
	if err != nil {
		return fmt.Errorf("error on Bridge creation stage")
	}
	// Create a veth pair
	netns, err := ns.GetNS(args.Netns)
	if err != nil {
		return err
	}
	hostIface := &current.Interface{}
	containerIface := &current.Interface{}
	podIP := fmt.Sprintf("%s/%d", addResp.GetPodIp(), addResp.GetNetMask())
	var handler = func(hostNS ns.NetNS) error {
		// Warning the interface name length should not exceed 15
		lname := "namla-" + args.ContainerID[:9]
		hostVeth, containerVeth, err := ip.SetupVethWithName(args.IfName, lname, 1500, "", hostNS)
		if err != nil {
			return err
		}

		hostIface.Name = hostVeth.Name
		hostIface.Mac = hostVeth.HardwareAddr.String()
		containerIface.Name = containerVeth.Name
		containerIface.Mac = containerVeth.HardwareAddr.String()
		containerIface.Sandbox = netns.Path()

		link, err := netlink.LinkByName(containerVeth.Name)
		if err != nil {
			ioutil.WriteFile("/var/log/myVeth.log", []byte("same error 82\n"), os.ModeAppend)
			return err
		}
		addr, _ := netlink.ParseAddr(podIP)
		if err = netlink.AddrAdd(link, addr); err != nil {
			ioutil.WriteFile("/var/log/myVeth.log", []byte("same error 87\n"), os.ModeAppend)
			return err
		}
		defaultRoute := net.ParseIP(addResp.Gateway)
		if err := ip.AddDefaultRoute(defaultRoute, link); err != nil {
			ioutil.WriteFile("/var/log/myVeth.log", []byte("same error 92\n"), os.ModeAppend)
			return err
		}
		return nil
	}

	if err := netns.Do(handler); err != nil {
		return err
	}
	// attach each pair to the correct place
	hostVeth, err := netlink.LinkByName(hostIface.Name)
	if err != nil {
		ioutil.WriteFile("/var/log/myVeth.log", []byte("same error 104\n"), os.ModeAppend)
		return err
	}

	if err := netlink.LinkSetMaster(hostVeth, br); err != nil {
		ioutil.WriteFile("/var/log/myVeth.log", []byte("same error 109\n"), os.ModeAppend)
		return err
	}

	// prepare the result
	myIpNet := net.IPNet{
		IP:   net.ParseIP(addResp.GetPodIp()),
		Mask: net.CIDRMask(int(addResp.NetMask), 32),
	}
	myGw := net.ParseIP(addReq.Gateway)
	cifaceIdx := 2
	_, routeDefault, _ := net.ParseCIDR("0.0.0.0/0")
	result := &current.Result{
		CNIVersion: config.CniVersion,
		Interfaces: []*current.Interface{
			brIface,
			hostIface,
			containerIface,
		},
		IPs: []*current.IPConfig{
			{
				Address:   myIpNet,
				Gateway:   myGw,
				Interface: &cifaceIdx,
			},
		},
		Routes: []*types.Route{
			{
				Dst: *routeDefault,
				GW:  myGw,
			},
		},
		DNS: types.DNS{
			Nameservers: []string{
				addReq.Gateway,
			},
		},
	}

	// setup nat for external connexion
	chain := k8s_utils.FormatChainName("MYPOD", args.ContainerID)
	comment := k8s_utils.FormatComment("MYPOD", args.ContainerID)
	for _, ipc := range result.IPs {
		if err = ip.SetupIPMasq(&ipc.Address, chain, comment); err != nil {
			return err
		}
	}
	ioutil.WriteFile("/var/log/myVeth.log", []byte(fmt.Sprintf("%v\n", result)), os.ModeAppend)
	types.PrintResult(result, config.CniVersion)
	return nil
}

func cmdDel(args *skel.CmdArgs) error {
	// clear the ip address
	grpClient := grpc.NewGrpcClient()
	delReq := &pb.DelRequest{
		ContainerId: args.ContainerID,
	}
	if _, err := grpClient.Del(context.Background(), delReq); err != nil {
		return err
	}
	// cleear the ns if still present
	if args.Netns == "" { // this is the logical state since it's kubelet job to delete the ns
		return nil
	}
	var ipnets []*net.IPNet
	err := ns.WithNetNSPath(args.Netns, func(_ ns.NetNS) error {
		var err error
		ipnets, err = ip.DelLinkByNameAddr(args.IfName)
		if err != nil && err == ip.ErrLinkNotFound {
			return nil
		}
		return err
	})

	if err != nil {
		return err
	}
	chain := k8s_utils.FormatChainName("MYPOD", args.ContainerID)
	comment := k8s_utils.FormatComment("MYPOD", args.ContainerID)
	for _, ipn := range ipnets {
		if err := ip.TeardownIPMasq(ipn, chain, comment); err != nil {
			return err
		}
	}
	return nil
}

func main() {
	skel.PluginMain(cmdAdd, nil, cmdDel, version.All, utils.BuildString("myBridge"))
}
