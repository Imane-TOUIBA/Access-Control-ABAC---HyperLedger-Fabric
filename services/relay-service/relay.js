'use strict';

const { connect, hash, checkpointers, GatewayError } = require('@hyperledger/fabric-gateway');
const grpc = require('@grpc/grpc-js');
const { newGrpcConnection, newIdentity, newSigner } = require('./connect');

// ---------------------------------------------------------------------------
// Ce service relais resout la limitation native de Hyperledger Fabric selon
// laquelle un chaincode ne peut pas invoquer en ecriture un chaincode deploye
// sur un canal different (un appel inter-canaux via InvokeChaincode n'est
// autorise qu'en lecture, et seulement si les deux chaincodes sont installes
// sur le meme peer). Le relais materialise donc le mecanisme decrit comme
// "peer relais + event listener" : un noeud membre des deux canaux ecoute un
// evenement sur le canal public (global-channel) et le retranscrit sous
// forme d'une nouvelle transaction, soumise au consensus du canal prive
// (project-channel).
//
// Important : le relais n'est pas un raccourci de confiance. La transaction
// qu'il soumet sur project-channel doit etre endossee et validee par le
// consensus de ce canal exactement comme n'importe quelle transaction
// normale ; le relais ne fait que porter l'information d'un canal a l'autre.
// ---------------------------------------------------------------------------

const GLOBAL_CHANNEL = process.env.GLOBAL_CHANNEL || 'global-channel';
const PROJECT_CHANNEL = process.env.PROJECT_CHANNEL || 'project-channel';
const GOVERNANCE_CHAINCODE = process.env.GOVERNANCE_CHAINCODE || 'governancecc';
const POLICY_CHAINCODE = process.env.POLICY_CHAINCODE || 'policycc';

const ATTESTATION_EVENT_NAME = 'AttestationValidated';

const utf8Decoder = new TextDecoder();

async function main() {
    const client = await newGrpcConnection();

    const gateway = connect({
        client,
        identity: await newIdentity(),
        signer: await newSigner(),
        hash: hash.sha256,
        evaluateOptions: () => ({ deadline: Date.now() + 5000 }),
        endorseOptions: () => ({ deadline: Date.now() + 15000 }),
        submitOptions: () => ({ deadline: Date.now() + 5000 }),
        commitStatusOptions: () => ({ deadline: Date.now() + 60000 }),
    });

    console.log(`[relay] connecte avec l'identite CGNMSP`);

    try {
        const globalNetwork = gateway.getNetwork(GLOBAL_CHANNEL);
        const projectNetwork = gateway.getNetwork(PROJECT_CHANNEL);
        const policyContract = projectNetwork.getContract(POLICY_CHAINCODE);

        await listenAndRelayForever(globalNetwork, policyContract);
    } finally {
        gateway.close();
        client.close();
    }
}

main().catch((error) => {
    console.error('[relay] erreur fatale :', error);
    process.exitCode = 1;
});

/**
 * Boucle d'ecoute principale, avec reprise automatique en cas d'erreur de
 * connexion gRPC. Un checkpointer en memoire est utilise pour ce prototype :
 * il evite de retraiter un evenement deja vu au sein d'une meme execution du
 * processus, mais ne survit pas a un redemarrage. Pour une version au-dela
 * du prototype, un FileCheckpointer (fourni par le SDK) serait necessaire
 * pour conserver la progression entre deux executions du relais.
 */
async function listenAndRelayForever(globalNetwork, policyContract) {
    const checkpointer = checkpointers.inMemory();

    // eslint-disable-next-line no-constant-condition
    while (true) {
        let events;
        try {
            events = await globalNetwork.getChaincodeEvents(GOVERNANCE_CHAINCODE, {
                checkpoint: checkpointer,
            });

            console.log(`[relay] ecoute des evenements "${ATTESTATION_EVENT_NAME}" sur ${GLOBAL_CHANNEL}...`);

            for await (const event of events) {
                await handleEvent(event, policyContract, checkpointer);
            }
        } catch (error) {
            if (isCancelledByClose(error)) {
                break;
            }
            console.error('[relay] erreur de connexion pendant l\'ecoute, nouvelle tentative dans 5s :', error.message || error);
            await sleep(5000);
        } finally {
            events?.close();
        }
    }
}

/**
 * Traite un evenement de chaincode recu depuis global-channel.
 * Seuls les evenements AttestationValidated dont le champ "valid" est vrai
 * sont relayes : un resultat negatif sur global-channel signifie que
 * Prequester, Ptrust, ou le consentement ont deja echoue, et il est inutile
 * (et incoherent avec le principe fail-fast) d'evaluer Powner sur une
 * demande deja refusee.
 */
async function handleEvent(event, policyContract, checkpointer) {
    if (event.eventName !== ATTESTATION_EVENT_NAME) {
        await checkpointer.checkpointChaincodeEvent(event);
        return;
    }

    let attestationResult;
    try {
        attestationResult = JSON.parse(utf8Decoder.decode(event.payload));
    } catch (parseError) {
        console.error('[relay] impossible de parser le payload de l\'evenement, evenement ignore :', parseError.message);
        await checkpointer.checkpointChaincodeEvent(event);
        return;
    }

    console.log(`[relay] evenement recu : attestation_id=${attestationResult.attestation_id} valid=${attestationResult.valid}`);

    if (!attestationResult.valid) {
        console.log(`[relay] attestation invalide (${attestationResult.deny_reason}), non relayee vers project-channel`);
        await checkpointer.checkpointChaincodeEvent(event);
        return;
    }

    try {
        const attestationResultJSON = JSON.stringify(attestationResult);

        console.log(`[relay] soumission de EvaluatePolicy sur project-channel pour attestation_id=${attestationResult.attestation_id}`);

        const resultBytes = await policyContract.submitTransaction(
            'EvaluatePolicy',
            attestationResultJSON,
        );

        const decision = JSON.parse(utf8Decoder.decode(resultBytes));
        console.log(`[relay] decision enregistree : ${decision.decision} (deny_reason="${decision.deny_reason || ''}")`);
    } catch (submitError) {
        // Une erreur de soumission ne doit pas bloquer le relais : elle est
        // journalisee, et l'evenement est tout de meme marque comme traite
        // pour eviter une boucle infinie de re-tentatives sur un evenement
        // structurellement invalide (par exemple une ressource jamais
        // enregistree sur project-channel). Pour une version au-dela du
        // prototype, une file de re-tentatives avec retry borne serait
        // preferable a cet abandon immediat.
        console.error('[relay] echec de soumission a PolicyContract :', submitError.message || submitError);
    }

    await checkpointer.checkpointChaincodeEvent(event);
}

function isCancelledByClose(error) {
    return error instanceof GatewayError && error.code === grpc.status.CANCELLED.valueOf();
}

function sleep(ms) {
    return new Promise((resolve) => setTimeout(resolve, ms));
}
