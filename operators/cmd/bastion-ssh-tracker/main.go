package main

import (
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	tracker "github.com/netgroup-polito/CrownLabs/operators/pkg/bastion-ssh-tracker"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func main() {
	iface := "any"
	port := 22
	snaplen := 1600

	// Ensure to correctly handle tracker graceful shutdown
	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		http.Handle("/metrics", promhttp.Handler())
		http.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})
		log.Println("Starting metrics server on :8080")
		if err := http.ListenAndServe(":8080", nil); err != nil && err != http.ErrServerClosed {
			log.Printf("Metrics server error: %v", err)
		}
	}()

	log.Printf("Starting SSH tracker on interface %s, port %d, snaplen %d", iface, port, snaplen)
	sshTracker := tracker.NewSSHTracker()
	go func() {
		if err := sshTracker.Start(iface, port, snaplen); err != nil {
			log.Printf("Tracker stopped with error: %v", err)
		}
	}()

	<-signalCh
	log.Println("Signal received, shutting down...")
	sshTracker.Stop()
	log.Println("Shutdown complete")
}
