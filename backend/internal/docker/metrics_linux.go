//go:build linux

package docker

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"syscall"
	"time"
)

func cpuSample() (uint64, uint64, error) {
	file, err := os.Open("/proc/stat")
	if err != nil {
		return 0, 0, err
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	if !scanner.Scan() {
		return 0, 0, fmt.Errorf("CPU statistics unavailable")
	}
	fields := strings.Fields(scanner.Text())
	var total, idle uint64
	for i, field := range fields[1:] {
		if i >= 8 {
			break
		}
		value, e := strconv.ParseUint(field, 10, 64)
		if e != nil {
			return 0, 0, e
		}
		total += value
		if i == 3 || i == 4 {
			idle += value
		}
	}
	return total, idle, nil
}
func hostMetrics(ctx context.Context) (Result, error) {
	total1, idle1, err := cpuSample()
	if err != nil {
		return Result{}, err
	}
	select {
	case <-ctx.Done():
		return Result{}, ctx.Err()
	case <-time.After(200 * time.Millisecond):
	}
	total2, idle2, err := cpuSample()
	if err != nil {
		return Result{}, err
	}
	cpu := float64(0)
	if total2 > total1 {
		cpu = 100 * (1 - float64(idle2-idle1)/float64(total2-total1))
	}
	mem, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return Result{}, err
	}
	values := map[string]uint64{}
	for _, line := range strings.Split(string(mem), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 {
			n, _ := strconv.ParseUint(fields[1], 10, 64)
			values[strings.TrimSuffix(fields[0], ":")] = n * 1024
		}
	}
	var disk syscall.Statfs_t
	diskAvailable := syscall.Statfs("/host/docker", &disk) == nil
	out, err := command(ctx, "", nil, "docker", "info", "--format", `{"containers":{{.Containers}},"running":{{.ContainersRunning}},"cpus":{{.NCPU}}}`)
	if err != nil {
		return Result{}, err
	}
	var info map[string]any
	if err = json.Unmarshal([]byte(out), &info); err != nil {
		return Result{}, err
	}
	info["available"] = true
	info["cpu_percent"] = cpu
	info["memory_total"] = values["MemTotal"]
	info["memory_used"] = values["MemTotal"] - values["MemAvailable"]
	info["disk_available"] = diskAvailable
	if diskAvailable {
		info["disk_total"] = disk.Blocks * uint64(disk.Bsize)
		info["disk_used"] = (disk.Blocks - disk.Bavail) * uint64(disk.Bsize)
	}
	data, err := json.Marshal(info)
	return Result{Output: string(data)}, err
}
