'use strict';

const grpc = require('@grpc/grpc-js');
const { signers } = require('@hyperledger/fabric-gateway');
const crypto = require('crypto');
const fs = require('fs').promises;
const path = require('path');

// ---------------------------------------------------------------------------
// Configuration du relais.
//
// Le relais agit avec l'identite MSP d'Org1MSP. Ce choix est arbitraire entre
// Org1MSP et Org2MSP : les deux organisations sont membres de global-channel
// ET de project-channel, ce qui est la condition necessaire pour pouvoir a la
// fois ecouter les evenements sur global-channel et soumettre une transaction
// sur project-channel. Org3MSP ne pourrait pas jouer ce role car elle n'est
// pas membre de project-channel.
//
// ADAPTER ces chemins a l'emplacement reel de ton repertoire test-network.
// ---------------------------------------------------------------------------

const MSP_ID = 'Org1MSP';

const CRYPTO_PATH = path.resolve(
    process.env.TEST_NETWORK_HOME || path.join(__dirname, '..', 'test-network'),
    'organizations', 'peerOrganizations', 'org1.example.com',
);

const KEY_DIRECTORY_PATH = path.resolve(
    CRYPTO_PATH, 'users', 'User1@org1.example.com', 'msp', 'keystore',
);

const CERT_DIRECTORY_PATH = path.resolve(
    CRYPTO_PATH, 'users', 'User1@org1.example.com', 'msp', 'signcerts',
);

const TLS_CERT_PATH = path.resolve(
    CRYPTO_PATH, 'peers', 'peer0.org1.example.com', 'tls', 'ca.crt',
);

// Adresse du peer gRPC d'Org1, par defaut celle de test-network.
const PEER_ENDPOINT = process.env.PEER_ENDPOINT || 'localhost:7051';
const PEER_HOST_ALIAS = process.env.PEER_HOST_ALIAS || 'peer0.org1.example.com';

/**
 * Cree une connexion gRPC TLS vers le peer d'Org1.
 * Cette connexion unique est reutilisee pour les deux canaux
 * (global-channel et project-channel), puisque le meme peer Org1
 * heberge les deux.
 */
async function newGrpcConnection() {
    const tlsRootCert = await fs.readFile(TLS_CERT_PATH);
    const tlsCredentials = grpc.credentials.createSsl(tlsRootCert);
    return new grpc.Client(PEER_ENDPOINT, tlsCredentials, {
        'grpc.ssl_target_name_override': PEER_HOST_ALIAS,
    });
}

/**
 * Construit l'identite Fabric (mspId + certificat) du relais.
 */
async function newIdentity() {
    const certPath = await getFirstFileInDirectory(CERT_DIRECTORY_PATH);
    const credentials = await fs.readFile(certPath);
    return { mspId: MSP_ID, credentials };
}

/**
 * Construit le signataire cryptographique du relais a partir de sa cle
 * privee. Conformement au perimetre du stage, la gestion de cette cle est
 * consideree comme deja resolue en amont (deja fournie par test-network) ;
 * le relais se contente de l'utiliser pour signer ses transactions.
 */
async function newSigner() {
    const keyPath = await getFirstFileInDirectory(KEY_DIRECTORY_PATH);
    const privateKeyPem = await fs.readFile(keyPath);
    const privateKey = crypto.createPrivateKey(privateKeyPem);
    return signers.newPrivateKeySigner(privateKey);
}

async function getFirstFileInDirectory(dirPath) {
    const files = await fs.readdir(dirPath);
    const file = files[0];
    if (!file) {
        throw new Error(`Aucun fichier trouve dans le repertoire : ${dirPath}`);
    }
    return path.join(dirPath, file);
}

module.exports = { newGrpcConnection, newIdentity, newSigner, MSP_ID };

