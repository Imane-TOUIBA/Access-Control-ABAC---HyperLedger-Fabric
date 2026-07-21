#!/bin/bash
set -e

########################
# CONFIG FIXE
########################

FABRIC_HOME=~/go/src/github.com/Imane-TOUIBA/fabric-samples
TESTNET=$FABRIC_HOME/test-network

CHANNEL_NAME="global-channel"
CC_NAME="attestationcontract"
CC_SRC_PATH="../attestationcc"
ORDERER_CA=$TESTNET/organizations/ordererOrganizations/example.com/orderers/orderer.example.com/msp/tlscacerts/tlsca.example.com-cert.pem

########################
# FORCE CONTEXT STABLE
########################

cd $TESTNET

export CORE_PEER_TLS_ENABLED=true

########################
# ORG FUNCTIONS (FIXES MSP BUG)
########################

setOrg1() {
export CORE_PEER_LOCALMSPID=Org1MSP
export CORE_PEER_ADDRESS=localhost:7051
export CORE_PEER_MSPCONFIGPATH=$TESTNET/organizations/peerOrganizations/org1.example.com/users/Admin@org1.example.com/msp
export CORE_PEER_TLS_ROOTCERT_FILE=$TESTNET/organizations/peerOrganizations/org1.example.com/peers/peer0.org1.example.com/tls/ca.crt
}

setOrg2() {
export CORE_PEER_LOCALMSPID=Org2MSP
export CORE_PEER_ADDRESS=localhost:9051
export CORE_PEER_MSPCONFIGPATH=$TESTNET/organizations/peerOrganizations/org2.example.com/users/Admin@org2.example.com/msp
export CORE_PEER_TLS_ROOTCERT_FILE=$TESTNET/organizations/peerOrganizations/org2.example.com/peers/peer0.org2.example.com/tls/ca.crt
}

setOrg3() {
export CORE_PEER_LOCALMSPID=Org3MSP
export CORE_PEER_ADDRESS=localhost:11051
export CORE_PEER_MSPCONFIGPATH=$TESTNET/organizations/peerOrganizations/org3.example.com/users/Admin@org3.example.com/msp
export CORE_PEER_TLS_ROOTCERT_FILE=$TESTNET/organizations/peerOrganizations/org3.example.com/peers/peer0.org3.example.com/tls/ca.crt
}

########################
# PACKAGE
########################

echo "Packaging chaincode..."

CC_VERSION="1.$(date +%s)"
CC_LABEL="${CC_NAME}_${CC_VERSION}"

peer lifecycle chaincode package ${CC_NAME}.tar.gz \
--path ${CC_SRC_PATH} \
--lang golang \
--label ${CC_LABEL}

########################
# INSTALL ALL ORGS
########################

echo "Installing on Org1..."
setOrg1
peer lifecycle chaincode install ${CC_NAME}.tar.gz

echo "Installing on Org2..."
setOrg2
peer lifecycle chaincode install ${CC_NAME}.tar.gz

echo "Installing on Org3..."
setOrg3
peer lifecycle chaincode install ${CC_NAME}.tar.gz

########################
# GET PACKAGE ID (FIXED)
########################

setOrg1

PACKAGE_ID=$(peer lifecycle chaincode queryinstalled \
| grep ${CC_LABEL} \
| awk -F'[,:]' '{print $2}' | xargs)

echo "PACKAGE_ID = $PACKAGE_ID"

########################
# AUTO SEQUENCE (FIX FIX FIX)
########################

CURRENT_SEQ=$(peer lifecycle chaincode querycommitted \
--channelID $CHANNEL_NAME \
--name $CC_NAME 2>/dev/null \
| grep Sequence | awk '{print $3}')

if [ -z "$CURRENT_SEQ" ]; then
  SEQ=1
else
  SEQ=$((CURRENT_SEQ + 1))
fi

echo "SEQUENCE = $SEQ"

########################
# APPROVE ALL ORGS
########################

approve() {
peer lifecycle chaincode approveformyorg \
-o localhost:7050 \
--ordererTLSHostnameOverride orderer.example.com \
--channelID $CHANNEL_NAME \
--name $CC_NAME \
--version $CC_VERSION \
--package-id $PACKAGE_ID \
--sequence $SEQ \
--tls \
--cafile $ORDERER_CA
}

echo "Approve Org1"
setOrg1
approve

echo "Approve Org2"
setOrg2
approve

echo "Approve Org3"
setOrg3
approve || true

########################
# COMMIT (FIXED TLS ALIGNMENT)
########################

echo "Committing chaincode..."

setOrg1

peer lifecycle chaincode commit \
-o localhost:7050 \
--ordererTLSHostnameOverride orderer.example.com \
--channelID $CHANNEL_NAME \
--name $CC_NAME \
--version $CC_VERSION \
--sequence $SEQ \
--tls \
--cafile $ORDERER_CA \
--peerAddresses localhost:7051 \
--tlsRootCertFiles $TESTNET/organizations/peerOrganizations/org1.example.com/peers/peer0.org1.example.com/tls/ca.crt \
--peerAddresses localhost:9051 \
--tlsRootCertFiles $TESTNET/organizations/peerOrganizations/org2.example.com/peers/peer0.org2.example.com/tls/ca.crt \
--peerAddresses localhost:11051 \
--tlsRootCertFiles $TESTNET/organizations/peerOrganizations/org3.example.com/peers/peer0.org3.example.com/tls/ca.crt

########################
# VERIFY
########################

echo "Verifying commit..."

peer lifecycle chaincode querycommitted \
--channelID $CHANNEL_NAME \
--name $CC_NAME

echo "DEPLOY DONE"
