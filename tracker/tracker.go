package tracker

import (
	"fmt"
	"net"
	"sync"
	"time"
)

type FlowKey struct {
	SrcIP    string
	DstIP    string
	SrcPort  uint16
	DstPort  uint16
	Protocol uint8
}

type FlowEvent struct {
	SrcIP     net.IP
	DstIP     net.IP
	SrcPort   uint16
	DstPort   uint16
	Protocol  uint8
	Payload   []byte
	Timestamp time.Time
	PacketLen int
	Interface string
	Direction DirectionType
	IsDNS     bool
	IsIPv6    bool
}

type DirectionType int

const (
	DirectionOutbound DirectionType = iota
	DirectionInbound
)

type PortKey struct {
	Port     uint16
	Protocol uint8
}

func (k PortKey) String() string {
	return fmt.Sprintf("%d/%d", k.Port, k.Protocol)
}

type PortEntry struct {
	Key PortKey

	LocalPort  uint16
	Protocol   uint8
	ProtoInfo  ProtocolInfo
	Interface  string

	ProcessName string
	ProcessCmd  string
	PID         int

	BytesSent   uint64
	BytesRecv   uint64
	PacketsSent uint64
	PacketsRecv uint64

	TickBytesSent uint64
	TickBytesRecv uint64

	FirstSeen time.Time
	LastSeen  time.Time

	IsListening bool
	IsActive    bool
}

type PortStore struct {
	mu       sync.RWMutex
	ports    map[PortKey]*PortEntry
	order    []PortKey
	maxPorts int
}

func NewStore(maxPorts int) *PortStore {
	return &PortStore{
		ports:    make(map[PortKey]*PortEntry),
		order:    make([]PortKey, 0),
		maxPorts: maxPorts,
	}
}

func canonicalLocalPort(ev FlowEvent) (localPort uint16, protocol uint8) {
	isInbound := ev.Direction == DirectionInbound
	if !isInbound {
		return ev.SrcPort, ev.Protocol
	}
	return ev.DstPort, ev.Protocol
}

func (s *PortStore) HandleEvent(ev FlowEvent) {
	s.mu.Lock()
	defer s.mu.Unlock()

	localPort, protocol := canonicalLocalPort(ev)
	key := PortKey{Port: localPort, Protocol: protocol}

	entry, ok := s.ports[key]
	if !ok {
		protoInfo := DetectProtocol(ev.Payload, ev.SrcPort, ev.DstPort, ev.Protocol)

		entry = &PortEntry{
			Key:       key,
			LocalPort: localPort,
			Protocol:  protocol,
			ProtoInfo: protoInfo,
			Interface: ev.Interface,
			FirstSeen: ev.Timestamp,
			LastSeen:  ev.Timestamp,
		}
		s.ports[key] = entry
		s.order = append(s.order, key)
	}

	entry.LastSeen = ev.Timestamp
	entry.Interface = ev.Interface
	entry.IsActive = true

	if ev.Direction == DirectionOutbound {
		entry.BytesSent += uint64(ev.PacketLen)
		entry.PacketsSent++
		entry.TickBytesSent += uint64(ev.PacketLen)
	} else {
		entry.BytesRecv += uint64(ev.PacketLen)
		entry.PacketsRecv++
		entry.TickBytesRecv += uint64(ev.PacketLen)
	}

	if len(ev.Payload) > 0 && entry.ProtoInfo.SNI == "" {
		info := DetectProtocol(ev.Payload, ev.SrcPort, ev.DstPort, ev.Protocol)
		if info.SNI != "" || info.DNSQuery != "" {
			entry.ProtoInfo = info
		}
	}

	if len(s.ports) > s.maxPorts {
		s.trimOld()
	}
}

func (s *PortStore) SeedFromProcMap(procMap map[uint16]map[uint8]ProcEntry) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for port, protoMap := range procMap {
		for proto, pe := range protoMap {
			key := PortKey{Port: port, Protocol: proto}
			if _, exists := s.ports[key]; !exists {
				s.ports[key] = &PortEntry{
					Key:         key,
					LocalPort:   port,
					Protocol:    proto,
					ProcessName: pe.Name,
					ProcessCmd:  pe.CmdLine,
					PID:         pe.PID,
					IsListening: true,
				}
				s.order = append(s.order, key)
			} else {
				entry := s.ports[key]
				entry.ProcessName = pe.Name
				entry.ProcessCmd = pe.CmdLine
				entry.PID = pe.PID
				entry.IsListening = true
			}
		}
	}
}

