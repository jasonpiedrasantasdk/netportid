package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/jasonpiedrasantasdk/netportid/capture"
	"github.com/jasonpiedrasantasdk/netportid/tracker"
	"github.com/jasonpiedrasantasdk/netportid/tui"

	tea "github.com/charmbracelet/bubbletea"
)

func main() {
	ifaceFlag := flag.String("i", "", "Interfaces to monitor (comma-separated, defaults to all active)")
	flag.Parse()

	log.SetPrefix("[netportid] ")
	log.SetFlags(log.Ltime | log.Lshortfile)

	if os.Geteuid() != 0 {
		fmt.Fprintln(os.Stderr, "netPortID needs root for packet capture.")
		fmt.Fprintln(os.Stderr, "Run: sudo", os.Args[0])
		os.Exit(1)
	}

	var ifaces []string
	var err error
	if *ifaceFlag != "" {
		ifaces = strings.Split(*ifaceFlag, ",")
		for i := range ifaces {
			ifaces[i] = strings.TrimSpace(ifaces[i])
		}
	} else {
		ifaces, err = capture.GetActiveIfaces()
		if err != nil {
			log.Fatalf("Error getting interfaces: %v", err)
		}
	}
	if len(ifaces) == 0 {
		log.Fatalf("No active network interfaces found")
	}
	log.Printf("Active interfaces: %v", ifaces)

	engine, err := capture.NewEngine(ifaces)
	if err != nil {
		log.Fatalf("Error creating capture engine: %v", err)
	}

	store := tracker.NewStore(2000)
	procMap := tracker.NewProcMapper()

	if err := engine.Start(); err != nil {
		log.Fatalf("Error starting capture: %v", err)
	}
	defer engine.Stop()

	go processEvents(engine, store, procMap)

	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			store.ExpireOld(5 * time.Minute)
		}
	}()

	m := tui.New(store, procMap, ifaces)
	p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseCellMotion())

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		p.Quit()
	}()

	if _, err := p.Run(); err != nil {
		log.Fatalf("Error running TUI: %v", err)
	}
}

func processEvents(engine *capture.Engine, store *tracker.PortStore, procMap *tracker.ProcMapper) {
	procMap.Update()

	throttle := time.NewTicker(100 * time.Microsecond)
	defer throttle.Stop()

	for ev := range engine.Events() {
		<-throttle.C

		flowEv := tracker.FlowEvent{
			SrcIP:     ev.SrcIP,
			DstIP:     ev.DstIP,
			SrcPort:   ev.SrcPort,
			DstPort:   ev.DstPort,
			Protocol:  ev.Protocol,
			Payload:   ev.Payload,
			Timestamp: ev.Timestamp,
			PacketLen: ev.PacketLen,
			Interface: ev.Interface,
			Direction: tracker.DirectionType(ev.Direction),
			IsDNS:     ev.IsDNS,
			IsIPv6:    ev.IsIPv6,
		}

		store.HandleEvent(flowEv)
	}
}
