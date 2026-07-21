#!/bin/bash
# =============================================================================
# deploy_chaincodes.sh
# Déploiement d'AttestationContract et PolicyContract
# Conforme à la documentation officielle Hyperledger Fabric :
# "Deploying a smart contract to a channel"
#
# Prérequis : réseau actif avec global-channel (Org1+Org2+Org3)
#             et project-channel (Org1+Org2)
# =============================================================================

set -e

FABRIC_SAMPLES="$HOME/go/src/github.com/Imane-TOUIBA/fabric-samples"
TEST_NETWORK="$FABRIC_SAMPLES/test-network"
ORDERER_CA="$TEST_NETWORK/organizations/ordererOrganizations/example.com/orderers/orderer.example.com/msp/tlscacerts/tlsca.example.com-cert.pem"

export PATH="$FABRIC_SAMPLES/bin:$PATH"
export FABRIC_CFG_PATH="$FABRIC_SAMPLES/config"
export CORE_PEER_TLS_ENABLED=true

# Fonctions de basculement d'organisation
setOrg1() {
    export CORE_PEER_LOCALMSPID=Org1MSP
    export CORE_PEER_ADDRESS=localhost:7051
    export CORE_PEER_MSPCONFIGPATH="$TEST_NETWORK/organizations/peerOrganizations/org1.example.com/users/Admin@org1.example.com/msp"
    export CORE_PEER_TLS_ROOTCERT_FILE="$TEST_NETWORK/organizations/peerOrganizations/org1.example.com/peers/peer0.org1.example.com/tls/ca.crt"
}

setOrg2() {
    export CORE_PEER_LOCALMSPID=Org2MSP
    export CORE_PEER_ADDRESS=localhost:9051
    export CORE_PEER_MSPCONFIGPATH="$TEST_NETWORK/organizations/peerOrganizations/org2.example.com/users/Admin@org2.example.com/msp"
    export CORE_PEER_TLS_ROOTCERT_FILE="$TEST_NETWORK/organizations/peerOrganizations/org2.example.com/peers/peer0.org2.example.com/tls/ca.crt"
}

setOrg3() {
    export CORE_PEER_LOCALMSPID=Org3MSP
    export CORE_PEER_ADDRESS=localhost:11051
    export CORE_PEER_MSPCONFIGPATH="$TEST_NETWORK/organizations/peerOrganizations/org3.example.com/users/Admin@org3.example.com/msp"
    export CORE_PEER_TLS_ROOTCERT_FILE="$TEST_NETWORK/organizations/peerOrganizations/org3.example.com/peers/peer0.org3.example.com/tls/ca.crt"
}

# =============================================================================
# PARTIE 1 : ATTESTATIONCONTRACT sur global-channel (Org1 + Org2 + Org3)
# =============================================================================

echo ""
echo "=================================================================="
echo " ÉTAPE 1 : Installation des dépendances Go — AttestationContract"
echo "=================================================================="
cd "$FABRIC_SAMPLES/attestationcc"
GO111MODULE=on go mod tidy
GO111MODULE=on go mod vendor
echo "Dépendances AttestationContract installées."

echo ""
echo "=================================================================="
echo " ÉTAPE 2 : Packaging — AttestationContract"
echo "=================================================================="
cd "$TEST_NETWORK"
peer lifecycle chaincode package attestationcc.tar.gz \
    --path "$FABRIC_SAMPLES/attestationcc" \
    --lang golang \
    --label attestationcc_1.0
echo "Package attestationcc.tar.gz créé."

echo ""
echo "=================================================================="
echo " ÉTAPE 3 : Installation sur les peers — AttestationContract"
echo "=================================================================="

echo "Installation sur peer0.org1.example.com..."
setOrg1
peer lifecycle chaincode install attestationcc.tar.gz

echo "Installation sur peer0.org2.example.com..."
setOrg2
peer lifecycle chaincode install attestationcc.tar.gz

echo "Installation sur peer0.org3.example.com..."
setOrg3
peer lifecycle chaincode install attestationcc.tar.gz

echo ""
echo "=================================================================="
echo " ÉTAPE 4 : Récupération du Package ID — AttestationContract"
echo "=================================================================="
setOrg1
peer lifecycle chaincode queryinstalled
echo ""
echo "ATTENTION : copie le Package ID de attestationcc_1.0 ci-dessus"
echo "Appuie sur Entrée une fois que tu as copié le Package ID..."
read -r
echo -n "Colle le Package ID ici : "
read -r ATTESTATION_PKG_ID
export CC_PACKAGE_ID_ATTEST="$ATTESTATION_PKG_ID"
echo "Package ID enregistré : $CC_PACKAGE_ID_ATTEST"

