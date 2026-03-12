# O-RAN WG5.C.1-v17.00 F1-C Procedure Checklist

This checklist contains all procedures and call flow diagrams (figures) from the O-RAN WG5.C.1-v17.00 specification, filtered to include only **F1-C** and **NR-Standalone** operations.

---

## **Group 1: Foundations & Setup (Sections 1 & 4)**
*Manage the initial connectivity and non-UE-associated interface logic.*

### **Procedures**
- [ ] 1.1.2 NR Standalone (SA)
- [ ] 1.3.1 IAB Node connectivity in Standalone mode
- [ ] 4.1.2 Reset (EN-DC context)
- [ ] 4.1.3 EN-DC Configuration Update
- [ ] 4.1.4 Partial Reset
- [ ] 4.1.5 F1 Setup
- [ ] 4.1.6 gNB-DU Configuration Update
- [ ] 4.1.7 gNB-CU Configuration Update
- [ ] 4.1.8 Mobility Load Balancing
- [ ] 4.2.2 Reset (Standalone)
- [ ] 4.2.3 F1 Setup (Standalone)
- [ ] 4.2.4 gNB-DU configuration update
- [ ] 4.2.5 gNB-CU configuration update
- [ ] 4.2.6 Paging (CN-initiated)
- [ ] 4.2.7 Write-Replace Warning
- [ ] 4.2.8 NG-RAN Node Configuration Update
- [ ] 4.2.9 Mobility Load Balancing

### **Call Flow Diagrams**
- [ ] Figure 1.3.1-1: IAB Node connectivity in Standalone mode
- [ ] Figure 4.1.2.3.1-1: gNB-DU initiated (Whole) Reset
- [ ] Figure 4.1.2.4.1-1: gNB-CU initiated (Whole) Reset
- [ ] Figure 4.1.5.1.1-1: F1 Setup
- [ ] Figure 4.1.6.1.1-1: gNB-DU Configuration Update
- [ ] Figure 4.1.7.1.1-1: gNB-CU Configuration Update
- [ ] Figure 4.2.3.1.1-1: F1 Setup (Standalone)
- [ ] Figure 4.2.6.1-1: Paging flow

---

## **Group 2: F1 in Dual Connectivity (Sections 5 & 7)**
*Procedures specifically involving F1-C interactions during EN-DC or NR-DC operations.*

### **Procedures**
- [ ] 5.3.1 Secondary Node Change (MN initiated)
- [ ] 5.3.2 Secondary Node Change (SN initiated)
- [ ] 5.6.4 SCG config query (MN initiated)
- [ ] 5.6.10 Addition of SN terminated split bearer
- [ ] 5.6.11 Removal of SN terminated split bearer
- [ ] 5.6.12/13/14 Bearer type changes (Split <-> MCG)
- [ ] 5.6.15/16 PSCell Change using SRB1
- [ ] 5.6.17/19 Intra gNB-DU PSCell Change
- [ ] 5.6.18 Inter gNB-DU PSCell Change
- [ ] 5.6.20 gNB-DU configuration query

### **Call Flow Diagrams**
- [ ] Figure 5.3.1.3-1: SN Change procedure (F1 flow)
- [ ] Figure 5.6.15.3-1: Inter gNB-DU PSCell change
- [ ] Figure 5.6.20.1-1: gNB-DU configuration query
- [ ] Figure 7.2.1.1.3-1: MN initiated SN Modification – PDU Session addition

---

## **Group 3: NR Standalone - Access & Context (Sections 6.1 - 6.3)**
*Core logic for UE lifecycle management. [CRITICAL]*

### **Procedures**
- [ ] 6.1.1 UE context creation (service request)
- [ ] 6.1.2 UE context creation (registration request)
- [ ] 6.1.3 Registration Update
- [ ] 6.2.1 gNB-DU initiated UE Context Release
- [ ] 6.2.2 gNB-CU initiated UE Context Release
- [ ] 6.3.1 gNB-CU initiated UE Context Modification
- [ ] 6.3.2 gNB-DU initiated UE Context Modification
- [ ] 6.3.3 QoS and QFI management (PDU Session Establishment)

### **Call Flow Diagrams**
- [ ] **Figure 6.1.1.2-1: Initial access - UE context creation**
- [ ] **Figure 6.1.2.2-1: Initial access - Registration Request**
- [ ] **Figure 6.3.3.1-1: PDU session establishment**
- [ ] Figure 6.2.2.1-1: gNB-CU initiated UE Context Release
- [ ] Figure 6.3.1-1: gNB-CU initiated UE Context Modification

---

## **Group 4: NR Standalone - Mobility & RRC (Sections 6.5 - 6.12)**
*Advanced UE state and mobility management over F1.*

### **Procedures**
- [ ] 6.5.1 Intra gNB-DU, Intra Cell Handover
- [ ] 6.5.2 Intra gNB-DU, Inter Cell Handover
- [ ] 6.5.3 Inter gNB-DU Handover
- [ ] 6.5.4 Intra gNB-DU Handover with LTM
- [ ] 6.9.1 RRC connected to RRC inactive
- [ ] 6.9.2 RRC inactive to RRC connected (Intra gNB-CU)
- [ ] 6.10.1 RRC Connection Re-establishment (Intra gNB-DU)
- [ ] 6.10.2 RRC Connection Re-establishment (Inter gNB-DU)
- [ ] 6.11.1 System Information Delivery
- [ ] 6.12 PDU session establishment after signalling-only connection

### **Call Flow Diagrams**
- [ ] Figure 6.5.3.1-1: Inter gNB-DU Handover
- [ ] Figure 6.9.1.1-1: RRC connected to RRC inactive
- [ ] Figure 6.10.2.1-1: RRC Connection Re-establishment (Inter gNB-DU)
- [ ] Figure 6.11.1.1-1: System Information Delivery - F1 message flow

---

## **Group 5: IAB Procedures (Section 8)**
*Backhaul and donor DU setup.*

### **Procedures**
- [ ] 8.1.1 IAB Donor DU Setup
- [ ] 8.1.2 IAB Node integration

### **Call Flow Diagrams**
- [ ] Figure 8.1.2.1.1-1: IAB-MT and BH RLC Channel setup
- [ ] Figure 8.1.2.2.3-1: Routing Update on Donor DU
- [ ] Figure 8.1.2.4.1-1: gNB-DU Resource configuration
- [ ] Figure 8.1.2.5.1-1: Uplink User-plane mapping