func (s *PortStore) SearchPort(port uint16, procMap map[uint16]map[uint8]ProcEntry) *PortEntry {
	s.mu.RLock()
	key6 := PortKey{Port: port, Protocol: 6}
	key17 := PortKey{Port: port, Protocol: 17}

	best := func(key PortKey) *PortEntry {
		if e, ok := s.ports[key]; ok {
			cp := *e
			return &cp
		}
		return nil
	}

	entry := best(key6)
	if entry == nil {
		entry = best(key17)
	}

	if entry != nil {
		if pm, ok := procMap[port]; ok {
			for _, pe := range pm {
				if entry.ProcessName == "" {
					entry.ProcessName = pe.Name
					entry.ProcessCmd = pe.CmdLine
					entry.PID = pe.PID
				}
				if !entry.IsListening {
					entry.IsListening = true
				}
			}
		}
		s.mu.RUnlock()
		return entry
	}

	if pm, ok := procMap[port]; ok {
		for proto, pe := range pm {
			s.mu.RUnlock()
			s.mu.Lock()
			key := PortKey{Port: port, Protocol: proto}
			e := &PortEntry{
				Key:         key,
				LocalPort:   port,
				Protocol:    proto,
				ProcessName: pe.Name,
				ProcessCmd:  pe.CmdLine,
				PID:         pe.PID,
				IsListening: true,
			}
			s.ports[key] = e
			s.order = append(s.order, key)
			cp := *e
			s.mu.Unlock()
			return &cp
		}
	}

	s.mu.RUnlock()
	return nil
}

func (s *PortStore) ResetTickDeltas() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, entry := range s.ports {
		entry.TickBytesSent = 0
		entry.TickBytesRecv = 0
	}
}

func (s *PortStore) GetPorts() []*PortEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]*PortEntry, 0, len(s.order))
	for _, key := range s.order {
		if entry, ok := s.ports[key]; ok {
			cp := *entry
			result = append(result, &cp)
		}
	}
	return result
}

func (s *PortStore) GetActivePorts() []*PortEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]*PortEntry, 0, len(s.order))
	cutoff := time.Now().Add(-30 * time.Second)
	for _, key := range s.order {
		if entry, ok := s.ports[key]; ok {
			if entry.LastSeen.After(cutoff) && entry.BytesSent+entry.BytesRecv > 0 {
				cp := *entry
				result = append(result, &cp)
			}
		}
	}
	return result
}

func (s *PortStore) SetProcess(key PortKey, pid int, name, cmd string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if entry, ok := s.ports[key]; ok {
		entry.PID = pid
		entry.ProcessName = name
		entry.ProcessCmd = cmd
	}
}

func (s *PortStore) SetProcessByPort(localPort uint16, protocol uint8, pid int, name, cmd string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := PortKey{Port: localPort, Protocol: protocol}
	if entry, ok := s.ports[key]; ok {
		entry.PID = pid
		entry.ProcessName = name
		entry.ProcessCmd = cmd
	}
}

func (s *PortStore) Count() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.ports)
}

func (s *PortStore) trimOld() {
	for len(s.ports) > s.maxPorts {
		var oldestKey PortKey
		var oldestTime time.Time
		for _, key := range s.order {
			if entry, ok := s.ports[key]; ok {
				if oldestKey == (PortKey{}) || entry.LastSeen.Before(oldestTime) {
					oldestKey = key
					oldestTime = entry.LastSeen
				}
			}
		}
		if oldestKey == (PortKey{}) {
			break
		}
		delete(s.ports, oldestKey)
		s.removeOrder(oldestKey)
	}
}

func (s *PortStore) removeOrder(key PortKey) {
	for i, k := range s.order {
		if k == key {
			s.order = append(s.order[:i], s.order[i+1:]...)
			return
		}
	}
}

func (s *PortStore) ExpireOld(maxAge time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()

	cutoff := time.Now().Add(-maxAge)
	for _, entry := range s.ports {
		if entry.LastSeen.Before(cutoff) {
			delete(s.ports, entry.Key)
			s.removeOrder(entry.Key)
		}
	}
}