echo ""
echo "=================================================================="
echo " ÉTAPE 5 : Approbation — AttestationContract (Org1)"
echo "=================================================================="
setOrg1
peer lifecycle chaincode approveformyorg \
    -o localhost:7050 \
    --ordererTLSHostnameOverride orderer.example.com \
    --channelID global-channel \
    --name attestationcc \
    --version 1.0 \
    --package-id "$CC_PACKAGE_ID_ATTEST" \
    --sequence 1 \
    --tls \
    --cafile "$ORDERER_CA"
echo "Org1 a approuvé AttestationContract."

echo ""
echo "=================================================================="
echo " ÉTAPE 6 : Approbation — AttestationContract (Org2)"
echo "=================================================================="
setOrg2
peer lifecycle chaincode approveformyorg \
    -o localhost:7050 \
    --ordererTLSHostnameOverride orderer.example.com \
    --channelID global-channel \
    --name attestationcc \
    --version 1.0 \
    --package-id "$CC_PACKAGE_ID_ATTEST" \
    --sequence 1 \
    --tls \
    --cafile "$ORDERER_CA"
echo "Org2 a approuvé AttestationContract."

echo ""
echo "=================================================================="
echo " ÉTAPE 7 : Approbation — AttestationContract (Org3)"
echo "=================================================================="
setOrg3
peer lifecycle chaincode approveformyorg \
    -o localhost:7050 \
    --ordererTLSHostnameOverride orderer.example.com \
    --channelID global-channel \
    --name attestationcc \
    --version 1.0 \
    --package-id "$CC_PACKAGE_ID_ATTEST" \
    --sequence 1 \
    --tls \
    --cafile "$ORDERER_CA"
echo "Org3 a approuvé AttestationContract."

echo ""
echo "=================================================================="
echo " ÉTAPE 8 : Vérification commit readiness — AttestationContract"
echo "=================================================================="
setOrg1
peer lifecycle chaincode checkcommitreadiness \
    --channelID global-channel \
    --name attestationcc \
    --version 1.0 \
    --sequence 1 \
    --tls \
    --cafile "$ORDERER_CA" \
    --output json
echo "Les trois organisations doivent afficher true."

echo ""
echo "=================================================================="
echo " ÉTAPE 9 : Commit — AttestationContract sur global-channel"
echo "=================================================================="
setOrg1
peer lifecycle chaincode commit \
    -o localhost:7050 \
    --ordererTLSHostnameOverride orderer.example.com \
    --channelID global-channel \
    --name attestationcc \
    --version 1.0 \
    --sequence 1 \
    --tls \
    --cafile "$ORDERER_CA" \
    --peerAddresses localhost:7051 \
    --tlsRootCertFiles "$TEST_NETWORK/organizations/peerOrganizations/org1.example.com/peers/peer0.org1.example.com/tls/ca.crt" \
    --peerAddresses localhost:9051 \
    --tlsRootCertFiles "$TEST_NETWORK/organizations/peerOrganizations/org2.example.com/peers/peer0.org2.example.com/tls/ca.crt" \
    --peerAddresses localhost:11051 \
    --tlsRootCertFiles "$TEST_NETWORK/organizations/peerOrganizations/org3.example.com/peers/peer0.org3.example.com/tls/ca.crt"
echo "AttestationContract commité sur global-channel."

echo ""
echo "=================================================================="
echo " ÉTAPE 10 : Vérification commit — AttestationContract"
echo "=================================================================="
peer lifecycle chaincode querycommitted \
    --channelID global-channel \
    --name attestationcc
echo ""
echo "AttestationContract déployé avec succès sur global-channel."
echo ""

# =============================================================================
# PARTIE 2 : POLICYCONTRACT sur project-channel (Org1 + Org2 seulement)
# =============================================================================

echo ""
echo "=================================================================="
echo " ÉTAPE 11 : Installation des dépendances Go — PolicyContract"
echo "=================================================================="
cd "$FABRIC_SAMPLES/policycc"
GO111MODULE=on go mod tidy
GO111MODULE=on go mod vendor
echo "Dépendances PolicyContract installées."

