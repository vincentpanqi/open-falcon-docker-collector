package main

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"time"
)

var Interval time.Duration // 检测时间间隔

func main() {
	tmp := os.Getenv("CADVISOR_INTERVAL")
	Interval = 60 * time.Second
	tmp1, err := strconv.ParseInt(tmp, 10, 64)
	if err == nil {
		Interval = time.Duration(tmp1) * time.Second
	}

	cadvisorPath := os.Getenv("CADVISOR_PATH")
	cadvisorPort := os.Getenv("CADVISOR_PORT")
	if len(cadvisorPath) == 0 {
		cadvisorPath = "./cadvisor"
	}
	if len(cadvisorPort) == 0 {
		cadvisorPort = "18080"
	}
	cmd := exec.Command(cadvisorPath, "-port="+cadvisorPort)
	if err = cmd.Start(); err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println("start cadvisor ok", Interval)

	pushDataPath := os.Getenv("PUSHDATAPATH")
	if len(pushDataPath) == 0 {
		pushDataPath = "./push_cavisor_data"
	}

	go func() {
		t := time.NewTicker(Interval)
		fmt.Println("start push_cavisor_data ok", Interval)
		for {
			<-t.C
			cmd = exec.Command(pushDataPath)
			if err := cmd.Start(); err != nil {
				fmt.Println(err)
				return
			}
			cmd.Wait()
		}

	}()
	for {
		time.Sleep(time.Second * 120)
		if isAlive() {
			clean()
		} else {
			os.Exit(1)
		}
	}
}

func isAlive() bool {
	f, _ := os.OpenFile("test.txt", os.O_CREATE|os.O_APPEND|os.O_RDONLY, 0660)
	defer f.Close()
	read_buf := make([]byte, 32)
	var pos int64 = 0
	n, _ := f.ReadAt(read_buf, pos)
	if n == 0 {
		return false
	}
	return true
}

func clean() {
	f, _ := os.OpenFile("test.txt", os.O_TRUNC, 0660)
	defer f.Close()
}
