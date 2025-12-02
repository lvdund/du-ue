package config

import (
	"fmt"
	"os"

	"github.com/reogac/nas"
	"gopkg.in/yaml.v3"
)

type Config struct {
	DU DUConfig `yaml:"du"`
	UE UEConfig `yaml:"ue"`
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
	PCI uint16 `yaml:"pci"`
	TAC string `yaml:"tac"`
}

type UEConfig struct {
	MSIN string     `yaml:"msin"`
	SUPI string     `yaml:"supi"`
	Key  string     `yaml:"key"` // K in hex
	OP   string     `yaml:"op"`  // OP in hex (optional)
	OPC  string     `yaml:"opc"` // OPC in hex (optional)
	AMF  string     `yaml:"amf"` // AMF in hex
	PLMN PLMNConfig `yaml:"plmn"`
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

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config file: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
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
	return nil
}
