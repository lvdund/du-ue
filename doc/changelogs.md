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