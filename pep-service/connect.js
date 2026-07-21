'use strict';
const grpc = require('@grpc/grpc-js');
const { signers } = require('@hyperledger/fabric-gateway');
const crypto = require('crypto');
const fs = require('fs').promises;
const path = require('path');

const MSP_ID = 'Org1MSP';
const TEST_NETWORK_HOME = path.resolve(__dirname, '..', 'test-network');
const CRYPTO_PATH = path.join(TEST_NETWORK_HOME, 'organizations', 'peerOrganizations', 'org1.example.com');

const KEY_DIRECTORY_PATH = path.join(CRYPTO_PATH, 'users', 'User1@org1.example.com', 'msp', 'keystore');
const CERT_DIRECTORY_PATH = path.join(CRYPTO_PATH, 'users', 'User1@org1.example.com', 'msp', 'signcerts');
const TLS_CERT_PATH = path.join(CRYPTO_PATH, 'peers', 'peer0.org1.example.com', 'tls', 'ca.crt');

const PEER_ENDPOINT = 'localhost:7051';
const PEER_HOST_ALIAS = 'peer0.org1.example.com';

async function newGrpcConnection() {
    const tlsRootCert = await fs.readFile(TLS_CERT_PATH);
    const tlsCredentials = grpc.credentials.createSsl(tlsRootCert);
    return new grpc.Client(PEER_ENDPOINT, tlsCredentials, { 'grpc.ssl_target_name_override': PEER_HOST_ALIAS });
}

async function newIdentity() {
    const certPath = await getFirstFileInDirectory(CERT_DIRECTORY_PATH);
    const credentials = await fs.readFile(certPath);
    return { mspId: MSP_ID, credentials };
}

async function newSigner() {
    const keyPath = await getFirstFileInDirectory(KEY_DIRECTORY_PATH);
    const privateKeyPem = await fs.readFile(keyPath);
    return signers.newPrivateKeySigner(crypto.createPrivateKey(privateKeyPem));
}

async function getFirstFileInDirectory(dirPath) {
    const files = await fs.readdir(dirPath);
    return path.join(dirPath, files[0]);
}

module.exports = { newGrpcConnection, newIdentity, newSigner };
