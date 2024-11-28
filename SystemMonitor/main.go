package main

import (
	"encoding/json"
	"github.com/shirou/gopsutil/cpu"
	"github.com/shirou/gopsutil/disk"
	"github.com/shirou/gopsutil/host"
	"github.com/shirou/gopsutil/mem"
	"github.com/shirou/gopsutil/net"
	"log"
	"net/http"
	"sync"
	"time"
)

type SystemMetrics struct {
	Timestamp   int64   `json:"timestamp"`
	CPUUsage    float64 `json:"cpu"`
	MemoryUsage float64 `json:"memory"`
	DiskUsage   float64 `json:"disk"`
	NetDownload float64 `json:"download"`
	NetUpload   float64 `json:"upload"`
	UptimeHours float64 `json:"uptime_hours"`
}

type NetworkState struct {
	lastBytesRecv uint64
	lastBytesSent uint64
	lastCheckTime time.Time
	mutex         sync.Mutex
}

var (
	metrics      []SystemMetrics
	networkState NetworkState
)

func main() {
	metrics = make([]SystemMetrics, 0)

	networkState = NetworkState{
		lastCheckTime: time.Now(),
	}

	if netStats, err := net.IOCounters(false); err == nil && len(netStats) > 0 {
		networkState.lastBytesRecv = netStats[0].BytesRecv
		networkState.lastBytesSent = netStats[0].BytesSent
	}

	go collectMetrics()

	http.Handle("/", http.FileServer(http.Dir(".")))

	http.HandleFunc("/metrics", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(metrics)
	})

	log.Println("Server starting on :8080...")
	log.Fatal(http.ListenAndServe(":8080", nil))
}

func getSystemUptime() float64 {
	uptime, err := host.Uptime()
	if err != nil {
		return 0
	}
	// Convert uptime from seconds to hours
	return float64(uptime) / 3600
}

func getNetworkSpeeds() (float64, float64) {
	networkState.mutex.Lock()
	defer networkState.mutex.Unlock()

	netStats, err := net.IOCounters(false)
	if err != nil || len(netStats) == 0 {
		return 0, 0
	}

	now := time.Now()
	duration := now.Sub(networkState.lastCheckTime).Seconds()

	if duration < 0.1 {
		return 0, 0
	}

	bytesRecvDiff := float64(netStats[0].BytesRecv - networkState.lastBytesRecv)
	bytesSentDiff := float64(netStats[0].BytesSent - networkState.lastBytesSent)

	downloadSpeed := (bytesRecvDiff / duration) / 1024 / 1024
	uploadSpeed := (bytesSentDiff / duration) / 1024 / 1024

	networkState.lastBytesRecv = netStats[0].BytesRecv
	networkState.lastBytesSent = netStats[0].BytesSent
	networkState.lastCheckTime = now

	if downloadSpeed < 0 {
		downloadSpeed = 0
	}
	if uploadSpeed < 0 {
		uploadSpeed = 0
	}

	return downloadSpeed, uploadSpeed
}

func collectMetrics() {
	ticker := time.NewTicker(1 * time.Second)
	for range ticker.C {
		metric := SystemMetrics{
			Timestamp: time.Now().Unix(),
		}

		if cpuPercent, err := cpu.Percent(0, false); err == nil && len(cpuPercent) > 0 {
			metric.CPUUsage = cpuPercent[0]
		}

		if vmStat, err := mem.VirtualMemory(); err == nil {
			metric.MemoryUsage = vmStat.UsedPercent
		}

		if diskStat, err := disk.Usage("/"); err == nil {
			metric.DiskUsage = diskStat.UsedPercent
		}

		metric.NetDownload, metric.NetUpload = getNetworkSpeeds()
		metric.UptimeHours = getSystemUptime()

		metrics = append(metrics, metric)
		if len(metrics) > 3600 {
			metrics = metrics[1:]
		}
	}
}
