package main

import (
	"log"

	"github.com/hyperledger/fabric-contract-api-go/v2/contractapi"
)

// Ce chaincode est destiné à être installé et commité sur project-channel
// (CGNMSP, IBMSP uniquement, HUMSP étant exclue de ce canal).
//
// Il contient un seul smart contract : PolicyContract, qui permet
// d'enregistrer une politique de ressource (ResourcePolicy), de l'évaluer
// (Powner), et d'enregistrer la décision finale (DecisionRecord).
func main() {
	policyContract := &PolicyContract{}

	chaincode, err := contractapi.NewChaincode(policyContract)
	if err != nil {
		log.Panicf("Erreur lors de la création du chaincode policycc : %v", err)
	}

	if err := chaincode.Start(); err != nil {
		log.Panicf("Erreur lors du démarrage du chaincode policycc : %v", err)
	}
}
