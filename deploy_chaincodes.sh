#!/bin/bash
# =============================================================================
# deploy_chaincodes.sh
# Déploiement d'AttestationContract et PolicyContract
# Conforme à la documentation officielle Hyperledger Fabric :
# "Deploying a smart contract to a channel"
#
# Prérequis : réseau actif avec global-channel (CGN+IB+HU)
#             et project-channel (CGN+IB)
# =============================================================================

set -e

FABRIC_SAMPLES="$HOME/go/src/github.com/Imane-TOUIBA/fabric-samples"
TEST_NETWORK="$FABRIC_SAMPLES/test-network"
ORDERER_CA="$TEST_NETWORK/organizations/ordererOrganizations/example.com/orderers/orderer.example.com/msp/tlscacerts/tlsca.example.com-cert.pem"

export PATH="$FABRIC_SAMPLES/bin:$PATH"
export FABRIC_CFG_PATH="$FABRIC_SAMPLES/config"
export CORE_PEER_TLS_ENABLED=true

# Fonctions de basculement d'organisation
setCGN() {
    export CORE_PEER_LOCALMSPID=CGNMSP
    export CORE_PEER_ADDRESS=localhost:7051
    export CORE_PEER_MSPCONFIGPATH="$TEST_NETWORK/organizations/peerOrganizations/cgn.example.com/users/Admin@cgn.example.com/msp"
    export CORE_PEER_TLS_ROOTCERT_FILE="$TEST_NETWORK/organizations/peerOrganizations/cgn.example.com/peers/peer0.cgn.example.com/tls/ca.crt"
}

setIB() {
    export CORE_PEER_LOCALMSPID=IBMSP
    export CORE_PEER_ADDRESS=localhost:9051
    export CORE_PEER_MSPCONFIGPATH="$TEST_NETWORK/organizations/peerOrganizations/ib.example.com/users/Admin@ib.example.com/msp"
    export CORE_PEER_TLS_ROOTCERT_FILE="$TEST_NETWORK/organizations/peerOrganizations/ib.example.com/peers/peer0.ib.example.com/tls/ca.crt"
}

setHU() {
    export CORE_PEER_LOCALMSPID=HUMSP
    export CORE_PEER_ADDRESS=localhost:11051
    export CORE_PEER_MSPCONFIGPATH="$TEST_NETWORK/organizations/peerOrganizations/hu.example.com/users/Admin@hu.example.com/msp"
    export CORE_PEER_TLS_ROOTCERT_FILE="$TEST_NETWORK/organizations/peerOrganizations/hu.example.com/peers/peer0.hu.example.com/tls/ca.crt"
}

# =============================================================================
# PARTIE 1 : ATTESTATIONCONTRACT sur global-channel (CGN + IB + HU)
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

echo "Installation sur peer0.cgn.example.com..."
setCGN
peer lifecycle chaincode install attestationcc.tar.gz

echo "Installation sur peer0.ib.example.com..."
setIB
peer lifecycle chaincode install attestationcc.tar.gz

echo "Installation sur peer0.hu.example.com..."
setHU
peer lifecycle chaincode install attestationcc.tar.gz

echo ""
echo "=================================================================="
echo " ÉTAPE 4 : Récupération du Package ID — AttestationContract"
echo "=================================================================="
setCGN
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
echo " ÉTAPE 5 : Approbation — AttestationContract (CGN)"
echo "=================================================================="
setCGN
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
echo "CGN a approuvé AttestationContract."

echo ""
echo "=================================================================="
echo " ÉTAPE 6 : Approbation — AttestationContract (IB)"
echo "=================================================================="
setIB
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
echo "IB a approuvé AttestationContract."

echo ""
echo "=================================================================="
echo " ÉTAPE 7 : Approbation — AttestationContract (HU)"
echo "=================================================================="
setHU
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
echo "HU a approuvé AttestationContract."

echo ""
echo "=================================================================="
echo " ÉTAPE 8 : Vérification commit readiness — AttestationContract"
echo "=================================================================="
setCGN
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
setCGN
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
    --tlsRootCertFiles "$TEST_NETWORK/organizations/peerOrganizations/cgn.example.com/peers/peer0.cgn.example.com/tls/ca.crt" \
    --peerAddresses localhost:9051 \
    --tlsRootCertFiles "$TEST_NETWORK/organizations/peerOrganizations/ib.example.com/peers/peer0.ib.example.com/tls/ca.crt" \
    --peerAddresses localhost:11051 \
    --tlsRootCertFiles "$TEST_NETWORK/organizations/peerOrganizations/hu.example.com/peers/peer0.hu.example.com/tls/ca.crt"
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
# PARTIE 2 : POLICYCONTRACT sur project-channel (CGN + IB seulement)
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

echo "Installation sur peer0.cgn.example.com..."
setCGN
peer lifecycle chaincode install policycc.tar.gz

echo "Installation sur peer0.ib.example.com..."
setIB
peer lifecycle chaincode install policycc.tar.gz

echo ""
echo "=================================================================="
echo " ÉTAPE 14 : Récupération du Package ID — PolicyContract"
echo "=================================================================="
setCGN
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
echo " ÉTAPE 15 : Approbation — PolicyContract (CGN)"
echo "=================================================================="
setCGN
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
echo "CGN a approuvé PolicyContract."

echo ""
echo "=================================================================="
echo " ÉTAPE 16 : Approbation — PolicyContract (IB)"
echo "=================================================================="
setIB
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
echo "IB a approuvé PolicyContract."

echo ""
echo "=================================================================="
echo " ÉTAPE 17 : Vérification commit readiness — PolicyContract"
echo "=================================================================="
setCGN
peer lifecycle chaincode checkcommitreadiness \
    --channelID project-channel \
    --name policycc \
    --version 1.0 \
    --sequence 1 \
    --tls \
    --cafile "$ORDERER_CA" \
    --output json
echo "CGNMSP et IBMSP doivent afficher true."

echo ""
echo "=================================================================="
echo " ÉTAPE 18 : Commit — PolicyContract sur project-channel"
echo "=================================================================="
setCGN
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
    --tlsRootCertFiles "$TEST_NETWORK/organizations/peerOrganizations/cgn.example.com/peers/peer0.cgn.example.com/tls/ca.crt" \
    --peerAddresses localhost:9051 \
    --tlsRootCertFiles "$TEST_NETWORK/organizations/peerOrganizations/ib.example.com/peers/peer0.ib.example.com/tls/ca.crt"
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
echo " AttestationContract : global-channel (CGN, IB, HU)"
echo " PolicyContract      : project-channel (CGN, IB)"
echo "=================================================================="
