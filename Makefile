.DEFAULT_GOAL := build
bin=loxilb

loxilbid=$(shell docker ps -f name=loxilb | cut  -d " "  -f 1 | grep -iv  "CONTAINER")

subsys:
	cd ebpf && $(MAKE) 

subsys-clean:
	cd ebpf && $(MAKE) clean

build: subsys
	@go build -o ${bin} -ldflags="-X 'main.buildInfo=${shell date '+%Y_%m_%d'}-${shell git branch --show-current}'"

clean: subsys-clean
	go clean

test:
	go test .

check:
	go test .

run:
	./$(bin)

docker-cp: build
	docker cp loxilb $(loxilbid):/root/loxilb-io/loxilb/loxilb
	docker cp /opt/loxilb/llb_ebpf_main.o $(loxilbid):/opt/loxilb/llb_ebpf_main.o
	docker cp /opt/loxilb/llb_xdp_main.o $(loxilbid):/opt/loxilb/llb_xdp_main.o

docker: 
	docker build -t loxilb-io/loxilb .

lint:
	golangci-lint run --enable-all
