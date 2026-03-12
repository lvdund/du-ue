#!/bin/bash

# ==============================================================================
# etrib5gc UE Provisioning Script
# ==============================================================================
# This script automatically provisions multiple User Equipments (UEs) into the 
# etrib5gc Core Network's MongoDB database (UDR).
#
# HOW IT WORKS:
# It connects to the local 'etrib5gc' MongoDB instance using 'mongosh' and clones 
# the existing security, access, and mobility credentials from UE 1 
# (imsi-001010000000001). It increments the IMSI and resets the SQN sequence 
# number for each new UE so they are ready for a fresh 5G-AKA authentication.
#
# USAGE: 
#   ./provision_ues_etribb5gc.sh <number_of_ues>
#
# EXAMPLE: To provision 50 UEs for the DU-UE simulator:
#   ./provision_ues_etribb5gc.sh 50
#
# AFTER RUNNING:
# Remember to update your du-ue simulator configuration to match:
#   1. Open: du-ue/config/config.yml
#   2. Set:  nue: <number_of_ues>
# ==============================================================================

if [ -z "$1" ]; then
    echo "Usage: $0 <number_of_ues>"
    echo "Example: $0 50"
    exit 1
fi

NUM_UES=$1

echo "Provisioning $NUM_UES UEs in etrib5gc MongoDB..."

# Create the temporary javascript file
cat << EOF > /tmp/provision_temp.js
const targetUes = $NUM_UES; 

// 1. Clone Authentication Data (authsub)
const authSource = db.authsub.findOne({ _id: 'imsi-001010000000001' });
if (authSource) {
  for (let i = 1; i <= targetUes; i++) {
    const newImsi = 'imsi-00101' + i.toString().padStart(10, '0');
    
    let newDoc = Object.assign({}, authSource);
    newDoc._id = newImsi;
    newDoc.ueId = newImsi;
    
    if (newDoc.dat) {
      newDoc.dat = Object.assign({}, authSource.dat);
      newDoc.dat.supi = newImsi;
      newDoc.dat.sequencenumber = Object.assign({}, authSource.dat.sequencenumber);
      newDoc.dat.sequencenumber.sqn = '000000000000'; // Reset SQN 
    }
    
    db.authsub.updateOne({_id: newImsi}, {\$set: newDoc}, {upsert: true});
  }
  print(\`✅ Provisioned \${targetUes} authentication (authsub) records\`);
} else {
  print(\`❌ Error: Source authsub imsi-001010000000001 not found.\`);
}

// 2. Clone Access & Mobility Data (amsub)
const amSource = db.amsub.findOne({ _id: 'imsi-001010000000001_001-01' });
if (amSource) {
  for (let i = 1; i <= targetUes; i++) {
    const newImsi = 'imsi-00101' + i.toString().padStart(10, '0');
    const newId = newImsi + '_001-01';
    
    let newDoc = Object.assign({}, amSource);
    newDoc._id = newId;
    newDoc.ueId = newImsi;
    
    db.amsub.updateOne({_id: newId}, {\$set: newDoc}, {upsert: true});
  }
  print(\`✅ Provisioned \${targetUes} mobility (amsub) records\`);
} else {
  print(\`❌ Error: Source amsub imsi-001010000000001_001-01 not found.\`);
}
EOF

# Execute the script against the etrib5gc database with mongosh
mongosh etrib5gc /tmp/provision_temp.js --quiet

# Clean up
rm /tmp/provision_temp.js

# Count total UEs in database
TOTAL_UES=$(mongosh etrib5gc --quiet --eval 'db.authsub.countDocuments({})')

echo ""
echo "Done! You can now update 'nue: $NUM_UES' in config/config.yml and run your simulator."
echo "Total UEs currently in the Core Network database: $TOTAL_UES"
