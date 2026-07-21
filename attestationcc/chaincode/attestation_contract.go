package chaincode

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/hyperledger/fabric-chaincode-go/v2/pkg/cid"
	"github.com/hyperledger/fabric-contract-api-go/v2/contractapi"
)

// AttestationContract gère les conventions inter-organisationnelles 
// et l'attestation des demandes d'accès sur le canal de gouvernance.
type AttestationContract struct {
	contractapi.Contract
}

type Convention struct {
	OwnerOrg    string   `json:"ownerOrg"`
	PartnerOrg  string   `json:"partnerOrg"`
	Projets     []string `json:"projets"`
	TExpiration string   `json:"tExpiration"`
	Revoked     bool     `json:"revoked"`
}

type AccessRequest struct {
	RequestID    string `json:"requestId"`
	RequesterOrg string `json:"requesterOrg"`
	OwnerOrg     string `json:"ownerOrg"`
	Resource     string `json:"resource"`
	Action       string `json:"action"`
	Projet       string `json:"projet"`
	Timestamp    string `json:"timestamp"`
}

type Attestation struct {
	AttestationID string `json:"attestationId"`
	RequestID     string `json:"requestId"`
	RequesterOrg  string `json:"requesterOrg"`
	OwnerOrg      string `json:"ownerOrg"`
	Projet        string `json:"projet"`
	ConvValide    bool   `json:"convValide"`
	Timestamp     string `json:"timestamp"`
}

// RegisterConvention : Enregistre une convention
func (c *AttestationContract) RegisterConvention(
	ctx contractapi.TransactionContextInterface,
	partnerOrg string,
	projetsJSON string,
	tExpiration string,
) error {
	callerOrg, err := cid.GetMSPID(ctx.GetStub())
	if err != nil {
		return fmt.Errorf("impossible de récupérer l'organisation appelante : %v", err)
	}
	if callerOrg == partnerOrg {
		return fmt.Errorf("une organisation ne peut pas créer une convention avec elle-même")
	}

	var projets []string
	if err := json.Unmarshal([]byte(projetsJSON), &projets); err != nil {
		return fmt.Errorf("projetsJSON invalide : %v", err)
	}
	if len(projets) == 0 {
		return fmt.Errorf("la convention doit couvrir au moins un projet")
	}

	expTime, err := time.Parse(time.RFC3339, tExpiration)
	if err != nil {
		return fmt.Errorf("tExpiration invalide (RFC3339 attendu) : %v", err)
	}

	txTs, _ := ctx.GetStub().GetTxTimestamp()
	txTime := time.Unix(txTs.Seconds, int64(txTs.Nanos)).UTC()
	if expTime.Before(txTime) {
		return fmt.Errorf("tExpiration doit être dans le futur")
	}

	convention := Convention{
		OwnerOrg:    callerOrg,
		PartnerOrg:  partnerOrg,
		Projets:     projets,
		TExpiration: tExpiration,
		Revoked:     false,
	}

	key, _ := ctx.GetStub().CreateCompositeKey("CONV", []string{callerOrg, partnerOrg})
	data, _ := json.Marshal(convention)

	return ctx.GetStub().PutState(key, data)
}

// RevokeConvention : Révoque une convention
func (c *AttestationContract) RevokeConvention(ctx contractapi.TransactionContextInterface, partnerOrg string) error {
	callerOrg, err := cid.GetMSPID(ctx.GetStub())
	if err != nil {
		return fmt.Errorf("impossible de récupérer l'organisation appelante : %v", err)
	}

	key, _ := ctx.GetStub().CreateCompositeKey("CONV", []string{callerOrg, partnerOrg})
	data, err := ctx.GetStub().GetState(key)
	if err != nil || data == nil {
		return fmt.Errorf("convention non trouvée")
	}

	var conv Convention
	json.Unmarshal(data, &conv)
	conv.Revoked = true

	updated, _ := json.Marshal(conv)
	return ctx.GetStub().PutState(key, updated)
}

// RegisterAccessRequest : Enregistre une demande d'accès
func (c *AttestationContract) RegisterAccessRequest(
	ctx contractapi.TransactionContextInterface,
	requestID string,
	ownerOrg string,
	resource string,
	action string,
	projet string,
) error {
	requesterOrg, _ := cid.GetMSPID(ctx.GetStub())

	key, _ := ctx.GetStub().CreateCompositeKey("REQUEST", []string{requestID})
	if existing, _ := ctx.GetStub().GetState(key); existing != nil {
		return fmt.Errorf("requestID %s déjà existant", requestID)
	}

	txTs, _ := ctx.GetStub().GetTxTimestamp()
	timestamp := time.Unix(txTs.Seconds, int64(txTs.Nanos)).UTC().Format(time.RFC3339)

	request := AccessRequest{
		RequestID:    requestID,
		RequesterOrg: requesterOrg,
		OwnerOrg:     ownerOrg,
		Resource:     resource,
		Action:       action,
		Projet:       projet,
		Timestamp:    timestamp,
	}

	data, _ := json.Marshal(request)
	return ctx.GetStub().PutState(key, data)
}

// VerifyAndAttest : Vérifie P_trust et émet une attestation persistée
func (c *AttestationContract) VerifyAndAttest(ctx contractapi.TransactionContextInterface, requestID string) (*Attestation, error) {
	// Lecture de la demande
	reqKey, _ := ctx.GetStub().CreateCompositeKey("REQUEST", []string{requestID})
	reqData, err := ctx.GetStub().GetState(reqKey)
	if err != nil || reqData == nil {
		return nil, fmt.Errorf("demande %s non trouvée", requestID)
	}

	var request AccessRequest
	json.Unmarshal(reqData, &request)

	// Vérification convention (P_trust)
	convValide := false
	convKey, _ := ctx.GetStub().CreateCompositeKey("CONV", []string{request.OwnerOrg, request.RequesterOrg})
	convData, _ := ctx.GetStub().GetState(convKey)

	if convData != nil {
		var conv Convention
		json.Unmarshal(convData, &conv)
		if !conv.Revoked {
			expTime, _ := time.Parse(time.RFC3339, conv.TExpiration)
			txTs, _ := ctx.GetStub().GetTxTimestamp()
			txTime := time.Unix(txTs.Seconds, int64(txTs.Nanos)).UTC()
			if txTime.Before(expTime) {
				for _, p := range conv.Projets {
					if p == request.Projet {
						convValide = true
						break
					}
				}
			}
		}
	}

	// Création attestation
	txID := ctx.GetStub().GetTxID()
	txTs, _ := ctx.GetStub().GetTxTimestamp()
	timestamp := time.Unix(txTs.Seconds, int64(txTs.Nanos)).UTC().Format(time.RFC3339)

	attestation := Attestation{
		AttestationID: fmt.Sprintf("ATT-%s", txID),
		RequestID:     requestID,
		RequesterOrg:  request.RequesterOrg,
		OwnerOrg:      request.OwnerOrg,
		Projet:        request.Projet,
		ConvValide:    convValide,
		Timestamp:     timestamp,
	}

	attJSON, _ := json.Marshal(attestation)
	attKey, _ := ctx.GetStub().CreateCompositeKey("ATTESTATION", []string{attestation.AttestationID})
	ctx.GetStub().PutState(attKey, attJSON)

	ctx.GetStub().SetEvent("AttestationCreated", attJSON)

	return &attestation, nil
}
