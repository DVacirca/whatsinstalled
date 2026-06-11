package scanner

// AllScanners is the complete registry of all known scanners.
var AllScanners = []Scanner{
	AptScanner{},
	SnapScanner{},
	NpmScanner{},
	PipScanner{},
	CondaScanner{},
	BinScanner{},
	PixiScanner{},
	GoScanner{},
	DockerScanner{},
	BrewScanner{},
	CargoScanner{},
	PacmanScanner{},
	YayScanner{},
	FlatpakScanner{},
	NixScanner{},
}

// DiscoverScanners returns only the scanners that have actual packages present.
func DiscoverScanners() []Scanner {
	var active []Scanner
	for _, sc := range AllScanners {
		if sc.IsAvailable() && sc.Probe() {
			active = append(active, sc)
		}
	}
	return active
}
