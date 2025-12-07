package mmdb

import (
	"fmt"
	"net"
	"net/netip"
	"os"
	"path/filepath"
	"time"

	"github.com/maxmind/mmdbwriter"
	"github.com/maxmind/mmdbwriter/inserter"
	"github.com/maxmind/mmdbwriter/mmdbtype"

	"github.com/lfrfrfr/beon-ipquality/pkg/logger"
	"github.com/lfrfrfr/beon-ipquality/pkg/models"
)

// WriterConfig holds configuration for MMDB writing
type WriterConfig struct {
	DatabaseType        string
	Description         string
	RecordSize          int // 24, 28, or 32
	IPVersion           int // 4, 6, or 0 for both
	IncludeReservedNets bool
	DisableIPv4Aliasing bool
}

// DefaultWriterConfig returns the default writer configuration
func DefaultWriterConfig() WriterConfig {
	return WriterConfig{
		DatabaseType:        "BEON-IPReputation",
		Description:         "BEON IP Reputation Database",
		RecordSize:          28,
		IPVersion:           0, // Both IPv4 and IPv6
		IncludeReservedNets: false,
		DisableIPv4Aliasing: false,
	}
}

// Writer handles writing MMDB files
type Writer struct {
	config WriterConfig
}

// NewWriter creates a new MMDB writer
func NewWriter(config WriterConfig) *Writer {
	return &Writer{config: config}
}

// NewDefaultWriter creates a writer with default configuration
func NewDefaultWriter() *Writer {
	return NewWriter(DefaultWriterConfig())
}

// ReputationEntry represents a single entry to write
type ReputationEntry struct {
	Prefix     netip.Prefix
	RiskScore  int
	RiskLevel  string
	ThreatType string
	Confidence float64
	Sources    []string
	Flags      EntryFlags
	LastUpdate time.Time
}

// EntryFlags represents boolean threat flags
type EntryFlags struct {
	IsTor        bool
	IsVPN        bool
	IsProxy      bool
	IsDatacenter bool
	IsBotnet     bool
	IsMalware    bool
	IsSpam       bool
	IsAttacker   bool
}

