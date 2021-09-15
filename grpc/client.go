package grpc

import (
	"context"
	"log"
	"net"

	"github.com/BENSARI-Fathi/cni/v1/pb"
	"google.golang.org/grpc"
)

var socketFile = "/tmp/my-ipam.sock"

func UnixConnect(context.Context, string) (net.Conn, error) {
	unixAddress, _ := net.ResolveUnixAddr("unix", socketFile)
	conn, err := net.DialUnix("unix", nil, unixAddress)
	return conn, err
}

func NewGrpcClient() pb.IpamClient {
	conn, err := grpc.Dial(socketFile, grpc.WithInsecure(), grpc.WithContextDialer(UnixConnect))
	if err != nil {
		log.Fatalf("Error while opening Unix connexion %s", err.Error())
	}
	return pb.NewIpamClient(conn)
}
