# Changelog

## Thêm mới

### DU
- `f1ap_handover.go`: Xử lý F1AP handover (source DU)
  - HandleUeContextModificationRequest, sendUeContextModificationResponse
  - HandleUeContextReleaseCommand, sendUeContextReleaseComplete
  - sendMeasurementReport
  
- `f1ap_target_handover.go`: Xử lý F1AP handover (target DU)
  - HandleUeContextSetupRequest, sendUeContextSetupResponse
  - allocateHandoverResources, createHandoverUeContext
  - HandleRandomAccessPreamble, sendRandomAccessResponse
  - HandleRrcReconfigurationComplete, HandleTargetUeContextModification

- `du_rach.go`: Xử lý RACH
  - StartRachMonitoring, SimulateRachReception

- `du_handover_state.go`: State machine cho handover
  - SetTargetHandoverState, IsTargetDU, IsSourceDU

### UE
- `ue_handover.go`: Xử lý measurement và handover
  - initMeasurement, TriggerMeasurement, sendMeasurementReport
  - HandleRrcReconfiguration, performRandomAccess
  - sendRrcReconfigurationComplete

- `ue_rrc_handler.go`: Xử lý RRC messages
  - HandleRrcMessage, handleDlCcchMessage, handleDlDcchMessage
  - handleRrcReconfigurationMessage, handleHandoverReconfiguration
  - handleRrcSetup, handleDlInformationTransfer, handleRrcRelease
  - sendRrcSetupComplete

## Cập nhật

### DU
- `du.go`: Thêm field `hoCtx *HandoverContext`, khởi tạo trong NewDU()
- `f1ap_client.go`: Thêm case UEContextModification và UEContextRelease

### UE
- `uecontext.go`: Thêm field `measurement *MeasurementContext`, functions InitUE() và handleRrcFromDU()

---

#### [2026-02-12] DU Handover & RACH Implementation
**feat(du): implement RACH procedure and RRM-based Handover logic**
- Implement `StartRachMonitoring` to validate Target DU role and Handover State (Preparation -> Execution).
- Implement `SimulateRachReception` to trigger the Random Access procedure upon handover request.
- Implement `sendRandomAccessResponse` (Msg2) to construct and transmit a valid MAC RAR PDU (header + payload) to the UE channel.
- Refactor `ue_context_setup.go`: Remove deprecated RACH stubs and redirect logic to the new `du_rach.go` module.
- Update `RACHContext` to support per-session tracking with Preamble ID validation.

**Action: Add logic into du_rach.go:**
- Update `RACHContext`: Now includes `preambleId` (Key), `tempCrnti` (Badge), `state` (Progress), and `ueChannel`.
- Implement `StartRachMonitoring`: Called by `handleTargetHandoverSetup`
  - Checks `du.IsTargetDU()` to ensure source doesn't run this.
  - Checks state is `HO_STATE_PREPARATION`.
  - Transitions state to `HO_STATE_EXECUTION` and logs the window open.
- Implement `SimulateRachReception`:
  - Creates the `RACHContext`.
  - Triggers sending Msg2 (`sendRandomAccessResponse`).
  - Logs waiting for Msg3.
- Implement `sendRandomAccessResponse(Msg2)`:
  - Constructs simulated RAR Payload (7 bytes: Header + TA + Grant + TC-RNTI).
  - Sends to `du.ue.SendToUeChannel`.

**Action 1: handleRrcFromUE() function**
- Listens for all RRC messages from the UE channel.
- Calls `dispatchRrcMessage()` to route to handlers.
- Sends to CU via `sendInitialULRRCMessageTransfer()` (first msg) or `sendULRRCMessageTransfer()` (others).

**Action 2: dispatchRrcMessage() function**
- Peeks into `UL-DCCH-Message` to trigger DU logic.
- Checks if C1 message is:
  - `MeasurementReport` -> Calls `handleMeasurementReport`.
  - `RRCReconfigurationComplete` -> Calls `handleRrcReconfigurationComplete`.

**Action 3: handleRrcReconfigurationComplete()**
- Checks if `du.IsTargetDU()` is true.
- If yes, sets Handover State to `HO_STATE_COMPLETED`. (Message is then forwarded to CU by the main loop).

**Action 4: handleMeasurementReport() [UPDATED]**
- **PCI Validation:** Calls `isValidNeighbor` to verify the target PCI is in the allowed neighbor list.
- **Admission Control:** Calls `checkAdmissionControl` to simulate load checks (rejects overloaded cells).
- If RRM checks pass, calls `TriggerHandover`.

**Action 5: TriggerHandover()**
- Sets state to `HO_STATE_PREPARATION`.
- Calls `sendUeContextModificationRequired` (`f1ap_handover.go`).

**Implement sendUeContextModificationRequired inside f1ap_handover.go**
- Constructs and sends the F1AP Handover Request to the CU.

**Current status:** Not tested yet.