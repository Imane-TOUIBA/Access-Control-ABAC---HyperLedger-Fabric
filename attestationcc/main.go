/*
SPDX-License-Identifier: Apache-2.0
*/

package main

import (
	"log"

	"github.com/Imane-TOUIBA/fabric-samples/attestationcc/chaincode"

	"github.com/hyperledger/fabric-contract-api-go/v2/contractapi"
)

func main() {
	attestationChaincode, err := contractapi.NewChaincode(&chaincode.AttestationContract{})
	if err != nil {
		log.Panicf("Erreur lors de la création du chaincode AttestationContract : %v", err)
	}

	if err := attestationChaincode.Start(); err != nil {
		log.Panicf("Erreur lors du démarrage du chaincode AttestationContract : %v", err)
	}
}

