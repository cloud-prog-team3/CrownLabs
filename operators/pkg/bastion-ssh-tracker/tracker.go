package bastionsshtracker

import (
	"fmt"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
)

type SSHTracker struct {
	stopCh chan struct{}
	done   chan struct{}
}

type SSHConnection struct {
	SourceIP   string
	SourcePort uint16
	DestIP     string
	DestPort   uint16
	StartTime  time.Time
}

type ConnectionEvent struct {
	Conn *SSHConnection
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

	newConn := &SSHConnection{
		SourceIP:   srcIP,
		SourcePort: srcPort,
		DestIP:     dstIP,
		DestPort:   dstPort,
		StartTime:  time.Now(),
	}

	// Send to event queue
	eventQueue <- ConnectionEvent{
		Conn: newConn,
	}
}

// Increment connections metric
func handleEvent(event ConnectionEvent) {
	fmt.Print("New SSH connection detected towards: ", event.Conn.DestIP, ":", event.Conn.DestPort, "\n")
	sshConnections.WithLabelValues(event.Conn.DestIP, strconv.Itoa(int(event.Conn.DestPort))).Inc()
}

func NewSSHTracker() *SSHTracker {
	return &SSHTracker{
		stopCh: make(chan struct{}),
		done:   make(chan struct{}),
	}
}

func (t *SSHTracker) Start(iface string, port int, snaplen int) error {
	defer close(t.done)

	szFrame, szBlock, numBlocks, err := afpacketComputeSize(8, snaplen, os.Getpagesize())
	if err != nil {
		return fmt.Errorf("error computing afpacket size: %v", err)
	}

	timeout := time.Millisecond * 100

	afHandle, err := newAfpacketHandle(iface, szFrame, szBlock, numBlocks, false, timeout)
	if err != nil {
		return fmt.Errorf("error creating afpacket handle: %v", err)
	}
	defer afHandle.Close()

	// Filter for new outbound TCP packets on the specified port
	// Explicitly check for SYN packets to identify new connections
	// We want to track when a connection is established
	filter := fmt.Sprintf("tcp dst port %d and tcp[tcpflags] & tcp-syn != 0", port)
	if err := afHandle.SetBPFFilter(filter, snaplen); err != nil {
		return fmt.Errorf("error setting BPF filter: %v", err)
	}

	source := gopacket.ZeroCopyPacketDataSource(afHandle)

	eventQueue := make(chan ConnectionEvent, 100)

	connections := make(map[string]bool)
	var connLock sync.RWMutex

	var wg sync.WaitGroup
	stopWorkers := make(chan struct{})
	stopPackets := make(chan struct{})

	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case event := <-eventQueue:
				handleEvent(event)
			case <-stopWorkers:
				return
			}
		}
	}()

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

	<-t.stopCh
	close(stopPackets)
	// Let the packet processing finish
	time.Sleep(2 * timeout)
	afHandle.Close()
	close(stopWorkers)
	wg.Wait()

	return nil
}

func (t *SSHTracker) Stop() {
	close(t.stopCh)
	<-t.done
}
