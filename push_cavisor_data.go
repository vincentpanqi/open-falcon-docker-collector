package main

import (
	"fmt"
	docker_types "github.com/docker/docker/api/types"
	docker "github.com/docker/docker/client"
	"github.com/google/cadvisor/client"
	info "github.com/google/cadvisor/info/v1"
	"golang.org/x/net/context"
	"io/ioutil"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

var (
	CPUNum   int64
	countNum int
)

func getCadvisorData() ([]info.ContainerInfo, error) {
	client, err := client.NewClient("http://127.0.0.1:18080/")
	if err != nil {
		return nil, err
	}
	request := info.ContainerInfoRequest{NumStats: -1}
	cadvisorData, err := client.AllDockerContainers(&request)
	if err != nil {
		return nil, err
	}

	return cadvisorData, nil
}

func getDockerContainerInfo(containerId string) (ContainerInfo docker_types.ContainerJSON, err error) {
	defaultHeaders := map[string]string{"User-Agent": "engine-api-cli-1.0"}
	client, err := docker.NewClient("unix:///var/run/docker.sock", "v1.24", nil, defaultHeaders)
	if err != nil {
		return
	}
	ContainerInfo, err = client.ContainerInspect(context.Background(), containerId)
	if err != nil {
		return
	}
	return
}

func pushData() {
	cadvisorDatas, err := getCadvisorData()
	fmt.Println("test")
	if err != nil {
		fmt.Println(err)
		return
	}
	t := time.Now().Unix()
	timestamp := fmt.Sprintf("%d", t)

	for _, cadvisorData := range cadvisorDatas {
		memLimit := cadvisorData.Spec.Memory.Limit
		containerId := cadvisorData.Id

		dockerData, err := getDockerContainerInfo(containerId)
		if err != nil {
			fmt.Println(err)
			continue
		}
		endpoint := containerId

		getCPUNum(dockerData)
		tag := getTag()

		aUsage, bUsage := getUsageData(cadvisorData)

		CPUUsage1 := aUsage.Cpu
		CPUUsage2 := bUsage.Cpu
		if err := pushCPU(CPUUsage1, CPUUsage2, timestamp, tag, containerId, endpoint); err != nil { //get cadvisor data about CPU
			fmt.Println(err)
		}

		// disk io usage
		diskIoUsage := aUsage.DiskIo
		if err := pushDiskIO(diskIoUsage, timestamp, tag,
			containerId, endpoint); err != nil {
			fmt.Println(err)
		}

		// memoryuage
		memoryUsage := aUsage.Memory
		if err := pushMem(memLimit, memoryUsage, timestamp, tag, containerId, endpoint); err != nil { //get cadvisor data about Memery
			fmt.Println(err)
		}

		// network
		networkUsage1 := aUsage.Network
		networkUsage2 := bUsage.Network

		if err := pushNetwork(networkUsage1, networkUsage2, timestamp, tag, containerId, endpoint); err != nil { //get cadvisor data about Memery
			fmt.Println(err)
		}

	}
}

func getCPUNum(dockerData docker_types.ContainerJSON) {
	CPUNum = dockerData.HostConfig.CPUCount
	if CPUNum == 0 {
		CPUNum = 1
	}
}

func getTag() string {
	return ""
}

func getUsageData(cadvisorData info.ContainerInfo) (ausge, busge *info.ContainerStats) {
	stats := cadvisorData.Stats
	ausge = stats[0]
	if len(stats) < 10 {
		busge = stats[1]
		countNum = 1
	} else {
		busge = stats[10]
		countNum = 10
	}
	return
}

func pushCPU(CPUUsage1, CPUUsage2 info.CpuStats, timestamp, tags,
	containerId, endpoint string) (err error) {

	fmt.Println("push CPU")
	if err = pushCount("cpu.busy",
		CPUUsage1.Usage.Total,
		CPUUsage2.Usage.Total,
		timestamp, tags,
		containerId,
		endpoint,
		10000000*float64(CPUNum)); err != nil {
		return
	}

	if err := pushCount("cpu.user",
		CPUUsage1.Usage.User,
		CPUUsage2.Usage.User, timestamp, tags, containerId,
		endpoint, 10000000*float64(CPUNum)); err != nil {
		return err
	}

	if err := pushCount("cpu.system", CPUUsage1.Usage.System,
		CPUUsage2.Usage.System,
		timestamp, tags, containerId,
		endpoint, 10000000*float64(CPUNum)); err != nil {
		return err
	}

	for i, _ := range CPUUsage1.Usage.PerCpu {
		usage := CPUUsage2.Usage.PerCpu[i] - CPUUsage1.Usage.PerCpu[i]
		perCpuUsage := fmt.Sprintf("%f", float64(usage)/10000000)
		if err := pushIt(perCpuUsage,
			timestamp, "cpu.core.busy",
			tags+",core="+fmt.Sprint(i),
			containerId, "GAUGE",
			endpoint); err != nil {
			fmt.Println(err)
			return err
		}
	}
	return

}

func pushCount(metric string, usageA, usageB uint64, timestamp, tags,
	containerId, endpoint string, weight float64) (err error) {
	temp1 := uint64(usageA)
	temp2 := uint64(usageB)
	usage := float64(temp2-temp1) / float64(countNum) / weight
	value := fmt.Sprintf("%f", usage)
	if err = pushIt(value, timestamp, metric, tags, containerId, "GAUGE", endpoint); err != nil {
		fmt.Println(err)
		return
	}
	return

}

func pushIt(value, timestamp, metric, tags, containerId, counterType,
	endpoint string) error {

	postThing := `[{"metric": "` + metric + `", "endpoint": "docker-` +
		endpoint + `", "timestamp": ` + timestamp + `,"step": ` + "60" + `,"value": ` + value + `,"counterType": "` + counterType + `","tags": "` + tags + `"}]`
	//push data to falcon-agent
	url := "http://127.0.0.1:1988/v1/push"
	resp, err := http.Post(url,
		"application/x-www-form-urlencoded",
		strings.NewReader(postThing))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, err1 := ioutil.ReadAll(resp.Body)
	if err1 != nil {
		return err1
	}
	return nil
}

func pushDiskIO(diskIOStats info.DiskIoStats, timestamp, tags, containerId,
	endpoint string) error {
	fmt.Println("push Disk IO")
	if len(diskIOStats.IoServiceBytes) == 0 {
		if err := pushIt("0", timestamp, "disk.io"+
			".read_bytes",
			tags, containerId, "COUNTER", endpoint); err != nil {
			fmt.Println(err)
			return err
		}
		if err := pushIt(fmt.Sprint("0"), timestamp, "disk.io"+
			".write_bytes", tags, containerId, "COUNTER", endpoint); err != nil {
			fmt.Println(err)
			return err
		}
		return nil
	}
	iOServiceBytes := diskIOStats.IoServiceBytes[0]
	readUsage := iOServiceBytes.Stats["Read"]

	if err := pushIt(fmt.Sprint(readUsage), timestamp, "disk.io"+
		".read_bytes",
		tags, containerId, "COUNTER", endpoint); err != nil {
		fmt.Println(err)
		return err
	}

	writeUsage := iOServiceBytes.Stats["Write"]
	if err := pushIt(fmt.Sprint(writeUsage), timestamp, "disk.io"+
		".write_bytes", tags, containerId, "COUNTER", endpoint); err != nil {
		fmt.Println(err)
		return err
	}
	return nil
}

func pushMem(memLimit uint64, memoryStats info.MemoryStats, timestamp, tags,
	containerId, endpoint string) error {
	fmt.Println("push Mem")
	memUsageNum := memoryStats.Usage

	memUsage := float64(memUsageNum) / float64(memLimit)
	if err := pushIt(fmt.Sprint(memUsage), timestamp, "mem.memused.percent", tags, containerId, "GAUGE", endpoint); err != nil {
		fmt.Println(err)
		return err
	}
	if err := pushIt(fmt.Sprint(memUsageNum), timestamp, "mem.memused",
		tags, containerId, "GAUGE", endpoint); err != nil {
		fmt.Println(err)
		return err
	}

	if err := pushIt(fmt.Sprint(memLimit), timestamp, "mem.memtotal", tags, containerId, "GAUGE", endpoint); err != nil {
		fmt.Println(err)
		return err
	}
	memHotUsageNum := memoryStats.WorkingSet
	memHotUsage := float64(memHotUsageNum) / float64(memLimit)

	if err := pushIt(fmt.Sprint(memHotUsage), timestamp, "mem.memused.hot", tags, containerId, "GAUGE", endpoint); err != nil {
		fmt.Println(err)
		return err
	}
	return nil
}

func pushNetwork(networkUsage1, networkUsage2 info.NetworkStats, timestamp, tags,
	containerId,
	endpoint string) error {
	fmt.Println("push Network")
	if err := pushCount("net.if.in.bytes", networkUsage1.RxBytes,
		networkUsage2.RxBytes, timestamp, tags, containerId,
		endpoint, 1); err != nil {
		return err
	}

	if err := pushCount("net.if.in.packets", networkUsage1.RxPackets,
		networkUsage2.RxPackets, timestamp, tags, containerId,
		endpoint, 1); err != nil {
		return err
	}

	if err := pushCount("net.if.in.errors", networkUsage1.RxErrors,
		networkUsage2.RxErrors, timestamp, tags, containerId,
		endpoint, 1); err != nil {
		return err
	}

	if err := pushCount("net.if.in.dropped", networkUsage1.RxDropped,
		networkUsage2.RxDropped, timestamp, tags, containerId,
		endpoint, 1); err != nil {
		return err
	}

	if err := pushCount("net.if.out.bytes", networkUsage1.TxBytes,
		networkUsage2.TxBytes, timestamp, tags, containerId,
		endpoint, 1); err != nil {
		return err
	}

	if err := pushCount("net.if.out.packets", networkUsage1.TxPackets,
		networkUsage2.TxPackets, timestamp, tags, containerId,
		endpoint, 1); err != nil {
		return err
	}

	if err := pushCount("net.if.out.errors", networkUsage1.TxErrors,
		networkUsage2.TxErrors, timestamp, tags, containerId,
		endpoint, 1); err != nil {
		return err
	}

	if err := pushCount("net.if.out.dropped", networkUsage1.TxDropped,
		networkUsage2.TxDropped, timestamp, tags, containerId,
		endpoint, 1); err != nil {
		return err
	}

	return nil
}

func main() {
	tmp := os.Getenv("CADVISOR_INTERVAL")
	Interval := 60 * time.Second
	tmp1, err := strconv.ParseInt(tmp, 10, 64)
	if err == nil {
		Interval = time.Duration(tmp1) * time.Second
	}
	t := time.NewTicker(Interval)
	fmt.Println("start push_cavisor_data ok", Interval)
	for {
		pushData()
		fmt.Println("push data done. wait for next time.")
		<-t.C
	}
}