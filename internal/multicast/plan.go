package multicast

import "time"

type Plan struct {
	Timestamp      time.Time `json:"timestamp"`
	Mode           string    `json:"mode"`
	Applied        bool      `json:"applied"`
	Target         Target    `json:"target"`
	InterfaceIndex int       `json:"interface_index"`
	DatagramBytes  int       `json:"datagram_bytes"`
	Prechecks      []string  `json:"prechecks"`
	Rollback       []string  `json:"rollback"`
	Validation     []string  `json:"validation"`
}

func DryRun(t Target, interfaces []Interface) (Plan, error) {
	index, e := resolve(t, interfaces)
	if e != nil {
		return Plan{}, e
	}
	return Plan{Timestamp: time.Now().UTC(), Mode: "dry_run", Target: t, InterfaceIndex: index, DatagramBytes: DatagramSize,
		Prechecks:  []string{"confirm isolated laboratory host, VLAN and operator-approved interface/IP/group/port", "confirm source IP and no address/group conflicts", "record routes/firewall/interface baseline without modifying them", "confirm switch IGMP snooping, VLAN membership and upstream multicast boundaries", "obtain approval before mode=send; dry-run does not test kernel permissions or L2 isolation"},
		Rollback:   []string{"DELETE /api/v1/streams/<channel_id>; confirm terminal status and closed socket", "restore dry_run configuration and previous local agent if approved", "compare baseline; module never changes routes/firewall/NIC"},
		Validation: []string{"capture approved interface: verify source IP, group/port, TTL=1 and 1316-byte payload", "capture all non-lab interfaces and upstream link: require zero matching datagrams", "stop output and confirm traffic ceases; socket writes do not prove receiver delivery"}}, nil
}
