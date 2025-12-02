<div align="center">

# 📡 du-ue

**A Golang implementation of Distributed Unit (DU) and User Equipment (UE) simulator for 5G Open RAN architecture**

[![Go Version](https://img.shields.io/badge/Go-1.24.4-blue.svg)](https://golang.org/)
[![License](https://img.shields.io/badge/License-MIT-green.svg)](LICENSE)
[![5G](https://img.shields.io/badge/5G-DU--UE-orange.svg)](https://www.3gpp.org/)

---

*A Golang implementation of a Distributed Unit (DU) and User Equipment (UE) simulator for 5G Open RAN architecture. This project simulates DU-UE interaction for validating CU-CP features, focusing on 5GMM registration procedure with F1AP (CU-CP interface) and RRC/NAS message handling.*

</div>

## Overview

The DU-UE Simulator implements a simplified 5G network stack that:

- **DU (Distributed Unit)**: Simulates the distributed unit that connects to CU-CP (Central Unit - Control Plane) via F1AP protocol over SCTP
- **UE (User Equipment)**: Simulates a user equipment that performs 5GMM registration procedure
- **RRC (Radio Resource Control)**: Handles RRC message exchange between UE and DU
- **NAS (Non-Access Stratum)**: Handles NAS messages (embedded in RRC) for 5GMM registration

### Key Features

- ✅ F1AP communication with CU-CP over SCTP (PPID=62)
- ✅ RRC message handling (RRCSetup, RRCSetupComplete, DLInformationTransfer, RRCReconfiguration)
- ✅ 5GMM Registration Procedure (Registration Request, Identity Request/Response, Authentication, Security Mode, Registration Accept)
- ✅ NAS message encoding/decoding embedded in RRC messages
- ✅ Simplified UE context focused on registration procedure only

## Requirements

### Operating System Support

This project requires **Linux** with SCTP (Stream Control Transmission Protocol) support. SCTP is typically available on:

- Ubuntu 18.04+
- Debian 10+
- CentOS/RHEL 7+
- Arch Linux

### Ubuntu Package Installation

For Ubuntu/Debian systems, install the required SCTP development libraries:

```bash
sudo apt-get update
sudo apt-get install -y libsctp-dev
```

### Go Requirements

- Go 1.18 or higher
- Go modules enabled

### Dependencies

The project uses the following key dependencies:

- `github.com/ishidawataru/sctp` - SCTP library for Go
- `github.com/JocelynWS/f1-gen` - F1AP protocol library
- `github.com/lvdund/rrc` - RRC protocol library
- `github.com/lvdund/ngap/aper` - ASN.1 APER encoding/decoding

## Configuration

The simulator is configured via `config/config.yml`. Here's a detailed description of each configuration section:

### DU Configuration

```yaml
du:
  id: 1                          # DU identifier (unique per DU)
  name: "DU-UE-Simulator"        # DU name
  cucp_address: "192.168.1.10"   # CU-CP IP address (F1AP server)
  cucp_port: 38472               # CU-CP SCTP port (F1AP port)
  local_address: "192.168.1.10"  # Local IP address for SCTP binding
  local_port: 38473              # Local SCTP port (optional, 0 for auto)
  plmn:
    mcc: "999"                   # Mobile Country Code (3 digits)
    mnc: "70"                    # Mobile Network Code (2-3 digits)
  cell:
    pci: 1                       # Physical Cell ID (0-1007)
    tac: "000001"                # Tracking Area Code (hex string, 3 bytes)
```

**Configuration Notes:**
- `cucp_address` and `cucp_port`: The address and port where the CU-CP F1AP server is listening
- `local_address` and `local_port`: Local binding address (can be empty/0 for auto-assignment)
- `plmn.mcc` and `plmn.mnc`: Must match the PLMN configuration in CU-CP
- `cell.pci`: Physical Cell Identifier (0-1007 range)
- `cell.tac`: Tracking Area Code as hex string (6 hex digits = 3 bytes)

### UE Configuration

```yaml
ue:
  msin: "0000000001"             # Mobile Station Identification Number (10 digits)
  supi: "208930000000001"        # Subscription Permanent Identifier (IMSI format)
  suci: "imsi-99970-0000000001" # Subscription Concealed Identifier (optional)
  key: "465B5CE8B199B49FAA5F0A2EE238A6BC"  # Authentication key K (32 hex chars = 16 bytes)
  opc: "E8ED289DEBA952E4283B54E88E6183CA"  # Operator Variant Algorithm Configuration Field (32 hex chars)
  amf: "8000"                    # Authentication Management Field (4 hex chars = 2 bytes)
  plmn:
    mcc: "999"                   # Mobile Country Code (must match DU PLMN)
    mnc: "70"                    # Mobile Network Code (must match DU PLMN)
```

**Configuration Notes:**
- `msin`: 10-digit MSIN (part of IMSI after MCC+MNC)
- `supi`: Full IMSI format (MCC+MNC+MSIN)
- `key`: 128-bit authentication key K in hexadecimal (32 characters)
- `opc`: 128-bit OPc value in hexadecimal (32 characters)
- `amf`: 16-bit AMF value in hexadecimal (4 characters)
- `plmn`: Must match DU PLMN configuration

## How to Run

### 1. Prerequisites Setup

Ensure SCTP support is installed:

```bash
# Ubuntu/Debian
sudo apt-get install -y libsctp-dev

# Verify SCTP kernel module
lsmod | grep sctp
```

### 2. Build the Project

```bash
# Clone the repository
git clone <repository-url>
cd du_ue

# Build the project
go build -o du-ue-simulator ./cmd/main.go
```

### 3. Configure

Edit `config/config.yml` to match your CU-CP setup:

- Set `cucp_address` to your CU-CP server IP
- Set `cucp_port` to your CU-CP F1AP port
- Configure PLMN (MCC/MNC) to match CU-CP configuration
- Configure UE credentials (key, opc, amf)

### 4. Run the Simulator

```bash
# Run with default config (config/config.yml)
./du-ue-simulator

# Or specify custom config path
./du-ue-simulator -config /path/to/config.yml
```

### 5. Expected Behavior

When running successfully, you should see:

1. **F1 Setup**: DU connects to CU-CP and performs F1 Setup procedure
2. **RRC Connection**: UE sends RRCSetupRequest, receives RRCSetup, sends RRCSetupComplete
3. **5GMM Registration**: UE performs registration procedure:
   - Registration Request (embedded in RRCSetupComplete)
   - Identity Request/Response
   - Authentication Request/Response
   - Security Mode Command/Complete
   - Registration Accept/Complete

### 6. Stop the Simulator

Press `Ctrl+C` to gracefully shutdown the simulator.

## Architecture

```
┌─────────────┐         F1AP/SCTP          ┌─────────────┐
│             │◄──────────────────────────►│             │
│     DU      │      (PPID=62, Port 38472) │   CU-CP     │
│             │                             │             │
└──────┬──────┘                             └─────────────┘
       │
       │ Channels (Go channels)
       │ RRC Messages
       │
┌──────▼──────┐
│             │
│     UE      │
│             │
└─────────────┘
```

### Message Flow

1. **F1 Setup**: DU ↔ CU-CP (F1 Setup Request/Response)
2. **RRC Setup**: UE → DU → CU-CP (RRCSetupRequest via Initial UL RRC Message Transfer)
3. **RRC Setup Response**: CU-CP → DU → UE (RRCSetup via DL RRC Message Transfer)
4. **RRC Setup Complete**: UE → DU → CU-CP (RRCSetupComplete with NAS Registration Request)
5. **NAS Registration**: CU-CP ↔ AMF ↔ UE (via DLInformationTransfer/RRCReconfiguration)

## Project Structure

```
du_ue/
├── cmd/
│   └── main.go              # Main entry point
├── config/
│   └── config.yml           # Configuration file
├── internal/
│   ├── common/
│   │   └── logger/          # Logging utilities
│   ├── du/
│   │   ├── du.go            # DU main logic
│   │   ├── f1ap_client.go   # F1AP SCTP client
│   │   ├── rrc_transfer.go  # RRC message transfer (UL/DL)
│   │   ├── ue_context_setup.go  # UE Context Setup handling
│   │   └── uplink_downlink.go   # UL/DL RRC Message Transfer
│   └── uecontext/
│       ├── ue.go            # UE context structure
│       ├── init.go          # UE initialization & RRC setup
│       ├── handle_rrc.go    # RRC message handling
│       ├── handle_n1mm.go    # NAS 5GMM message handling
│       ├── trigger.go       # Registration trigger
│       ├── auth.go           # Authentication handling
│       └── sec/             # Security context (Milenage, keys)
├── pkg/
│   └── config/              # Configuration loading
└── README.md
```

## Future Work

### Planned Enhancements

1. **Separate UE Module**
   - Extract UE functionality into a standalone module
   - Support multiple UE instances
   - Independent UE lifecycle management

2. **Extended Procedures**
   - **Handover**: Support inter-DU and intra-DU handover procedures
   - **PDU Session**: Implement 5GSM (5G Session Management) procedures
     - PDU Session Establishment
     - PDU Session Modification
     - PDU Session Release
   - **Service Request**: Support service request procedure for idle UEs
   - **Deregistration**: Support UE-initiated and network-initiated deregistration

3. **Enhanced Features**
   - Multiple UE support (multiple UEs per DU)
   - UE mobility simulation
   - Session management (multiple PDU sessions)
   - QoS handling
   - Measurement reporting
   - RRC Reconfiguration handling

4. **Testing & Validation**
   - Unit tests for RRC/NAS message encoding/decoding
   - Integration tests with CU-CP
   - Performance benchmarking
   - Stress testing with multiple UEs

5. **Documentation**
   - Detailed protocol flow diagrams
   - API documentation
   - Troubleshooting guide
   - Configuration examples for different scenarios