echo ""
echo "=================================================================="
echo " ÉTAPE 12 : Packaging — PolicyContract"
echo "=================================================================="
cd "$TEST_NETWORK"
peer lifecycle chaincode package policycc.tar.gz \
    --path "$FABRIC_SAMPLES/policycc" \
    --lang golang \
    --label policycc_1.0
echo "Package policycc.tar.gz créé."

echo ""
echo "=================================================================="
echo " ÉTAPE 13 : Installation sur les peers — PolicyContract"
echo "=================================================================="

echo "Installation sur peer0.org1.example.com..."
setOrg1
peer lifecycle chaincode install policycc.tar.gz

echo "Installation sur peer0.org2.example.com..."
setOrg2
peer lifecycle chaincode install policycc.tar.gz

echo ""
echo "=================================================================="
echo " ÉTAPE 14 : Récupération du Package ID — PolicyContract"
echo "=================================================================="
setOrg1
peer lifecycle chaincode queryinstalled
echo ""
echo "ATTENTION : copie le Package ID de policycc_1.0 ci-dessus"
echo "Appuie sur Entrée une fois que tu as copié le Package ID..."
read -r
echo -n "Colle le Package ID ici : "
read -r POLICY_PKG_ID
export CC_PACKAGE_ID_POLICY="$POLICY_PKG_ID"
echo "Package ID enregistré : $CC_PACKAGE_ID_POLICY"

echo ""
echo "=================================================================="
echo " ÉTAPE 15 : Approbation — PolicyContract (Org1)"
echo "=================================================================="
setOrg1
peer lifecycle chaincode approveformyorg \
    -o localhost:7050 \
    --ordererTLSHostnameOverride orderer.example.com \
    --channelID project-channel \
    --name policycc \
    --version 1.0 \
    --package-id "$CC_PACKAGE_ID_POLICY" \
    --sequence 1 \
    --tls \
    --cafile "$ORDERER_CA"
echo "Org1 a approuvé PolicyContract."

echo ""
echo "=================================================================="
echo " ÉTAPE 16 : Approbation — PolicyContract (Org2)"
echo "=================================================================="
setOrg2
peer lifecycle chaincode approveformyorg \
    -o localhost:7050 \
    --ordererTLSHostnameOverride orderer.example.com \
    --channelID project-channel \
    --name policycc \
    --version 1.0 \
    --package-id "$CC_PACKAGE_ID_POLICY" \
    --sequence 1 \
    --tls \
    --cafile "$ORDERER_CA"
echo "Org2 a approuvé PolicyContract."

echo ""
echo "=================================================================="
echo " ÉTAPE 17 : Vérification commit readiness — PolicyContract"
echo "=================================================================="
setOrg1
peer lifecycle chaincode checkcommitreadiness \
    --channelID project-channel \
    --name policycc \
    --version 1.0 \
    --sequence 1 \
    --tls \
    --cafile "$ORDERER_CA" \
    --output json
echo "Org1MSP et Org2MSP doivent afficher true."

echo ""
echo "=================================================================="
echo " ÉTAPE 18 : Commit — PolicyContract sur project-channel"
echo "=================================================================="
setOrg1
peer lifecycle chaincode commit \
    -o localhost:7050 \
    --ordererTLSHostnameOverride orderer.example.com \
    --channelID project-channel \
    --name policycc \
    --version 1.0 \
    --sequence 1 \
    --tls \
    --cafile "$ORDERER_CA" \
    --peerAddresses localhost:7051 \
    --tlsRootCertFiles "$TEST_NETWORK/organizations/peerOrganizations/org1.example.com/peers/peer0.org1.example.com/tls/ca.crt" \
    --peerAddresses localhost:9051 \
    --tlsRootCertFiles "$TEST_NETWORK/organizations/peerOrganizations/org2.example.com/peers/peer0.org2.example.com/tls/ca.crt"
echo "PolicyContract commité sur project-channel."

echo ""
echo "=================================================================="
echo " ÉTAPE 19 : Vérification commit — PolicyContract"
echo "=================================================================="
peer lifecycle chaincode querycommitted \
    --channelID project-channel \
    --name policycc
echo ""
echo "PolicyContract déployé avec succès sur project-channel."
echo ""
echo "=================================================================="
echo " DÉPLOIEMENT TERMINÉ"
echo " AttestationContract : global-channel (Org1, Org2, Org3)"
echo " PolicyContract      : project-channel (Org1, Org2)"
echo "=================================================================="