// CompileToMMDB compiles reputation entries to an MMDB file
func (w *Writer) CompileToMMDB(entries []ReputationEntry, outputPath string) error {
	logger.Info(fmt.Sprintf("Starting MMDB compilation with %d entries", len(entries)))
	startTime := time.Now()

	// Create output directory if it doesn't exist
	dir := filepath.Dir(outputPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	// Create MMDB writer
	tree, err := mmdbwriter.New(mmdbwriter.Options{
		DatabaseType:            w.config.DatabaseType,
		Description:             map[string]string{"en": w.config.Description},
		RecordSize:              w.config.RecordSize,
		IPVersion:               w.config.IPVersion,
		IncludeReservedNetworks: w.config.IncludeReservedNets,
		DisableIPv4Aliasing:     w.config.DisableIPv4Aliasing,
		Inserter:                inserter.ReplaceWith,
	})
	if err != nil {
		return fmt.Errorf("failed to create MMDB writer: %w", err)
	}

	// Insert entries
	var insertedCount int
	var errorCount int

	for _, entry := range entries {
		record := w.entryToMMDBRecord(entry)

		// Convert netip.Prefix to net.IPNet
		ipNet := prefixToIPNet(entry.Prefix)

		err := tree.Insert(ipNet, record)
		if err != nil {
			logger.Debug(fmt.Sprintf("Failed to insert %s: %v", entry.Prefix, err))
			errorCount++
			continue
		}
		insertedCount++
	}

	// Write to file
	tempPath := outputPath + ".tmp"
	file, err := os.Create(tempPath)
	if err != nil {
		return fmt.Errorf("failed to create output file: %w", err)
	}

	_, err = tree.WriteTo(file)
	file.Close()
	if err != nil {
		os.Remove(tempPath)
		return fmt.Errorf("failed to write MMDB: %w", err)
	}

	// Atomic rename
	if err := os.Rename(tempPath, outputPath); err != nil {
		os.Remove(tempPath)
		return fmt.Errorf("failed to rename output file: %w", err)
	}

	logger.Info(fmt.Sprintf("MMDB compilation complete: %d entries inserted, %d errors, took %v",
		insertedCount, errorCount, time.Since(startTime)))

	return nil
}

// entryToMMDBRecord converts a ReputationEntry to MMDB record format
func (w *Writer) entryToMMDBRecord(entry ReputationEntry) mmdbtype.DataType {
	// Build sources array
	sources := mmdbtype.Slice{}
	for _, s := range entry.Sources {
		sources = append(sources, mmdbtype.String(s))
	}

	record := mmdbtype.Map{
		"risk_score":    mmdbtype.Uint16(entry.RiskScore),
		"risk_level":    mmdbtype.String(entry.RiskLevel),
		"threat_type":   mmdbtype.String(entry.ThreatType),
		"confidence":    mmdbtype.Uint16(int(entry.Confidence * 100)),
		"sources":       sources,
		"last_update":   mmdbtype.Uint64(entry.LastUpdate.Unix()),
		"is_tor":        mmdbtype.Bool(entry.Flags.IsTor),
		"is_vpn":        mmdbtype.Bool(entry.Flags.IsVPN),
		"is_proxy":      mmdbtype.Bool(entry.Flags.IsProxy),
		"is_datacenter": mmdbtype.Bool(entry.Flags.IsDatacenter),
		"is_botnet":     mmdbtype.Bool(entry.Flags.IsBotnet),
		"is_malware":    mmdbtype.Bool(entry.Flags.IsMalware),
		"is_spam":       mmdbtype.Bool(entry.Flags.IsSpam),
		"is_attacker":   mmdbtype.Bool(entry.Flags.IsAttacker),
	}

	return record
}

// prefixToIPNet converts netip.Prefix to *net.IPNet
func prefixToIPNet(prefix netip.Prefix) *net.IPNet {
	addr := prefix.Addr()
	bits := prefix.Bits()

	ip := net.IP(addr.AsSlice())

	var mask net.IPMask
	if addr.Is4() {
		mask = net.CIDRMask(bits, 32)
	} else {
		mask = net.CIDRMask(bits, 128)
	}

	return &net.IPNet{
		IP:   ip,
		Mask: mask,
	}
}

// CompileFromIPReputations compiles from models.IPReputation slice
func (w *Writer) CompileFromIPReputations(reputations []models.IPReputation, outputPath string) error {
	entries := make([]ReputationEntry, 0, len(reputations))

	for _, rep := range reputations {
		prefix, err := netip.ParsePrefix(rep.IPRange)
		if err != nil {
			// Try as single IP
			addr, err := netip.ParseAddr(rep.IPRange)
			if err != nil {
				logger.Debug(fmt.Sprintf("Invalid IP range: %s", rep.IPRange))
				continue
			}
			// Convert single IP to /32 or /128
			if addr.Is4() {
				prefix = netip.PrefixFrom(addr, 32)
			} else {
				prefix = netip.PrefixFrom(addr, 128)
			}
		}

		entry := ReputationEntry{
			Prefix:     prefix,
			RiskScore:  rep.RiskScore,
			RiskLevel:  classifyRisk(rep.RiskScore),
			ThreatType: rep.ThreatType,
			Confidence: rep.Confidence,
			Sources:    []string{rep.Source},
			Flags:      threatTypeToFlags(rep.ThreatType),
			LastUpdate: rep.LastSeen,
		}

		entries = append(entries, entry)
	}

	return w.CompileToMMDB(entries, outputPath)
}

// classifyRisk returns risk level based on score
func classifyRisk(score int) string {
	switch {
	case score >= 85:
		return "critical"
	case score >= 70:
		return "high"
	case score >= 50:
		return "medium"
	case score >= 25:
		return "low"
	default:
		return "clean"
	}
}

// threatTypeToFlags converts threat type to flags
func threatTypeToFlags(threatType string) EntryFlags {
	flags := EntryFlags{}

	switch threatType {
	case "tor":
		flags.IsTor = true
	case "vpn":
		flags.IsVPN = true
	case "proxy":
		flags.IsProxy = true
	case "datacenter":
		flags.IsDatacenter = true
	case "botnet_c2":
		flags.IsBotnet = true
	case "malware":
		flags.IsMalware = true
	case "spam":
		flags.IsSpam = true
	case "attack":
		flags.IsAttacker = true
	}

	return flags
}

// MergeAndCompile merges multiple reputation sources and compiles to MMDB
func (w *Writer) MergeAndCompile(sources map[string][]models.IPReputation, outputPath string) error {
	// Merge entries by IP, keeping highest risk scores
	merged := make(map[string]ReputationEntry)

	for sourceName, reputations := range sources {
		for _, rep := range reputations {
			key := rep.IPRange

			existing, exists := merged[key]
			if !exists {
				prefix, err := parseToPrefix(rep.IPRange)
				if err != nil {
					continue
				}

				merged[key] = ReputationEntry{
					Prefix:     prefix,
					RiskScore:  rep.RiskScore,
					RiskLevel:  classifyRisk(rep.RiskScore),
					ThreatType: rep.ThreatType,
					Confidence: rep.Confidence,
					Sources:    []string{sourceName},
					Flags:      threatTypeToFlags(rep.ThreatType),
					LastUpdate: rep.LastSeen,
				}
			} else {
				// Merge: keep higher score, combine sources
				if rep.RiskScore > existing.RiskScore {
					existing.RiskScore = rep.RiskScore
					existing.RiskLevel = classifyRisk(rep.RiskScore)
				}
				if rep.Confidence > existing.Confidence {
					existing.Confidence = rep.Confidence
				}
				// Add source if not already present
				sourceExists := false
				for _, s := range existing.Sources {
					if s == sourceName {
						sourceExists = true
						break
					}
				}
				if !sourceExists {
					existing.Sources = append(existing.Sources, sourceName)
				}
				// Merge flags
				newFlags := threatTypeToFlags(rep.ThreatType)
				existing.Flags.IsTor = existing.Flags.IsTor || newFlags.IsTor
				existing.Flags.IsVPN = existing.Flags.IsVPN || newFlags.IsVPN
				existing.Flags.IsProxy = existing.Flags.IsProxy || newFlags.IsProxy
				existing.Flags.IsDatacenter = existing.Flags.IsDatacenter || newFlags.IsDatacenter
				existing.Flags.IsBotnet = existing.Flags.IsBotnet || newFlags.IsBotnet
				existing.Flags.IsMalware = existing.Flags.IsMalware || newFlags.IsMalware
				existing.Flags.IsSpam = existing.Flags.IsSpam || newFlags.IsSpam
				existing.Flags.IsAttacker = existing.Flags.IsAttacker || newFlags.IsAttacker

				if rep.LastSeen.After(existing.LastUpdate) {
					existing.LastUpdate = rep.LastSeen
				}

				merged[key] = existing
			}
		}
	}

	// Convert map to slice
	entries := make([]ReputationEntry, 0, len(merged))
	for _, entry := range merged {
		entries = append(entries, entry)
	}

	logger.Info(fmt.Sprintf("Merged %d unique IP ranges from %d sources", len(entries), len(sources)))

	return w.CompileToMMDB(entries, outputPath)
}

// parseToPrefix parses a string to netip.Prefix
func parseToPrefix(s string) (netip.Prefix, error) {
	prefix, err := netip.ParsePrefix(s)
	if err != nil {
		addr, err := netip.ParseAddr(s)
		if err != nil {
			return netip.Prefix{}, err
		}
		if addr.Is4() {
			return netip.PrefixFrom(addr, 32), nil
		}
		return netip.PrefixFrom(addr, 128), nil
	}
	return prefix, nil
}
