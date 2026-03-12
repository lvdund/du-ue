package config

import (
	"fmt"
	"os"
	"time"

	"github.com/reogac/nas"
	"gopkg.in/yaml.v3"
)

type Config struct {
	DU      DUConfig      `yaml:"du"`
	DUs     []DUConfig    `yaml:"dus"`     // Multiple DUs for handover simulation
	UE      UEConfig      `yaml:"ue"`
	Logging LoggingConfig `yaml:"logging"`
}

type LoggingConfig struct {
	Level string `yaml:"level"`
}

type DUConfig struct {
	ID        int64      `yaml:"id"`
	Name      string     `yaml:"name"`
	CUCPAddr  string     `yaml:"cucp_address"`
	CUCPPort  int        `yaml:"cucp_port"`
	LocalAddr string     `yaml:"local_address"`
	LocalPort int        `yaml:"local_port"`
	PLMN      PLMNConfig `yaml:"plmn"`
	Cell      CellConfig `yaml:"cell"`
}

type PLMNConfig struct {
	MCC string `yaml:"mcc"`
	MNC string `yaml:"mnc"`
}

type CellConfig struct {
	PCI     uint16 `yaml:"pci"`
	TAC     string `yaml:"tac"`
	NRARFCN uint32 `yaml:"nrarfcn"`
	Band    int64  `yaml:"band"`
}

type UEConfig struct {
	NUE        int          `yaml:"nue"`
	MSIN       string       `yaml:"msin"`        // Base MSIN, will increment for multiple UEs
	Key        string       `yaml:"key"`         // K in hex
	OP         string       `yaml:"op"`          // OP in hex (optional)
	OPC        string       `yaml:"opc"`         // OPC in hex (optional)
	AMF        string       `yaml:"amf"`         // AMF in hex
	PLMN       PLMNConfig   `yaml:"plmn"`
	Scenarios  []UEScenario `yaml:"scenarios"`   // List of scenarios to execute
	DefaultDnn string       `yaml:"default_dnn"`
	DUID       string       `yaml:"duid"`        // ID of the initial DU to connect to (e.g. "du-0")
}

// UEScenario defines a sequence of events for UE(s)
type UEScenario struct {
	Name        string       `yaml:"name"`        // Scenario name for logging
	Description string       `yaml:"description"` // Optional description
	ApplyTo     string       `yaml:"apply_to"`    // "all", "first", "last", or MSIN pattern
	Events      []EventEntry `yaml:"events"`      // Sequence of events
}

// EventEntry defines a single event with timing and parameters
type EventEntry struct {
	Type   string                 `yaml:"type"`   // Event type: rrc_setup, registration, pdu_establishment, etc.
	Delay  string                 `yaml:"delay"`  // Delay before triggering (e.g., "1s", "500ms", "2m")
	Params map[string]interface{} `yaml:"params"` // Optional parameters for the event
}

// GetUESecurityCapability returns UE security capability with all algorithms enabled
func (ue *UEConfig) GetUESecurityCapability() *nas.UeSecurityCapability {
	secCap := new(nas.UeSecurityCapability)

	// Enable all ciphering algorithms (NEA0, NEA1, NEA2, NEA3)
	secCap.SetEA(0, true) // NEA0
	secCap.SetEA(1, true) // NEA1
	secCap.SetEA(2, true) // NEA2
	secCap.SetEA(3, true) // NEA3

	// Enable all integrity algorithms (NIA0, NIA1, NIA2, NIA3)
	secCap.SetIA(0, true) // NIA0
	secCap.SetIA(1, true) // NIA1
	secCap.SetIA(2, true) // NIA2
	secCap.SetIA(3, true) // NIA3

	return secCap
}

// ParseDelay converts string delay to time.Duration
func (e *EventEntry) ParseDelay() (time.Duration, error) {
	if e.Delay == "" {
		return 0, nil
	}
	return time.ParseDuration(e.Delay)
}

// ShouldApplyToUE checks if scenario should apply to given UE
func (s *UEScenario) ShouldApplyToUE(msin string, ueIndex int, totalUEs int) bool {
	switch s.ApplyTo {
	case "", "all":
		return true
	case "first":
		return ueIndex == 0
	case "last":
		return ueIndex == totalUEs-1
	default:
		// Check if it matches MSIN pattern or specific MSIN
		return msin == s.ApplyTo
	}
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config file: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	// Try to automatically load the crypto keys from etrib5gc generated yaml
	if generatedUE, err := os.ReadFile("third_party/etrib5gc/util/ue-gen/ue_1.yaml"); err == nil {
		var uegen struct {
			Key    string `yaml:"key"`
			Op     string `yaml:"op"`
			OpType string `yaml:"opType"`
			Amf    string `yaml:"amf"`
		}
		if err := yaml.Unmarshal(generatedUE, &uegen); err == nil && uegen.Key != "" {
			cfg.UE.Key = uegen.Key
			cfg.UE.AMF = uegen.Amf
			if uegen.OpType == "OPC" || uegen.OpType == "opc" {
				cfg.UE.OPC = uegen.Op
				cfg.UE.OP = ""
			} else {
				cfg.UE.OP = uegen.Op
				cfg.UE.OPC = ""
			}
		}
	}

	// If DUs list is empty but single DU is configured, use it as default
	if len(cfg.DUs) == 0 && cfg.DU.Name != "" {
		cfg.DUs = []DUConfig{cfg.DU}
	}

	// Default DUID to first DU if not specified
	if cfg.UE.DUID == "" && len(cfg.DUs) > 0 {
		cfg.UE.DUID = fmt.Sprintf("du-%d", cfg.DUs[0].ID)
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("validate config: %w", err)
	}

	return &cfg, nil
}

func (c *Config) Validate() error {
	if c.DU.Name == "" {
		return fmt.Errorf("du.name is required")
	}
	if c.DU.CUCPAddr == "" {
		return fmt.Errorf("du.cucp_address is required")
	}
	if c.DU.CUCPPort == 0 {
		return fmt.Errorf("du.cucp_port is required")
	}
	if c.DU.PLMN.MCC == "" {
		return fmt.Errorf("du.plmn.mcc is required")
	}
	if c.DU.PLMN.MNC == "" {
		return fmt.Errorf("du.plmn.mnc is required")
	}
	if c.UE.MSIN == "" {
		return fmt.Errorf("ue.msin is required")
	}
	if c.UE.Key == "" {
		return fmt.Errorf("ue.key is required")
	}
	if c.UE.AMF == "" {
		return fmt.Errorf("ue.amf is required")
	}

	// Validate scenarios and events
	for i, scenario := range c.UE.Scenarios {
		if scenario.Name == "" {
			return fmt.Errorf("scenario[%d].name is required", i)
		}
		for j, event := range scenario.Events {
			if event.Type == "" {
				return fmt.Errorf("scenario[%d].events[%d].type is required", i, j)
			}
			// Validate delay format
			if event.Delay != "" {
				if _, err := time.ParseDuration(event.Delay); err != nil {
					return fmt.Errorf("scenario[%d].events[%d].delay invalid: %w", i, j, err)
				}
			}
		}
	}

	return nil
}