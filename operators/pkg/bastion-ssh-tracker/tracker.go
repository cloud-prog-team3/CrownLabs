package bastionsshtracker

import (
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/google/gopacket/pcap"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type SSHConnection struct {
	SourceIP   string
	SourcePort uint16
	DestIP     string
	DestPort   uint16
	StartTime  time.Time
}

type ConnectionEvent struct {
	ConnID string
	Conn   *SSHConnection
}

var (
	sshConnectionsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "ssh_connections_total",
			Help: "Total number of SSH connections detected",
		},
		[]string{"destination_ip", "destination_port", "source_ip"},
	)

	activeSSHConnections = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "ssh_connections_active",
			Help: "Current number of active SSH connections",
		},
		[]string{"destination_ip"},
	)
)

func init() {
	// Register metrics with Prometheus
	prometheus.MustRegister(sshConnectionsTotal)
	prometheus.MustRegister(activeSSHConnections)
}

func Tracker(iface string, port int, snaplen int) error {
	// Setup signal handling for graceful shutdown
	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, syscall.SIGINT, syscall.SIGTERM)

	// Start HTTP server for Prometheus metrics
	go func() {
		http.Handle("/metrics", promhttp.Handler())
		err := http.ListenAndServe(":8080", nil)
		if err != nil {
			fmt.Printf("Error starting metrics server: %v\n", err)
		}
	}()

	// Use afpacket instead of pcap
	szFrame, szBlock, numBlocks, err := afpacketComputeSize(8, snaplen, os.Getpagesize())
	if err != nil {
		return fmt.Errorf("error computing afpacket size: %v", err)
	}

	afHandle, err := newAfpacketHandle(iface, szFrame, szBlock, numBlocks, false, pcap.BlockForever)
	if err != nil {
		return fmt.Errorf("error creating afpacket handle: %v", err)
	}
	defer afHandle.Close()

	// Set BPF filter for outbound SSH traffic
	filter := fmt.Sprintf("tcp dst port %d", port)
	if err := afHandle.SetBPFFilter(filter, snaplen); err != nil {
		return fmt.Errorf("error setting BPF filter: %v", err)
	}

	source := gopacket.ZeroCopyPacketDataSource(afHandle)

	eventQueue := make(chan ConnectionEvent, 100)

	connections := make(map[string]bool)
	var connLock sync.RWMutex

	var wg sync.WaitGroup
	numWorkers := 1
	stopWorkers := make(chan struct{})
	stopPackets := make(chan struct{})

	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for {
				select {
				case event := <-eventQueue:
					handleEvent(event)
				case <-stopWorkers:
					return
				}
			}
		}(i)
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stopPackets:
				return
			default:
				data, _, err := source.ZeroCopyReadPacketData()
				if err != nil {
					continue
				}
				packet := gopacket.NewPacket(data, layers.LayerTypeEthernet, gopacket.Default)
				processPacket(&packet, connections, &connLock, eventQueue)
			}
		}
	}()

	// TODO: or it exists with a segfault or it hangs, how to fix?
	<-signalCh
	fmt.Println("Signal received, shutting down...")
	close(stopPackets)
	afHandle.Close()
	close(stopWorkers)
	wg.Wait()

	return nil
}

func processPacket(packet *gopacket.Packet, connections map[string]bool, connLock *sync.RWMutex, eventQueue chan ConnectionEvent) {
	// Get IP layer
	ipLayer := (*packet).Layer(layers.LayerTypeIPv4)
	if ipLayer == nil {
		return
	}
	ip, _ := ipLayer.(*layers.IPv4)

	// Get TCP layer
	tcpLayer := (*packet).Layer(layers.LayerTypeTCP)
	if tcpLayer == nil {
		return
	}
	tcp, _ := tcpLayer.(*layers.TCP)

	srcIP := ip.SrcIP.String()
	dstIP := ip.DstIP.String()
	srcPort := uint16(tcp.SrcPort)
	dstPort := uint16(tcp.DstPort)

	// Create connection identifier - only track in outbound direction
	connID := fmt.Sprintf("%s:%d->%s:%d", srcIP, srcPort, dstIP, dstPort)

	if tcp.SYN && !tcp.ACK {
		// Check if we've already seen this connection
		connLock.RLock()
		seen := connections[connID]
		connLock.RUnlock()

		if !seen {
			// New connection we haven't processed yet
			newConn := &SSHConnection{
				SourceIP:   srcIP,
				SourcePort: srcPort,
				DestIP:     dstIP,
				DestPort:   dstPort,
				StartTime:  time.Now(),
			}

			// Mark as seen
			connLock.Lock()
			connections[connID] = true
			connLock.Unlock()

			// Send to event queue
			eventQueue <- ConnectionEvent{
				ConnID: connID,
				Conn:   newConn,
			}
		}
	} else if !tcp.SYN && (tcp.ACK || tcp.PSH) {
		// Check if this is a connection we missed the SYN for
		connLock.RLock()
		seen := connections[connID]
		connLock.RUnlock()

		if !seen {
			// Connection we missed the SYN for
			newConn := &SSHConnection{
				SourceIP:   srcIP,
				SourcePort: srcPort,
				DestIP:     dstIP,
				DestPort:   dstPort,
				StartTime:  time.Now(),
			}

			// Mark as seen
			connLock.Lock()
			connections[connID] = true
			connLock.Unlock()

			// Send to event queue
			eventQueue <- ConnectionEvent{
				ConnID: connID,
				Conn:   newConn,
			}
		}
	}
}

func handleEvent(event ConnectionEvent) {
	fmt.Print("New SSH connection detected towards: ", event.Conn.DestIP, ":", event.Conn.DestPort, "\n")
	// Increment connections metric
	SSHConnections.WithLabelValues(event.Conn.DestIP, string(event.Conn.DestPort)).Inc()
}
