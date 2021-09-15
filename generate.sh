#!/bin/bash

protoc --go-grpc_out=. --go_out=. v1/pb/ipam.proto