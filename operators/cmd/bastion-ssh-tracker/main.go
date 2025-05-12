package main

import (
	"log"

	tracker "github.com/netgroup-polito/CrownLabs/operators/pkg/bastion-ssh-tracker"
)

func main() {
	iface := "any"
	port := 22
	snaplen := 1600

	err := tracker.Tracker(iface, port, snaplen)
	if err != nil {
		log.Fatalf("Error running tracker: %v", err)
	}
}
