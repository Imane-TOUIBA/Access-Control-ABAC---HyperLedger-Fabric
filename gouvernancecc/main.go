package main

import (
	"log"

	"github.com/hyperledger/fabric-contract-api-go/v2/contractapi"
)

// Ce chaincode est destiné à être installé et commité sur global-channel
// (Org1MSP, Org2MSP, Org3MSP).
//
// Il regroupe deux smart contracts qui partagent le même world state :
//   - TrustContract        : registre des conventions de collaboration (Ptrust)
//   - AttestationContract  : ressources, consentements, et orchestration
//     Prequester -> Ptrust -> Consentement
//
// Les deux contrats sont dans le même binaire pour pouvoir s'appeler
// directement en Go (sans passer par InvokeChaincode), ce qui est cohérent
// avec la documentation Fabric : plusieurs smart contracts ne devraient
// être regroupés dans le même chaincode que s'ils partagent le même world
// state, ce qui est le cas ici (AttestationContract lit les conventions
// écrites par TrustContract).
func main() {
	trustContract := &TrustContract{}
	attestationContract := &AttestationContract{}

	chaincode, err := contractapi.NewChaincode(trustContract, attestationContract)
	if err != nil {
		log.Panicf("Erreur lors de la création du chaincode governancecc : %v", err)
	}

	if err := chaincode.Start(); err != nil {
		log.Panicf("Erreur lors du démarrage du chaincode governancecc : %v", err)
	}
}
